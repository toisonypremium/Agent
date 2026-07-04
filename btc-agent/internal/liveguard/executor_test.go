package liveguard

import (
	"context"
	"strings"
	"testing"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

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
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, "wrong", placer)
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
}

func TestExecuteManualProofOrderBlocksProofNotReady(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	proof.Status = NotReadyNoDeterministicOrder
	placer := &fakeOrderPlacer{}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer)
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
}

func TestExecuteManualProofOrderBlocksRealTradingFalse(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	cfg.Execution.RealTradingEnabled = false
	placer := &fakeOrderPlacer{}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer)
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
}

func TestExecuteManualProofOrderBlocksProofOnly(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	cfg.Live.ProofOnly = true
	placer := &fakeOrderPlacer{}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer)
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
}

func TestExecuteManualProofOrderBlocksBadPreflight(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	proof.Preflight.Pass = false
	placer := &fakeOrderPlacer{}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer)
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
}

func TestExecuteManualProofOrderBlocksMarketCandidate(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	proof.Candidate.Type = "market"
	placer := &fakeOrderPlacer{}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer)
	if got.Status != LiveOrderBlocked || placer.called {
		t.Fatalf("unexpected result: %+v called=%v", got, placer.called)
	}
}

func TestExecuteManualProofOrderSubmits(t *testing.T) {
	cfg, proof := executableConfigAndProof()
	placer := &fakeOrderPlacer{result: live.OrderResult{InstID: "ETH-USDT", OrderID: "123", ClientOrderID: "abc", Submitted: true}}
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, placer)
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
	got := ExecuteManualProofOrder(context.Background(), cfg, proof, ManualLiveConfirmPhrase, &fakeOrderPlacer{})
	if strings.Contains(got.Summary, secret) || strings.Contains(strings.Join(got.Reasons, " "), secret) {
		t.Fatalf("secret leaked: %+v", got)
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
