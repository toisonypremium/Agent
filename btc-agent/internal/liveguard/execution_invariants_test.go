package liveguard

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
)

// TestP0ManagedBuyExecutionInvariants proves that the normal managed-order path
// fails before the exchange boundary whenever a non-overridable authority or
// trading-mode invariant is missing.
func TestP0ManagedBuyExecutionInvariants(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*configFixture)
	}{
		{name: "live disabled", mutate: func(f *configFixture) { f.cfg.Live.Enabled = false }},
		{name: "auto execute disabled", mutate: func(f *configFixture) { f.cfg.Live.AutoExecute = false }},
		{name: "manual confirmation required", mutate: func(f *configFixture) { f.cfg.Live.RequireManualConfirm = true }},
		{name: "proof only", mutate: func(f *configFixture) { f.cfg.Live.ProofOnly = true }},
		{name: "real trading disabled", mutate: func(f *configFixture) { f.cfg.Execution.RealTradingEnabled = false }},
		{name: "futures prohibition missing", mutate: func(f *configFixture) { f.cfg.Risk.NoFutures = false }},
		{name: "leverage prohibition missing", mutate: func(f *configFixture) { f.cfg.Risk.NoLeverage = false }},
		{name: "spot limit policy missing", mutate: func(f *configFixture) { f.cfg.Risk.SpotLimitOnly = false }},
		{name: "plan not active", mutate: func(f *configFixture) { f.plan.State = agent2.StateWatch }},
		{name: "permission not allowed", mutate: func(f *configFixture) { f.plan.ActionPermission = agent1.Watch }},
		{name: "BTC accumulation not confirmed", mutate: func(f *configFixture) { f.exec.BTCAccumulationPhase = "MARKDOWN" }},
		{name: "first real order lacks dry run proof", mutate: func(f *configFixture) {
			f.cfg.Live.FirstOrderRequireDryRun = true
			f.exec.FirstOrderDryRunApproved = false
		}},
		{name: "operator halt active", mutate: func(f *configFixture) { f.halted = true }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newConfigFixture()
			tt.mutate(&fixture)
			exchange := &fakeManagedExchange{}
			recorder := &fakeManagedRecorder{}
			result := ManageLiveOrdersWithRecorderAndContext(
				context.Background(), fixture.cfg, fixture.plan, nil, nil, nil,
				exchange, exchange, fakeHaltReader{halted: fixture.halted}, fixture.exec, recorder, false,
			)
			if len(exchange.placed) != 0 {
				t.Fatalf("unsafe order reached exchange: case=%s placed=%+v result=%+v", tt.name, exchange.placed, result)
			}
			if len(recorder.submitted) != 0 {
				t.Fatalf("blocked order marked submitted: case=%s submitted=%v", tt.name, recorder.submitted)
			}
		})
	}
}

// TestP0ManagedBuyBlocksWhenPortfolioLossStateIsUnknown documents the next
// capital-safety invariant. A configured drawdown guard without a fresh,
// explicitly known portfolio-loss state must fail closed before exchange
// submission. The current execution context cannot carry that state, so this
// regression is intentionally expected to fail until the final assertion gains
// a typed portfolio-loss lock.
func TestP0ManagedBuyBlocksWhenPortfolioLossStateIsUnknown(t *testing.T) {
	fixture := newConfigFixture()
	fixture.cfg.Risk.MaxTotalEquityDrawdownPct = 0.10
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}})
	exchange := &fakeManagedExchange{}
	recorder := &fakeManagedRecorder{}
	result := ManageLiveOrdersWithRecorderAndContext(
		context.Background(), fixture.cfg, fixture.plan, nil, nil, nil,
		exchange, exchange, fakeHaltReader{}, fixture.exec, recorder, false,
	)
	if len(exchange.placed) != 0 {
		t.Fatalf("BUY reached exchange while portfolio loss state was unknown: placed=%d result=%+v", len(exchange.placed), result)
	}
	if len(recorder.submitted) != 0 {
		t.Fatalf("BUY marked submitted while portfolio loss state was unknown: submitted=%v", recorder.submitted)
	}
}

func TestP0ManagedBuyBlocksWhenPortfolioDrawdownLockIsActive(t *testing.T) {
	fixture := newConfigFixture()
	fixture.cfg.Risk.MaxTotalEquityDrawdownPct = 0.10
	fixture.exec.PortfolioLossStateKnown = true
	fixture.exec.PortfolioLossLockActive = true
	fixture.exec.PortfolioLossDrawdownPct = 0.12
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}})
	exchange := &fakeManagedExchange{}
	result := ManageLiveOrdersWithRecorderAndContext(context.Background(), fixture.cfg, fixture.plan, nil, nil, nil, exchange, exchange, fakeHaltReader{}, fixture.exec, &fakeManagedRecorder{}, false)
	if len(exchange.placed) != 0 {
		t.Fatalf("BUY reached exchange while portfolio drawdown lock was active: placed=%d result=%+v", len(exchange.placed), result)
	}
}

func TestP0ManagedBuyPassesWhenPortfolioDrawdownIsKnownAndBelowThreshold(t *testing.T) {
	fixture := newConfigFixture()
	fixture.cfg.Risk.MaxTotalEquityDrawdownPct = 0.10
	fixture.exec.PortfolioLossStateKnown = true
	fixture.exec.PortfolioLossDrawdownPct = 0.05
	writeHistoryQualityReportForTest(t, map[string]historyQualityScore{"ETHUSDT": {Score: 80, Grade: "A"}})
	exchange := &fakeManagedExchange{}
	recorder := &fakeManagedRecorder{}
	result := ManageLiveOrdersWithRecorderAndContext(context.Background(), fixture.cfg, fixture.plan, nil, nil, nil, exchange, exchange, fakeHaltReader{}, fixture.exec, recorder, false)
	if len(exchange.placed) != 1 || len(recorder.submitted) != 1 {
		t.Fatalf("known safe drawdown state must preserve valid BUY path: placed=%d submitted=%d result=%+v", len(exchange.placed), len(recorder.submitted), result)
	}
}

// TestP0HermesBuyExecutionInvariants exercises the final Hermes BUY boundary
// with deliberately malformed pre-built desired orders. This protects against
// a future caller bypassing the normal builder and preflight path.
func TestP0HermesBuyExecutionInvariants(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*hermesInvariantFixture)
	}{
		{name: "source is not Hermes operator", mutate: func(f *hermesInvariantFixture) { f.desired.Source = "REPORT" }},
		{name: "missing decision id", mutate: func(f *hermesInvariantFixture) { f.desired.DecisionID = "" }},
		{name: "missing intent", mutate: func(f *hermesInvariantFixture) { f.desired.Intent = "" }},
		{name: "sell cannot enter BUY path", mutate: func(f *hermesInvariantFixture) { f.desired.Side = "SELL" }},
		{name: "market order", mutate: func(f *hermesInvariantFixture) { f.desired.Type = "market" }},
		{name: "not post only", mutate: func(f *hermesInvariantFixture) { f.desired.PostOnly = false }},
		{name: "missing instrument", mutate: func(f *hermesInvariantFixture) { f.desired.InstID = "" }},
		{name: "zero price", mutate: func(f *hermesInvariantFixture) { f.desired.Price = 0 }},
		{name: "zero quantity", mutate: func(f *hermesInvariantFixture) { f.desired.Quantity = 0 }},
		{name: "zero notional", mutate: func(f *hermesInvariantFixture) { f.desired.Notional = 0 }},
		{name: "per order cap exceeded", mutate: func(f *hermesInvariantFixture) { f.desired.Notional = 11 }},
		{name: "canary allocation is not probe", mutate: func(f *hermesInvariantFixture) { f.desired.AllocationTier = string(OpportunityNormal) }},
		{name: "duplicate reservation", mutate: func(f *hermesInvariantFixture) { f.recorder.reserveErr = os.ErrExist }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newHermesInvariantFixture()
			tt.mutate(&fixture)
			result := ExecuteHermesDesiredOrders(
				context.Background(), fixture.cfg, fixture.plan, []ManagedDesiredOrder{fixture.desired}, nil,
				fixture.placer, fixture.recorder, fixture.exec, false,
			)
			if len(fixture.placer.calls) != 0 {
				t.Fatalf("unsafe Hermes order reached exchange: case=%s calls=%+v result=%+v", tt.name, fixture.placer.calls, result)
			}
			if len(fixture.recorder.submitted) != 0 {
				t.Fatalf("blocked Hermes order marked submitted: case=%s submitted=%v", tt.name, fixture.recorder.submitted)
			}
		})
	}
}

func TestP0HermesBuyExecutionPositiveControlCallsExchangeOnce(t *testing.T) {
	fixture := newHermesInvariantFixture()
	result := ExecuteHermesDesiredOrders(
		context.Background(), fixture.cfg, fixture.plan, []ManagedDesiredOrder{fixture.desired}, nil,
		fixture.placer, fixture.recorder, fixture.exec, false,
	)
	if len(fixture.placer.calls) != 1 || len(fixture.recorder.reserved) != 1 || len(fixture.recorder.submitted) != 1 {
		t.Fatalf("positive control must submit exactly once: calls=%d reserved=%d submitted=%d result=%+v",
			len(fixture.placer.calls), len(fixture.recorder.reserved), len(fixture.recorder.submitted), result)
	}
	request := fixture.placer.calls[0]
	if request.Side != "buy" || !request.PostOnly {
		t.Fatalf("positive control produced unsafe request: %+v", request)
	}
}

// TestP0ProductionOrderSubmissionCallerAllowlist makes every new production
// caller of PlaceSpotLimitOrder an explicit review event. Simulator method
// declarations are not CallExpr nodes and therefore do not enter this list.
func TestP0ProductionOrderSubmissionCallerAllowlist(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	got := map[string]int{}
	fileset := token.NewFileSet()

	err := filepath.WalkDir(repositoryRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			base := entry.Name()
			if base == ".git" || base == ".claude" || base == "vendor" || base == "backups" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || filepath.Base(path) == "execution_invariants_test.go" || len(path) >= 8 && path[len(path)-8:] == "_test.go" {
			return nil
		}
		file, parseErr := parser.ParseFile(fileset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		count := 0
		ast.Inspect(file, func(node ast.Node) bool {
			call, callOK := node.(*ast.CallExpr)
			if !callOK {
				return true
			}
			selector, selectorOK := call.Fun.(*ast.SelectorExpr)
			if selectorOK && selector.Sel.Name == "PlaceSpotLimitOrder" {
				count++
			}
			return true
		})
		if count > 0 {
			relative, relErr := filepath.Rel(repositoryRoot, path)
			if relErr != nil {
				return relErr
			}
			got[filepath.ToSlash(relative)] = count
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]int{
		"internal/liveguard/executor.go":         2,
		"internal/liveguard/exit_manager.go":     1,
		"internal/liveguard/hermes_execution.go": 2,
		"internal/liveguard/order_manager.go":    1,
	}
	if !equalCallerCounts(got, want) {
		t.Fatalf("production PlaceSpotLimitOrder caller set changed; review and update only after proving final safety gates\nwant=%v\n got=%v", sortedCallerCounts(want), sortedCallerCounts(got))
	}
}

type configFixture struct {
	cfg    config.Config
	plan   agent2.Plan
	exec   ManagedExecutionContext
	halted bool
}

func newConfigFixture() configFixture {
	cfg := managedConfig()
	cfg.Live.MaxAutoLayersPerAsset = 1
	plan := managedPlan()
	plan.Assets = plan.Assets[:1]
	plan.Assets[0].Layers = plan.Assets[0].Layers[:1]
	return configFixture{cfg: cfg, plan: plan, exec: confirmedExecContext()}
}

type hermesInvariantFixture struct {
	cfg      config.Config
	plan     agent2.Plan
	desired  ManagedDesiredOrder
	exec     ManagedExecutionContext
	placer   *hermesTestPlacer
	recorder *hermesTestRecorder
}

func newHermesInvariantFixture() hermesInvariantFixture {
	cfg := managedConfig()
	cfg.HermesOperator.Enabled = true
	cfg.HermesOperator.Mode = "canary"
	cfg.Live.FirstOrderRequireDryRun = false
	desired := ManagedDesiredOrder{
		Symbol: "ETHUSDT", InstID: "ETH-USDT", LayerIndex: 1,
		Side: "BUY", Type: "limit", Price: 100, Quantity: 0.02, Notional: 2, PostOnly: true,
		Source: "HERMES_OPERATOR", DecisionID: "p0-decision-1", Intent: "PROBE_LIMIT",
		AllocationTier: string(OpportunityProbe),
	}
	return hermesInvariantFixture{
		cfg: cfg, plan: agent2.Plan{State: agent2.StateScout}, desired: desired,
		exec:   ManagedExecutionContext{HermesMode: "canary"},
		placer: &hermesTestPlacer{}, recorder: &hermesTestRecorder{},
	}
}

func equalCallerCounts(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if b[key] != value {
			return false
		}
	}
	return true
}

func sortedCallerCounts(in map[string]int) []string {
	out := make([]string, 0, len(in))
	for path, count := range in {
		out = append(out, path+"="+string(rune('0'+count)))
	}
	sort.Strings(out)
	return out
}
