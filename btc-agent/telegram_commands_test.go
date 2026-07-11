package main

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/liveguard"
)

func TestNormalizeTelegramCommandReadOnlyAllowlist(t *testing.T) {
	for _, input := range []string{"/status", "/why now", "/coins@btc_agent_bot", "/filters", "/scorecard", "/allocation", "/capital", "/universe", "/dashboard", "/trigger", "/orders", "/positions", "/doctor", "/supervisor", "/next", "/risk", "/help"} {
		if got := normalizeTelegramCommand(input); got == "" {
			t.Fatalf("expected allowed command for %q", input)
		}
	}
	for _, input := range []string{"buy", "/buy", "/sell", "/market", "/leverage", "/override", "/resume", "/halt", "/cancel", "/close"} {
		if got := normalizeTelegramCommand(input); got != "" {
			t.Fatalf("expected blocked command for %q, got %q", input, got)
		}
	}
}

func TestTelegramChatAllowedExactMatch(t *testing.T) {
	if !telegramChatAllowed("12345", 12345) {
		t.Fatal("expected exact chat id allowed")
	}
	if telegramChatAllowed("12345", 67890) {
		t.Fatal("expected different chat id blocked")
	}
	if telegramChatAllowed("", 12345) {
		t.Fatal("expected empty configured chat blocked")
	}
}

func TestTelegramCommandsHelpIsReadOnly(t *testing.T) {
	text := telegramCommandsHelp()
	for _, want := range []string{"/status", "/why", "/coins", "/filters", "/scorecard", "/allocation", "/capital", "/universe", "/dashboard", "/trigger", "/orders", "/positions", "/doctor", "/supervisor", "/next", "/risk", "Không có lệnh đặt mua/bán"} {
		if !strings.Contains(text, want) {
			t.Fatalf("help missing %q:\n%s", want, text)
		}
	}
	for _, blocked := range []string{"/buy", "/sell", "/market", "/leverage", "/override", "/cancel", "/close"} {
		if strings.Contains(text, blocked) {
			t.Fatalf("help exposes blocked command %q:\n%s", blocked, text)
		}
	}
}

func TestTelegramCommandFiltersIsReadOnly(t *testing.T) {
	report := FilterAttributionReport{
		Summary:   "Filter attribution coins=1 near_actionable=0 top_blocker=DISCOUNT_ZONE",
		Aggregate: []FilterAttributionAggregateRow{{Key: "DISCOUNT_ZONE", Count: 1}},
		Coins:     []FilterAttributionCoinRow{{Symbol: "ETHUSDT", State: agent2.StateWatch, SetupScore: 0.62, TopBlockerKey: "DISCOUNT_ZONE", FailedSoft: 2}},
		Safety:    "spot limit BUY post-only only; no futures, no leverage, no market order",
	}
	text := telegramCommandFilters(report)
	for _, want := range []string{"BTC Agent — Filters", "DISCOUNT_ZONE", "Read-only", "không bypass ACTIVE_LIMIT"} {
		if !strings.Contains(text, want) {
			t.Fatalf("filters reply missing %q:\n%s", want, text)
		}
	}
}

func TestTelegramCommandPositionsIsReadOnly(t *testing.T) {
	report := liveguard.LiveLedgerReport{GeneratedAt: time.Now(), Summary: "no live positions recorded"}
	text := telegramCommandPositions(report)
	for _, want := range []string{"BTC Agent — Positions", "Không có vị thế live", "Read-only"} {
		if !strings.Contains(text, want) {
			t.Fatalf("positions reply missing %q:\n%s", want, text)
		}
	}
}

func TestTelegramCommandScorecardIsReadOnly(t *testing.T) {
	report := TechnicalScorecardReport{Summary: "Technical scorecard coins=1", Coins: []TechnicalScorecardCoin{{Symbol: "ETHUSDT", State: agent2.StateWatch, TechnicalScore: 0.71, Verdict: TechnicalVerdictNearReady, RewardRisk: 2.1, TopBlockerKey: "DISCOUNT_ZONE"}}}
	text := telegramCommandScorecard(report)
	for _, want := range []string{"BTC Agent — Scorecard", "ETHUSDT", "Read-only", "không bypass ACTIVE_LIMIT"} {
		if !strings.Contains(text, want) {
			t.Fatalf("scorecard reply missing %q:\n%s", want, text)
		}
	}
}

func TestTelegramCommandAllocationIsResearchOnly(t *testing.T) {
	report := CapitalPlanResearchReport{Summary: "Capital research plan coins=1", Coins: []CapitalPlanResearchCoin{{Symbol: "ETHUSDT", State: agent2.StateWatch, CurrentConfigAllocation: 0.4, SuggestedResearchAllocation: 0.5, MaxResearchNotional: 50, OpportunityScore: 70, OpportunityVerdict: agent2.OpportunityVerdictNormal, SuggestedLayers: 2}}}
	text := telegramCommandAllocation(report)
	for _, want := range []string{"BTC Agent — Capital Research", "ETHUSDT", "Research-only", "không sửa config allocation"} {
		if !strings.Contains(text, want) {
			t.Fatalf("allocation reply missing %q:\n%s", want, text)
		}
	}
}

func TestTelegramCommandUniverseIsResearchOnly(t *testing.T) {
	report := agent2.UniverseResearchReport{Summary: "Universe research symbols=1", TopCandidates: []agent2.UniverseResearchRow{{Symbol: "LINKUSDT", State: agent2.StateWatch, DataStatus: agent2.UniverseDataOK, OpportunityScore: 66, OpportunityVerdict: agent2.OpportunityVerdictNormal}}}
	text := telegramCommandUniverse(report)
	for _, want := range []string{"BTC Agent — Universe Research", "LINKUSDT", "Research-only", "không tự thay production assets"} {
		if !strings.Contains(text, want) {
			t.Fatalf("universe reply missing %q:\n%s", want, text)
		}
	}
}

func TestTelegramCommandDashboardIsReadOnly(t *testing.T) {
	report := DecisionDashboardReport{BotReady: true, MarketReady: false, CanSubmitNow: false, PlanState: agent2.StateWatch, BTCPermission: "WATCH", BestProductionCoin: "ETHUSDT", BestUniverseCoin: "LINKUSDT", NextTrigger: "Chờ BTC ALLOWED", Blockers: []string{"plan chưa ACTIVE_LIMIT"}}
	text := telegramCommandDashboard(report)
	for _, want := range []string{"BTC Agent — Dashboard", "ETHUSDT", "Read-only", "không bypass ACTIVE_LIMIT"} {
		if !strings.Contains(text, want) {
			t.Fatalf("dashboard reply missing %q:\n%s", want, text)
		}
	}
}

func TestTelegramCommandTriggerIsReadOnly(t *testing.T) {
	report := DecisionDashboardReport{NextTrigger: "Chờ BTC ALLOWED", Blockers: []string{"plan chưa ACTIVE_LIMIT"}, Actions: []string{"Đứng ngoài"}}
	text := telegramCommandTrigger(report)
	for _, want := range []string{"BTC Agent — Trigger", "Chờ BTC ALLOWED", "Read-only", "không bypass ACTIVE_LIMIT"} {
		if !strings.Contains(text, want) {
			t.Fatalf("trigger reply missing %q:\n%s", want, text)
		}
	}
}
