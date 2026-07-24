package dca

import (
	"btc-agent/internal/agent2"
	"btc-agent/internal/storage"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildDesiredOrdersUsesAllocationAndExplicitThesisOnly(t *testing.T) {
	db, e := storage.Open(filepath.Join(t.TempDir(), "d.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	at := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	epoch, _, e := db.CreateDCAAllocationEpoch(storage.DCAAllocationEpochRequest{IdempotencyKey: "e", ObservedAvailableUSDT: 2000, EnvelopeUSDT: 1600, NetNewUSDT: 1600, ObservedAt: at})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.ApplyDCAAllocationEpochToTheses(epoch.ID); e != nil {
		t.Fatal(e)
	}
	plan := agent2.Plan{State: agent2.StateActiveLimit, Assets: []agent2.AssetPlan{{ThesisID: "thesis-eth", Symbol: "ETHUSDT", Layers: []agent2.Layer{{Index: 1, Price: 2000, Quantity: 1, Notional: 2000}}}}}
	gate := GateInput{MarketAllowed: true, LiquidityPass: true, RuntimeHealthy: true, ReconciliationClean: true, ArtifactFresh: true}
	out, blocked, e := BuildDesiredOrders(db, DesiredOrderInput{Plan: plan, Gate: gate, Now: at})
	if e != nil || len(blocked) != 0 || len(out) != 1 {
		t.Fatalf("out=%+v blocked=%+v err=%v", out, blocked, e)
	}
	d := out[0]
	if d.ThesisID != "thesis-eth" || d.Source != "dca_canary_layer_1" || !d.PostOnly || d.ExpiresAt.Sub(at) != 240*time.Minute || d.Notional != 32 {
		t.Fatalf("desired=%+v", d)
	}
	plan.Assets[0].ThesisID = ""
	out, blocked, e = BuildDesiredOrders(db, DesiredOrderInput{Plan: plan, Gate: gate, Now: at})
	if e != nil || len(out) != 0 || len(blocked) != 1 {
		t.Fatalf("unbound out=%+v blocked=%+v err=%v", out, blocked, e)
	}
}
