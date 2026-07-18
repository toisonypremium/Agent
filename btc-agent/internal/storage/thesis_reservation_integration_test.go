package storage

import (
	"context"
	"path/filepath"
	"testing"

	"btc-agent/internal/liveguard"
)

func TestManagedThesisBuyRequiresLedgerBeforeExchange(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "thesis-integration.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	writeUnknownOutcomeQualityReport(t)
	cfg := unknownOutcomeConfig()
	plan := unknownOutcomePlan()
	plan.Assets[0].ThesisID = "thesis-e2e"
	plan.Assets[0].Layers[0].ThesisID = "thesis-e2e"
	exchange := &acceptedThenTimeoutExchange{}
	execCtx := liveguard.ManagedExecutionContext{BTCAccumulationPhase: "ACCUMULATION_CONFIRMED", FirstOrderDryRunApproved: true}

	missing := liveguard.ManageLiveOrdersWithRecorderAndContext(context.Background(), cfg, plan, nil, nil, nil, exchange, exchange, alwaysRunningHaltReader{}, execCtx, db, false)
	if len(exchange.accepted) != 0 || len(missing.Blocked) == 0 {
		t.Fatalf("missing thesis ledger reached exchange: calls=%d result=%+v", len(exchange.accepted), missing)
	}
	if err := db.SaveThesisCapitalLedger(ThesisCapitalLedger{ThesisID: "thesis-e2e", Symbol: "ETHUSDT", MaxExposureUSDT: 2, RemainingDCAUSDT: 2, Status: "ACCUMULATING"}); err != nil {
		t.Fatal(err)
	}
	valid := liveguard.ManageLiveOrdersWithRecorderAndContext(context.Background(), cfg, plan, nil, nil, nil, exchange, exchange, alwaysRunningHaltReader{}, execCtx, db, false)
	if len(exchange.accepted) != 1 {
		t.Fatalf("valid thesis BUY exchange calls=%d result=%+v", len(exchange.accepted), valid)
	}
	ledger, err := db.ThesisCapitalLedgerByID("thesis-e2e")
	if err != nil {
		t.Fatal(err)
	}
	if ledger.ReservedUSDT != 2 || ledger.RemainingDCAUSDT != 0 {
		t.Fatalf("atomic reservation missing: %+v", ledger)
	}
}
