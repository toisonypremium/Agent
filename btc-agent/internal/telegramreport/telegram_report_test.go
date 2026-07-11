package telegramreport

import (
	"strings"
	"testing"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/liquidity"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/research"
)

func TestDailyHumanTextIncludesMMLiquidityWatchlist(t *testing.T) {
	plan := agent2.Plan{State: agent2.StateWatch, Watchlist: agent2.WatchlistReport{Candidates: []agent2.WatchCandidate{{
		Symbol: "ETHUSDT", ReadinessScore: 0.49, MMCase: agent2.MMCaseNoEdge, MMScore: 11,
		LiquidityQuality: liquidity.Quality{Grade: liquidity.GradeD, Score: 22, Reasons: []string{"liquidity thin"}},
		DiscountGap:      0.12, RewardRisk: 2.2, Missing: []string{"MM case NO_EDGE chưa đủ footprint"}, NextTrigger: "Chờ reclaim.",
	}}}}
	got := DailyHumanText(agent1.MarketAnalysis{ActionPermission: agent1.Watch}, plan)
	for _, want := range []string{"MM=NO_EDGE", "Liq=D", "gap 12.0%", "RR 2.20", "điều kiện kích hoạt: Chờ reclaim"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
}

func TestLiveReadinessHumanTextExplainsNotReady(t *testing.T) {
	text := LiveReadinessHumanText(LiveReadinessView{
		Mode:           "live-proof",
		OperatorHalted: true,
		CredentialEnvPresent: map[string]bool{
			"OKX_API_KEY":        true,
			"OKX_API_SECRET":     true,
			"OKX_API_PASSPHRASE": true,
		},
		PlanState: agent2.StateWatch,
		Proof: liveguard.Proof{
			Status:  liveguard.NotReadyNoDeterministicOrder,
			Reasons: []string{"no deterministic ACTIVE_LIMIT layer available"},
		},
		AutoLiveBlockers:     []string{"operator halt active"},
		ManagedCoinSummaries: []liveguard.ManagedCoinSummary{{Symbol: "ETHUSDT", State: agent2.StateScout, WhyNoOrder: []string{"BTC permission WATCH", "reward/risk thấp"}, NextTrigger: "Chờ BTC chuyển ALLOWED."}},
	})
	for _, want := range []string{
		"lý do đang chặn",
		"Khóa vận hành đang bật",
		"Biến môi trường OKX: đủ",
		"Giới hạn live-auto tùy chọn",
		"Vì sao chưa tự đặt lệnh",
		"ETHUSDT",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("text missing %q:\n%s", want, text)
		}
	}
	// Must NOT contain old proof-only language
	for _, stale := range []string{"CHƯA SẴN SÀNG ĐẶT LỆNH", "live-proof 24/7, chưa resume", "chưa bật auto"} {
		if strings.Contains(text, stale) {
			t.Fatalf("text contains stale proof-only phrase %q:\n%s", stale, text)
		}
	}
	if strings.Contains(text, "apiKey") || strings.Contains(text, "secret") || strings.Contains(text, "passphrase value") {
		t.Fatalf("text appears to leak secret-like content:\n%s", text)
	}
}

func TestExplainBlockerMapsAutoEnv(t *testing.T) {
	got := ExplainBlocker("BTC_AGENT_ALLOW_AUTO_LIVE=true required for auto live execution")
	if !strings.Contains(got, "Tự động giao dịch thật chưa bật") || !strings.Contains(got, "khóa an toàn") {
		t.Fatalf("unexpected blocker explanation: %s", got)
	}
}

func TestLiveProofHumanTextLengthAndSafety(t *testing.T) {
	proof := liveguard.Proof{Status: liveguard.NotReadyNoDeterministicOrder, Reasons: []string{"no deterministic ACTIVE_LIMIT layer available"}}
	text := LiveProofHumanText(proof)
	if len(text) > 3500 {
		t.Fatalf("telegram text too long: %d", len(text))
	}
	if !strings.Contains(text, "KHÔNG đặt lệnh") {
		t.Fatalf("expected no-order safety line:\n%s", text)
	}
}

func TestLiveLadderOrderHumanText(t *testing.T) {
	text := LiveLadderOrderHumanText(liveguard.LadderExecutionResult{
		Status:        liveguard.LiveOrderSubmitted,
		Candidates:    []liveguard.CandidateOrder{{Symbol: "ETHUSDT", Side: "BUY", Price: 100, Notional: 2, PostOnly: true, LiveAuto: true}},
		TotalNotional: 2,
	})
	for _, want := range []string{"Rải lệnh tự động", "ĐÃ gửi", "ETHUSDT", "2.00 USDT"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q:\n%s", want, text)
		}
	}
}

func TestLiveSupervisorHumanText(t *testing.T) {
	result := liveguard.SupervisorResult{
		Status:            liveguard.SupervisorWarn,
		Action:            liveguard.SupervisorActionManagedCycle,
		ConsecutiveErrors: 1,
		Reasons:           []string{"operator halt active"},
		Managed: &liveguard.ManagedCycleResult{
			Status:  liveguard.ManagedCycleBlocked,
			Desired: []liveguard.ManagedDesiredOrder{{Symbol: "ETHUSDT"}},
			Blocked: []liveguard.ManagedOrderDecision{{Symbol: "ETHUSDT", Reason: "risk governor block"}},
			Summary: "blocked",
			DryRun:  true,
		},
	}
	result.RefreshSummary()
	got := LiveSupervisorHumanText(result)
	for _, want := range []string{"Giám sát giao dịch thật", "SUPERVISOR_WARN", "mua giao ngay bằng lệnh giới hạn tạo thanh khoản", "mong muốn=1", "chặn=1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
	for _, leak := range []string{"OKX_API_SECRET", "telegram_token", "passphrase value"} {
		if strings.Contains(got, leak) {
			t.Fatalf("supervisor text leaks secret-like content %q: %s", leak, got)
		}
	}
}

func TestResearchBriefHumanText(t *testing.T) {
	got := ResearchBriefHumanText(research.BriefResult{
		Status:  research.BriefOK,
		Summary: "ok",
		Items:   []research.ResearchItem{{Source: "Test RSS", Title: "BTC exchange outage", URL: "https://example.com/news", Risk: research.RiskWarn, Tags: []string{"BTC", "OKX"}}},
	})
	for _, want := range []string{"Tóm tắt tin tức chiến lược", "Tin tức chỉ để tham khảo", "BTC exchange outage", "BTC", "KẾ HOẠCH BOT"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "https://example.com/news") {
		t.Fatalf("research Telegram should not include raw URLs: %s", got)
	}
	for _, leak := range []string{"OKX_API_SECRET", "telegram_token", "08ceca61", "276BDF"} {
		if strings.Contains(got, leak) {
			t.Fatalf("research text leaks secret-like content %q: %s", leak, got)
		}
	}
}

func TestLiveOrderManagementHumanText(t *testing.T) {
	placed := liveguard.ManagedOrderDecision{Action: "place", Symbol: "ETHUSDT", LayerIndex: 1, Reason: "missing active accumulation layer order", Desired: liveguard.ManagedDesiredOrder{Symbol: "ETHUSDT", LayerIndex: 1, Price: 100, Notional: 2}}
	canceled := liveguard.ManagedOrderDecision{Action: "cancel", Symbol: "SOLUSDT", LayerIndex: 1, Reason: "plan no longer ACTIVE_LIMIT"}
	result := liveguard.ManagedCycleResult{
		Status:    liveguard.ManagedCycleCompleted,
		PlanState: agent2.StateActiveLimit,
		Summary:   "managed ok",
		Placed:    []liveguard.ManagedOrderDecision{placed},
		Canceled:  []liveguard.ManagedOrderDecision{canceled},
		PerCoin: []liveguard.ManagedCoinSummary{
			{Symbol: "ETHUSDT", State: agent2.StateActiveLimit, DesiredLayers: 1, Placed: 1, PendingNotional: 2, Actions: []liveguard.ManagedOrderDecision{placed}, Reasons: []string{placed.Reason}},
			{Symbol: "SOLUSDT", State: agent2.StateWatch, OpenOrders: 1, Canceled: 1, Actions: []liveguard.ManagedOrderDecision{canceled}, Reasons: []string{canceled.Reason}},
			{Symbol: "RENDERUSDT", State: agent2.StateWatch, WhyNoOrder: []string{"giá chưa vào discount zone"}, NextTrigger: "Chờ giá về support/discount zone."},
		},
	}
	got := LiveOrderManagementHumanText(result)
	for _, want := range []string{"Quản lý lệnh thật", "Đã hủy: 1", "Đã đặt mới: 1", "Theo từng coin", "ETHUSDT", "SOLUSDT", "RENDERUSDT", "Vì sao chưa đặt", "Điều kiện kích hoạt tiếp theo", "Trạng thái: ĐỦ ĐIỀU KIỆN ĐẶT LỆNH", "Vốn đang chờ: 2.00 USDT", "không hợp đồng tương lai"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
}
