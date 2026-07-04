package liveguard

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

type fakeHaltReader struct {
	halted bool
	err    error
}

func (f fakeHaltReader) IsHalted() (bool, error) {
	return f.halted, f.err
}

type fakeOrderPlacer struct {
	result live.OrderResult
	err    error
	called bool
	req    live.LimitOrderRequest
}

func (f *fakeOrderPlacer) PlaceSpotLimitOrder(ctx context.Context, req live.LimitOrderRequest) (live.OrderResult, error) {
	f.called = true
	f.req = req
	return f.result, f.err
}

func TestExecuteManualProofOrderBlocksWithoutConfirm(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	placer := &fakeOrderPlacer{}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, "wrong", placer, fakeHaltReader{halted: false})
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
}

func TestExecuteManualProofOrderBlocksProofNotReady(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	proof.Status = NotReadyNoDeterministicOrder
	placer := &fakeOrderPlacer{}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer, fakeHaltReader{halted: false})
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
}

func TestExecuteManualProofOrderBlocksRealTradingFalse(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	cfg.Execution.RealTradingEnabled = false
	placer := &fakeOrderPlacer{}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer, fakeHaltReader{halted: false})
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
}

func TestExecuteManualProofOrderBlocksProofOnly(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	cfg.Live.ProofOnly = true
	placer := &fakeOrderPlacer{}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer, fakeHaltReader{halted: false})
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
}

func TestExecuteManualProofOrderBlocksBadPreflight(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	proof.Preflight.Pass = false
	placer := &fakeOrderPlacer{}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer, fakeHaltReader{halted: false})
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
}

func TestExecuteManualProofOrderBlocksMarketCandidate(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	proof.Candidate.Type = "market"
	placer := &fakeOrderPlacer{}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer, fakeHaltReader{halted: false})
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
}

func TestExecuteManualProofOrderSubmits(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	placer := &fakeOrderPlacer{result: live.OrderResult{InstID: "ETH-USDT", OrderID: "123", ClientOrderID: "abc", Submitted: true}}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer, fakeHaltReader{halted: false})
	if got.Status != LiveOrderSubmitted || !placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
	if placer.req.InstID != "ETH-USDT" || placer.req.Side != "buy" || !placer.req.PostOnly {
		t.Fatalf("bad request: %+v", placer.req)
	}
}

func TestExecuteManualProofOrderNoSecretLeak(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	secret := "supersecret"
	proof.Status = NotReadyNoDeterministicOrder
	proof.Summary = secret
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, &fakeOrderPlacer{}, fakeHaltReader{halted: false})
	if strings.Contains(got.Summary, secret) || strings.Contains(strings.Join(got.Reasons, " "), secret) {
		t.Fatalf("secret leaked: %+v", got)
	}
}

func TestExecuteAutoProofOrderBlocksWhenDisabled(t *testing.T) {
	cfg, proof := autoExecutableConfigAndProof()
	cfg.Live.AutoExecute = false
	placer := &fakeOrderPlacer{}
	got := ExecuteAutoProofOrder(context.Background(), cfg, proof, placer, nil, nil, fakeHaltReader{halted: false})
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
}

func TestExecuteAutoProofOrderBlocksOpenLiveOrder(t *testing.T) {
	cfg, proof := autoExecutableConfigAndProof()
	placer := &fakeOrderPlacer{}
	open := []live.OrderStatus{{InstID: "ETH-USDT", ClientOrderID: "existing", Status: live.StatusLiveOpen}}
	got := ExecuteAutoProofOrder(context.Background(), cfg, proof, placer, open, nil, fakeHaltReader{halted: false})
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
	if !strings.Contains(strings.Join(got.Reasons, " "), "open live order") {
		t.Fatalf("missing open order blocker: %+v", got.Reasons)
	}
}

func TestExecuteAutoProofOrderBlocksBudgetExceeded(t *testing.T) {
	cfg, proof := autoExecutableConfigAndProof()
	placer := &fakeOrderPlacer{}
	positions := []live.LivePosition{{Symbol: "ETHUSDT", CostBasis: 244}}
	got := ExecuteAutoProofOrder(context.Background(), cfg, proof, placer, nil, positions, fakeHaltReader{halted: false})
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
	if !strings.Contains(strings.Join(got.Reasons, " "), "budget") {
		t.Fatalf("missing budget blocker: %+v", got.Reasons)
	}
}

func TestExecuteAutoProofOrderSubmits(t *testing.T) {
	cfg, proof := autoExecutableConfigAndProof()
	placer := &fakeOrderPlacer{result: live.OrderResult{InstID: "ETH-USDT", OrderID: "123", ClientOrderID: "abc", Submitted: true}}
	got := ExecuteAutoProofOrder(context.Background(), cfg, proof, placer, nil, nil, fakeHaltReader{halted: false})
	if got.Status != LiveOrderSubmitted || !placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
	if placer.req.InstID != "ETH-USDT" || placer.req.Side != "buy" || !placer.req.PostOnly {
		t.Fatalf("bad request: %+v", placer.req)
	}
}

func executableConfigAndProof() (config.Config, Proof) {
	var cfg config.Config
	cfg.Live.Enabled = true
	cfg.Live.ProofOnly = false
	cfg.Live.RequireManualConfirm = true
	cfg.Live.RequirePostOnly = true
	cfg.Live.MaxOrderNotionalUSDT = 10
	cfg.Execution.RealTradingEnabled = true
	cfg.Risk.NoFutures = true
	cfg.Risk.NoLeverage = true
	cfg.Risk.SpotLimitOnly = true
	proof := Proof{
		Status:    ReadyForManualLiveProofOrder,
		Candidate: CandidateOrder{Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 100, Quantity: 0.05, Notional: 5, PostOnly: true},
		Preflight: PreflightResult{Enabled: true, Pass: true, Symbol: "ETHUSDT", InstID: "ETH-USDT", Price: 100, Quantity: 0.05, Notional: 5},
		Account:   AccountCheck{Enabled: true, AuthOK: true, BalanceOK: true, FreeUSDT: 20},
	}
	_ = agent2.StateWatch
	return cfg, proof
}

func autoExecutableConfigAndProof() (config.Config, Proof) {
	cfg, proof := executableConfigAndProof()
	cfg.Live.AutoExecute = true
	cfg.Live.RequireManualConfirm = false
	cfg.Portfolio.TotalCapital = 1000
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": 0.35}
	cfg.Risk.MaxTotalDeploymentPerCycle = 0.70
	cfg.Risk.MaxSingleAssetDeployment = 0.45
	return cfg, proof
}

func TestExecuteManualProofOrderBlocksWhenHalted(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	placer := &fakeOrderPlacer{}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer, fakeHaltReader{halted: true})
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result when halted: %+v called=%v", got, placer.called)
	}
	if !strings.Contains(strings.Join(got.Reasons, " "), "operator halt active") {
		t.Fatalf("expected operator halt active blocker, got: %+v", got.Reasons)
	}
}

func TestExecuteAutoProofOrderBlocksWhenHalted(t *testing.T) {
	cfg, proof := autoExecutableConfigAndProof()
	placer := &fakeOrderPlacer{}
	got := ExecuteAutoProofOrder(context.Background(), cfg, proof, placer, nil, nil, fakeHaltReader{halted: true})
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result when halted: %+v called=%v", got, placer.called)
	}
	if !strings.Contains(strings.Join(got.Reasons, " "), "operator halt active") {
		t.Fatalf("expected operator halt active blocker, got: %+v", got.Reasons)
	}
}

func TestExecuteManualProofOrderBlocksWhenHaltReaderErrorsOrNil(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	placer := &fakeOrderPlacer{}

	// Nil haltReader
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer, nil)
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result with nil reader: %+v", got)
	}

	// Error haltReader
	got2 := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer, fakeHaltReader{err: fmt.Errorf("db error")})
	if got2.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result with error reader: %+v", got2)
	}
}

func TestExecuteAutoProofOrderBlocksWhenHaltReaderErrorsOrNil(t *testing.T) {
	cfg, proof := autoExecutableConfigAndProof()
	placer := &fakeOrderPlacer{}

	// Nil haltReader
	got := ExecuteAutoProofOrder(context.Background(), cfg, proof, placer, nil, nil, nil)
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result with nil reader: %+v", got)
	}

	// Error haltReader
	got2 := ExecuteAutoProofOrder(context.Background(), cfg, proof, placer, nil, nil, fakeHaltReader{err: fmt.Errorf("db error")})
	if got2.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result with error reader: %+v", got2)
	}
}
