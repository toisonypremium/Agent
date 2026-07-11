package liveguard

import (
	"context"
	"strings"
	"testing"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

type fakeBalanceReader struct {
	balances []live.Balance
	err      error
}

func (f fakeBalanceReader) AccountBalance(ctx context.Context) ([]live.Balance, error) {
	return f.balances, f.err
}

type fakeFilterReader struct {
	filters []live.InstrumentFilter
	err     error
}

func (f fakeFilterReader) InstrumentFilters(ctx context.Context) ([]live.InstrumentFilter, error) {
	return f.filters, f.err
}

type secretErr string

func (e secretErr) Error() string { return string(e) }

func TestBuildProofNoDeterministicOrder(t *testing.T) {
	cfg := liveTestConfig(t)
	got := BuildProof(cfg, agent2.Plan{State: agent2.StateWatch})
	if got.Status != NotReadyNoDeterministicOrder {
		t.Fatalf("status=%s want %s", got.Status, NotReadyNoDeterministicOrder)
	}
}

func TestBuildProofWithAccountNoDeterministicOrder(t *testing.T) {
	cfg := liveTestConfig(t)
	cfg.Live.MinAccountFreeUSDT = 20
	reader := fakeBalanceReader{balances: []live.Balance{{Asset: "USDT", Free: 25}}}
	got := BuildProofWithAccount(context.Background(), cfg, agent2.Plan{State: agent2.StateWatch}, reader)
	if got.Status != NotReadyNoDeterministicOrder {
		t.Fatalf("status=%s want %s", got.Status, NotReadyNoDeterministicOrder)
	}
	if !got.Account.Enabled || !got.Account.AuthOK || !got.Account.BalanceOK || got.Account.FreeUSDT != 25 {
		t.Fatalf("bad account check: %+v", got.Account)
	}
}

func TestBuildProofWithAccountLowBalance(t *testing.T) {
	cfg := liveTestConfig(t)
	cfg.Live.MinAccountFreeUSDT = 20
	reader := fakeBalanceReader{balances: []live.Balance{{Asset: "USDT", Free: 5}}}
	plan := agent2.Plan{State: agent2.StateActiveLimit, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, Layers: []agent2.Layer{{Index: 1, Price: 100, Notional: 50, Quantity: 0.5}}}}}
	got := BuildProofWithAccount(context.Background(), cfg, plan, reader)
	if got.Status != NotReadyBalance {
		t.Fatalf("status=%s want %s", got.Status, NotReadyBalance)
	}
	if !got.Account.AuthOK || got.Account.BalanceOK {
		t.Fatalf("bad account check: %+v", got.Account)
	}
}

func TestBuildProofWithAccountReaderErrorSanitized(t *testing.T) {
	cfg := liveTestConfig(t)
	secret := "set"
	reader := fakeBalanceReader{err: secretErr("upstream leaked " + secret)}
	got := BuildProofWithAccount(context.Background(), cfg, agent2.Plan{State: agent2.StateWatch}, reader)
	if got.Status != NotReadyBalance {
		t.Fatalf("status=%s want %s", got.Status, NotReadyBalance)
	}
	if strings.Contains(got.Account.Error, secret) {
		t.Fatalf("secret leaked in error: %q", got.Account.Error)
	}
}

func TestBuildProofWithChecksPreflightReady(t *testing.T) {
	cfg := liveTestConfig(t)
	cfg.Live.MinAccountFreeUSDT = 20
	plan := agent2.Plan{State: agent2.StateActiveLimit, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, Layers: []agent2.Layer{{Index: 1, Price: 100.019, Notional: 10, Quantity: 0.099981}}}}}
	balanceReader := fakeBalanceReader{balances: []live.Balance{{Asset: "USDT", Free: 25}}}
	filterReader := fakeFilterReader{filters: []live.InstrumentFilter{{Symbol: "ETHUSDT", InstID: "ETH-USDT", TickSize: 0.01, StepSize: 0.0001, MinSize: 0.001, MinNotional: 5}}}
	got := BuildProofWithChecks(context.Background(), cfg, plan, balanceReader, filterReader)
	if got.Status != ReadyForManualLiveProofOrder {
		t.Fatalf("unexpected proof: %+v", got)
	}
	if !got.Preflight.Pass || !near(got.Candidate.Price, 100.01) || !near(got.Candidate.Quantity, 0.0999) {
		t.Fatalf("bad preflight/candidate: proof=%+v", got)
	}
}

func TestBuildProofWithChecksFilterError(t *testing.T) {
	cfg := liveTestConfig(t)
	plan := agent2.Plan{State: agent2.StateActiveLimit, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, Layers: []agent2.Layer{{Index: 1, Price: 100, Notional: 10, Quantity: 0.1}}}}}
	balanceReader := fakeBalanceReader{balances: []live.Balance{{Asset: "USDT", Free: 25}}}
	filterReader := fakeFilterReader{err: secretErr("filter failed set")}
	got := BuildProofWithChecks(context.Background(), cfg, plan, balanceReader, filterReader)
	if got.Status != NotReadyFilters {
		t.Fatalf("status=%s want %s", got.Status, NotReadyFilters)
	}
	if strings.Contains(strings.Join(got.Preflight.Reasons, " "), "set") {
		t.Fatalf("secret leaked in preflight reasons: %+v", got.Preflight.Reasons)
	}
}

func TestBuildProofReadyCapsNotional(t *testing.T) {
	cfg := liveTestConfig(t)
	cfg.Live.MaxOrderNotionalUSDT = 5
	plan := agent2.Plan{State: agent2.StateActiveLimit, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, Layers: []agent2.Layer{{Index: 1, Price: 100, Notional: 50, Quantity: 0.5}}}}}
	got := BuildProof(cfg, plan)
	if got.Status != ReadyForManualLiveProofOrder {
		t.Fatalf("unexpected proof: %+v", got)
	}
	if got.Candidate.Notional != 5 || got.Candidate.Quantity != 0.05 || got.Candidate.Type != "limit" || got.Candidate.Side != "BUY" {
		t.Fatalf("bad candidate: %+v", got.Candidate)
	}
}

func TestBuildProofReadyLiveAutoModeScalesNotional(t *testing.T) {
	cfg := liveTestConfig(t)
	cfg.Live.LiveAutoMode = true
	cfg.Live.LiveAutoMaxNotionalUSDT = 2.0
	cfg.Live.MaxOrderNotionalUSDT = 10.0
	plan := agent2.Plan{State: agent2.StateActiveLimit, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, Layers: []agent2.Layer{{Index: 1, Price: 100, Notional: 50, Quantity: 0.5}}}}}
	got := BuildProof(cfg, plan)
	if got.Status != ReadyForManualLiveProofOrder {
		t.Fatalf("unexpected proof status: %+v", got)
	}
	if got.Candidate.Notional != 2.0 || got.Candidate.Quantity != 0.02 {
		t.Fatalf("live auto scaling failed in BuildProof: %+v", got.Candidate)
	}
}

func TestBuildLadderProofWithChecksReady(t *testing.T) {
	cfg := liveTestConfig(t)
	cfg.Execution.RealTradingEnabled = true
	cfg.Live.ProofOnly = false
	cfg.Live.AutoExecute = true
	cfg.Live.AutoLadderEnabled = true
	cfg.Live.LiveAutoMode = true
	cfg.Live.LiveAutoMaxNotionalUSDT = 2
	cfg.Live.AutoLadderMaxNotionalUSDT = 4
	cfg.Live.MaxAutoLayersPerCycle = 2
	plan := agent2.Plan{State: agent2.StateActiveLimit, Assets: []agent2.AssetPlan{{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, Layers: []agent2.Layer{{Index: 1, Price: 100, Notional: 2, Quantity: 0.02}, {Index: 2, Price: 90, Notional: 2, Quantity: 0.022222}}}}}
	balanceReader := fakeBalanceReader{balances: []live.Balance{{Asset: "USDT", Free: 25}}}
	filterReader := fakeFilterReader{filters: []live.InstrumentFilter{{Symbol: "ETHUSDT", InstID: "ETH-USDT", TickSize: 0.01, StepSize: 0.0001, MinSize: 0.001, MinNotional: 1}}}
	got := BuildLadderProofWithChecks(context.Background(), cfg, plan, balanceReader, filterReader)
	if got.Status != ReadyForManualLiveProofOrder || len(got.Candidates) != 2 {
		t.Fatalf("unexpected ladder proof: %+v", got)
	}
	if got.TotalNotional > 4.000001 {
		t.Fatalf("total notional not capped: %.2f", got.TotalNotional)
	}
}

func liveTestConfig(t *testing.T) config.Config {
	t.Helper()
	t.Setenv("OKX_API_KEY", "set")
	t.Setenv("OKX_API_SECRET", "set")
	t.Setenv("OKX_API_PASSPHRASE", "set")
	var cfg config.Config
	cfg.Risk.NoFutures = true
	cfg.Risk.NoLeverage = true
	cfg.Risk.SpotLimitOnly = true
	cfg.Live.Enabled = true
	cfg.Live.Exchange = "okx"
	cfg.Live.APIKeyEnv = "OKX_API_KEY"
	cfg.Live.APISecretEnv = "OKX_API_SECRET"
	cfg.Live.APIPassphraseEnv = "OKX_API_PASSPHRASE"
	cfg.Live.MaxOrderNotionalUSDT = 10
	cfg.Live.RequirePostOnly = true
	cfg.Live.ProofOnly = true
	return cfg
}
