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
	calls  int
	reqs   []live.LimitOrderRequest
}

func (f *fakeOrderPlacer) PlaceSpotLimitOrder(ctx context.Context, req live.LimitOrderRequest) (live.OrderResult, error) {
	f.called = true
	f.calls++
	f.req = req
	f.reqs = append(f.reqs, req)
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

func TestExecuteAutoLadderProofOrderSubmits(t *testing.T) {
	cfg, _ := autoExecutableConfigAndProof()
	cfg.Live.AutoLadderEnabled = true
	cfg.Live.MaxAutoLayersPerCycle = 2
	cfg.Live.MaxOpenLiveOrders = 2
	cfg.Live.AutoLadderMaxNotionalUSDT = 4
	proof := LadderProof{
		Status:  ReadyForManualLiveProofOrder,
		Account: AccountCheck{Enabled: true, AuthOK: true, BalanceOK: true, FreeUSDT: 20},
		Candidates: []CandidateOrder{
			{Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 100, Quantity: 0.02, Notional: 2, PostOnly: true, Canary: true},
			{Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 90, Quantity: 0.02222222, Notional: 2, PostOnly: true, Canary: true},
		},
		Preflights:    []PreflightResult{{Enabled: true, Pass: true, Symbol: "ETHUSDT", InstID: "ETH-USDT"}, {Enabled: true, Pass: true, Symbol: "ETHUSDT", InstID: "ETH-USDT"}},
		TotalNotional: 4,
	}
	placer := &fakeOrderPlacer{result: live.OrderResult{InstID: "ETH-USDT", OrderID: "123", ClientOrderID: "abc", Submitted: true}}
	got := ExecuteAutoLadderProofOrder(context.Background(), cfg, proof, placer, nil, nil, fakeHaltReader{halted: false})
	if got.Status != LiveOrderSubmitted || placer.calls != 2 {
		t.Fatalf("unexpected result: %+v calls=%d", got, placer.calls)
	}
}

func TestExecuteAutoLadderProofOrderBlocksOpenLimit(t *testing.T) {
	cfg, _ := autoExecutableConfigAndProof()
	cfg.Live.AutoLadderEnabled = true
	cfg.Live.MaxOpenLiveOrders = 1
	cfg.Live.AutoLadderMaxNotionalUSDT = 2
	proof := LadderProof{Status: ReadyForManualLiveProofOrder, Account: AccountCheck{AuthOK: true, BalanceOK: true}, Candidates: []CandidateOrder{{Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 100, Quantity: 0.02, Notional: 2, PostOnly: true}}, Preflights: []PreflightResult{{Pass: true}}, TotalNotional: 2}
	placer := &fakeOrderPlacer{}
	open := []live.OrderStatus{{InstID: "ETH-USDT", Status: live.StatusLiveOpen}}
	got := ExecuteAutoLadderProofOrder(context.Background(), cfg, proof, placer, open, nil, fakeHaltReader{halted: false})
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
}

func TestExecuteAutoLadderProofOrderReportsZeroNotionalClearly(t *testing.T) {
	cfg, _ := autoExecutableConfigAndProof()
	cfg.Live.AutoLadderEnabled = true
	cfg.Live.MaxOpenLiveOrders = 1
	cfg.Live.AutoLadderMaxNotionalUSDT = 2
	proof := LadderProof{Status: NotReadyNoDeterministicOrder, Account: AccountCheck{AuthOK: true, BalanceOK: true}, TotalNotional: 0}
	placer := &fakeOrderPlacer{}
	got := ExecuteAutoLadderProofOrder(context.Background(), cfg, proof, placer, nil, nil, fakeHaltReader{halted: false})
	joined := strings.Join(got.Reasons, " ")
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
	if strings.Contains(joined, "ladder total notional above max") {
		t.Fatalf("zero notional reported as above max: %+v", got.Reasons)
	}
	if !strings.Contains(joined, "ladder total notional must be positive") {
		t.Fatalf("missing zero-notional blocker: %+v", got.Reasons)
	}
}

func TestClientOrderIDUniqueAndWithinOKXLimit(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := clientOrderID("RENDERUSDT", true)
		if seen[id] {
			t.Fatalf("duplicate client order ID: %s", id)
		}
		seen[id] = true
		if len(id) > 32 {
			t.Fatalf("client order ID too long: len=%d id=%s", len(id), id)
		}
		if !strings.HasPrefix(id, "btccanaryrenderusdt") {
			t.Fatalf("bad canary prefix: %s", id)
		}
	}
}

func TestExecuteManualProofOrderSanitizesExchangeError(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	secret := "manual-secret-value"
	t.Setenv("OKX_API_SECRET", secret)
	cfg.Live.APISecretEnv = "OKX_API_SECRET"
	placer := &fakeOrderPlacer{err: fmt.Errorf("okx order failed secret=%s OK-ACCESS-KEY=abc", secret)}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer, fakeHaltReader{halted: false})
	joined := got.Summary + " " + strings.Join(got.Reasons, " ")
	if got.Status != LiveOrderRejected || strings.Contains(joined, secret) || strings.Contains(joined, "abc") {
		t.Fatalf("exchange error not sanitized: %+v", got)
	}
}

func TestExecuteAutoProofOrderSanitizesExchangeError(t *testing.T) {
	cfg, proof := autoExecutableConfigAndProof()
	secret := "auto-secret-value"
	t.Setenv("OKX_API_KEY", secret)
	cfg.Live.APIKeyEnv = "OKX_API_KEY"
	placer := &fakeOrderPlacer{err: fmt.Errorf("okx order failed apiKey=%s", secret)}
	got := ExecuteAutoProofOrder(context.Background(), cfg, proof, placer, nil, nil, fakeHaltReader{halted: false})
	joined := got.Summary + " " + strings.Join(got.Reasons, " ")
	if got.Status != LiveOrderRejected || strings.Contains(joined, secret) {
		t.Fatalf("exchange error not sanitized: %+v", got)
	}
}

func TestExecuteAutoLadderProofOrderSanitizesExchangeError(t *testing.T) {
	cfg, _ := autoExecutableConfigAndProof()
	cfg.Live.AutoLadderEnabled = true
	cfg.Live.MaxAutoLayersPerCycle = 1
	cfg.Live.MaxOpenLiveOrders = 1
	cfg.Live.AutoLadderMaxNotionalUSDT = 2
	secret := "ladder-secret-value"
	t.Setenv("OKX_API_PASSPHRASE", secret)
	cfg.Live.APIPassphraseEnv = "OKX_API_PASSPHRASE"
	proof := LadderProof{Status: ReadyForManualLiveProofOrder, Account: AccountCheck{AuthOK: true, BalanceOK: true}, Candidates: []CandidateOrder{{Symbol: "ETHUSDT", Side: "BUY", Type: "limit", Price: 100, Quantity: 0.02, Notional: 2, PostOnly: true}}, Preflights: []PreflightResult{{Pass: true, Symbol: "ETHUSDT", InstID: "ETH-USDT"}}, TotalNotional: 2}
	placer := &fakeOrderPlacer{err: fmt.Errorf("okx ladder failed passphrase=%s", secret)}
	got := ExecuteAutoLadderProofOrder(context.Background(), cfg, proof, placer, nil, nil, fakeHaltReader{halted: false})
	joined := got.Summary + " " + strings.Join(got.Reasons, " ")
	if got.Status != LiveOrderRejected || strings.Contains(joined, secret) {
		t.Fatalf("exchange error not sanitized: %+v", got)
	}
}

func TestExecuteManualProofOrderCanaryPrefix(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	cfg.Live.CanaryMode = true
	cfg.Live.CanaryMaxNotionalUSDT = 2.0
	proof.Candidate.Canary = true
	proof.Candidate.Notional = 2.0
	proof.Preflight.Canary = true
	proof.Preflight.Notional = 2.0
	placer := &fakeOrderPlacer{result: live.OrderResult{InstID: "ETH-USDT", OrderID: "123", ClientOrderID: "abc", Submitted: true}}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer, fakeHaltReader{halted: false})
	if got.Status != LiveOrderSubmitted || !placer.called {
		t.Fatalf("unexpected result: %+v", got)
	}
	if !strings.HasPrefix(placer.req.ClientOrderID, "btccanary") {
		t.Fatalf("expected client order ID to start with btccanary, got: %s", placer.req.ClientOrderID)
	}
}

func TestExecuteAutoProofOrderCanaryPrefix(t *testing.T) {
	cfg, proof := autoExecutableConfigAndProof()
	cfg.Live.CanaryMode = true
	cfg.Live.CanaryMaxNotionalUSDT = 2.0
	proof.Candidate.Canary = true
	proof.Candidate.Notional = 2.0
	proof.Preflight.Canary = true
	proof.Preflight.Notional = 2.0
	placer := &fakeOrderPlacer{result: live.OrderResult{InstID: "ETH-USDT", OrderID: "123", ClientOrderID: "abc", Submitted: true}}
	got := ExecuteAutoProofOrder(context.Background(), cfg, proof, placer, nil, nil, fakeHaltReader{halted: false})
	if got.Status != LiveOrderSubmitted || !placer.called {
		t.Fatalf("unexpected result: %+v", got)
	}
	if !strings.HasPrefix(placer.req.ClientOrderID, "btccanary") {
		t.Fatalf("expected client order ID to start with btccanary, got: %s", placer.req.ClientOrderID)
	}
}
