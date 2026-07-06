package schedulerreport

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/market"
)

func TestBuildDeterministicHasRequiredSectionsAndSafety(t *testing.T) {
	text := BuildDeterministic(RunNowSnapshot{
		GeneratedAt: time.Date(2026, 7, 6, 1, 2, 0, 0, time.UTC),
		Analysis: agent1.MarketAnalysis{
			BTCPrice:           63712,
			MarketRegime:       "RANGING",
			TrendScore:         28,
			WeeklyBias:         "NEUTRAL",
			DailyBias:          "BEARISH",
			FourHourBias:       "NEUTRAL",
			RiskLevel:          agent1.Medium,
			FallingKnifeRisk:   agent1.Low,
			FomoRisk:           agent1.High,
			ActionPermission:   agent1.Watch,
			AccumulationZone:   market.Zone{Low: 60000, High: 62000},
			PrimarySupportZone: market.Zone{Low: 61000, High: 63000},
			DeepSupportZone:    market.Zone{Low: 56000, High: 58000},
			ResistanceZone:     market.Zone{Low: 66000, High: 68000},
			InvalidationZone:   market.Zone{Low: 54000, High: 55000},
			ScenarioMain:       "đi ngang chờ xác nhận",
		},
		Plan:            agent2.Plan{State: agent2.StateWatch},
		ResearchSummary: "Tin nền trung lập",
		DailyOK:         true,
		ReconcileOK:     true,
	})
	for _, want := range []string{
		"I. KẾT LUẬN",
		"II. PHÂN TÍCH KỸ THUẬT BTC",
		"III. KỊCH BẢN THỊ TRƯỜNG",
		"IV. KẾ HOẠCH BOT",
		"V. TIN TỨC / RESEARCH",
		"VI. TRẠNG THÁI THỰC THI",
		"Chưa có coin ACTIVE_LIMIT. Bot không đặt lệnh.",
		"không futures, không leverage, không market order",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestCompactPlanLimitsWatchlist(t *testing.T) {
	plan := agent2.Plan{}
	for i := 0; i < 7; i++ {
		plan.Watchlist.Candidates = append(plan.Watchlist.Candidates, agent2.WatchCandidate{Symbol: "X"})
	}
	got := CompactPlan(plan)
	watch := got["watchlist"].([]map[string]any)
	if len(watch) != 5 {
		t.Fatalf("watchlist len=%d want 5", len(watch))
	}
}
