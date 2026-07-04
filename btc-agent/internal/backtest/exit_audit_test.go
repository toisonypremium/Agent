package backtest

import (
	"strings"
	"testing"

	"btc-agent/internal/market"
)

func TestRunExitAuditProducesRows(t *testing.T) {
	cfg := simConfig()
	cfg.Data.Symbols.Assets = []string{"ETHUSDT"}
	btc := map[string][]market.Candle{"1d": simCandles("BTCUSDT", 90, 100)}
	assets := map[string][]market.Candle{"ETHUSDT": simCandles("ETHUSDT", 90, 100)}
	got, err := RunExitAudit(cfg, btc, assets, ExitAuditConfig{TakeProfitPcts: []float64{0.03}, TimeStopDays: []int{0}, TargetSymbols: []string{"ETHUSDT"}})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || len(got.Rows) != 1 || got.Rows[0].Verdict == "" {
		t.Fatalf("expected one exit audit row with verdict: %+v", got)
	}
}

func TestExitAuditVerdictRejectsNoPlans(t *testing.T) {
	row := ExitAuditRow{Symbol: "ETHUSDT"}
	if got := exitAuditVerdict(row); got != "REJECT" {
		t.Fatalf("verdict=%s want REJECT", got)
	}
}

func TestExitAuditMarkdownSection(t *testing.T) {
	got, err := RunBTC(Config{MinWindow1D: 30, HorizonDays: []int{1, 3}}, btcCandles(70))
	if err != nil {
		t.Fatal(err)
	}
	got.ExitAudit = ExitAuditResult{Enabled: true, Summary: "exit audit test", Rows: []ExitAuditRow{{Symbol: "ETHUSDT", TakeProfitPct: 0.03, TimeStopDays: 3, PlansCreated: 1, OrdersFilled: 1, TakeProfits: 1, Verdict: "WATCH"}}}
	md := Markdown(got)
	if !strings.Contains(md, "Agent 2 Exit / Take-Profit Audit") {
		t.Fatalf("markdown missing exit audit section:\n%s", md)
	}
}
