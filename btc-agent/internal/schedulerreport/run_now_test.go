package schedulerreport

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/liquidity"
	"btc-agent/internal/liveguard"
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
		Plan:            agent2.Plan{State: agent2.StateWatch, Watchlist: agent2.WatchlistReport{Candidates: []agent2.WatchCandidate{{Symbol: "ETHUSDT", State: agent2.StateWatch, ReadinessScore: 0.49, Tier: agent2.WatchTierEarly, MMCase: agent2.MMCaseNoEdge, MMScore: 10, MMMissing: []string{"chưa thấy sweep/reclaim/absorption đủ rõ"}, LiquidityQuality: liquidity.Quality{Enabled: true, Grade: liquidity.GradeD, Score: 22, Reasons: []string{"liquidity gate: order quá lớn"}}, DiscountGap: 0.12, RewardRisk: 2.2, NextTrigger: "Chờ sweep low + close reclaim support + retest giữ vùng."}}}},
		ShadowProbe:     liveguard.ShadowProbeJournal{Profile: liveguard.ShadowProfileArmedProbeLight, ProductionPermission: agent1.Watch, ResearchPermission: agent1.Watch, Blockers: []string{"BTC research profile not ARMED"}},
		ResearchSummary: "Tin nền trung lập",
		DailyOK:         true,
		ReconcileOK:     true,
	})
	for _, want := range []string{
		"I. KẾT LUẬN",
		"II. BTC & KỊCH BẢN",
		"III. DANH SÁCH THEO DÕI",
		"IV. BOT & AN TOÀN",
		"Tin tức:",
		"Phân tích ngày=OK",
		"Không ĐỦ ĐIỀU KIỆN ĐẶT LỆNH: không đặt lệnh",
		"mở khóa=",
		"dòng tiền lớn=CHƯA CÓ LỢI THẾ RÕ",
		"thanh khoản=D",
		"Không hợp đồng tương lai",
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

func TestCompactPlanIncludesMMLiquidityEvidence(t *testing.T) {
	plan := agent2.Plan{
		Assets: []agent2.AssetPlan{{
			Symbol: "ETHUSDT", State: agent2.StateWatch, MMCase: agent2.MMCaseNoEdge, MMScore: 12,
			MMMissing: []string{"missing mm"}, LiquidityQuality: liquidity.Quality{Grade: liquidity.GradeD, Score: 33, Reasons: []string{"thin liquidity"}},
			HardBlockers: []string{"BTC permission WATCH"}, NextTrigger: "Chờ BTC.",
		}},
		Watchlist: agent2.WatchlistReport{Candidates: []agent2.WatchCandidate{{
			Symbol: "ETHUSDT", MMCase: agent2.MMCaseNoEdge, MMScore: 12, MMMissing: []string{"missing mm"},
			LiquidityQuality: liquidity.Quality{Grade: liquidity.GradeD, Score: 33, Reasons: []string{"thin liquidity"}},
			EntryChecklist:   []agent2.EntryChecklistItem{{Name: agent2.EntryCheckMMAccumulation, Pass: false}},
		}}},
	}
	got := CompactPlan(plan)
	assets := got["assets"].([]map[string]any)
	watch := got["watchlist"].([]map[string]any)
	if assets[0]["mm_case"] != agent2.MMCaseNoEdge || assets[0]["liquidity_grade"] != liquidity.GradeD {
		t.Fatalf("asset compact missing MM/liquidity: %+v", assets[0])
	}
	if watch[0]["mm_score"] != float64(12) || watch[0]["liquidity_reasons"] == nil || watch[0]["entry_checklist"] == nil {
		t.Fatalf("watch compact missing evidence: %+v", watch[0])
	}
}

func TestBuildDeterministicIncludesUnlockConditions(t *testing.T) {
	text := BuildDeterministic(RunNowSnapshot{Analysis: agent1.MarketAnalysis{TrendScore: 19.8, MarketRegime: "DOWNTREND", ActionPermission: agent1.Watch}, Plan: agent2.Plan{State: agent2.StateWatch}, DailyOK: true, ReconcileOK: true})
	for _, want := range []string{"Cần:", "Điểm xu hướng cần", "Không ĐỦ ĐIỀU KIỆN ĐẶT LỆNH", "chính=", "vô hiệu=", "Không hợp đồng tương lai"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	// Old probe language must be gone
	for _, stale := range []string{"WATCH không tạo probe", "ARMED mới probe nhỏ", "ALLOWED mới ladder"} {
		if strings.Contains(text, stale) {
			t.Fatalf("stale phrase %q still present:\n%s", stale, text)
		}
	}
}
