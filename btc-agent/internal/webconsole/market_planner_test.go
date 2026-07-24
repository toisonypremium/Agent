package webconsole

import (
	"path/filepath"
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/storage"
)

func TestMarketPlannerReadModelIsTypedAndUnavailableWithoutInputs(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "market.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc, err := NewService(db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.MarketPlanner()
	if err != nil {
		t.Fatal(err)
	}
	if out.Available {
		t.Fatalf("empty=%+v", out)
	}
	now := time.Date(2026, 7, 24, 2, 0, 0, 0, time.UTC)
	if err := db.SaveAnalysis(agent1.MarketAnalysis{Timestamp: now, BTCPrice: 100, MarketRegime: "RANGE", ActionPermission: agent1.Watch, PermissionReason: "wait for confirmation", RiskLevel: agent1.Medium, FallingKnifeRisk: agent1.Low, FomoRisk: agent1.Low, Summary: "market summary"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SavePlan(agent2.Plan{Timestamp: now, State: agent2.StateWatch, ActionPermission: agent1.Watch, Summary: "plan summary", Warnings: []string{"stale asset input"}}); err != nil {
		t.Fatal(err)
	}
	out, err = svc.MarketPlanner()
	if err != nil {
		t.Fatal(err)
	}
	if !out.Available || out.Permission != "WATCH" || out.PlanState != "WATCH" || out.MarketSummary != "market summary" || len(out.Warnings) != 1 {
		t.Fatalf("out=%+v", out)
	}
}
