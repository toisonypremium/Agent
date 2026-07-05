package telegramreport

import (
	"strings"
	"testing"

	"btc-agent/internal/agent2"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/research"
)

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
		AutoLiveBlockers: []string{"operator halt active"},
	})
	for _, want := range []string{
		"CHƯA SẴN SÀNG ĐẶT LỆNH",
		"KHÔNG đặt lệnh",
		"Agent 2 chưa tạo layer ACTIVE_LIMIT",
		"Operator halt đang bật",
		"OKX env: đủ",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("text missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "apiKey") || strings.Contains(text, "secret") || strings.Contains(text, "passphrase value") {
		t.Fatalf("text appears to leak secret-like content:\n%s", text)
	}
}

func TestExplainBlockerMapsAutoEnv(t *testing.T) {
	got := ExplainBlocker("BTC_AGENT_ALLOW_AUTO_LIVE=true required for auto live execution")
	if !strings.Contains(got, "Auto live chưa bật") || !strings.Contains(got, "khóa an toàn") {
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
		Candidates:    []liveguard.CandidateOrder{{Symbol: "ETHUSDT", Side: "BUY", Price: 100, Notional: 2, PostOnly: true, Canary: true}},
		TotalNotional: 2,
	})
	for _, want := range []string{"Auto ladder", "ĐÃ gửi", "ETHUSDT", "2.00 USDT"} {
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
	for _, want := range []string{"Live supervisor", "SUPERVISOR_WARN", "managed_cycle", "Consecutive errors: 1", "Managed cycle", "Blocked: 1", "spot limit BUY post-only"} {
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
	for _, want := range []string{"Research Brief", "Research-only", "BTC exchange outage", "https://example.com/news", "BTC"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
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
	for _, want := range []string{"Quản lý live orders", "Đã hủy: 1", "Đã đặt mới: 1", "Theo từng coin", "ETHUSDT", "SOLUSDT", "RENDERUSDT", "Vì sao chưa đặt", "Trigger tiếp theo", "State: ACTIVE_LIMIT", "Pending: 2.00 USDT", "không futures"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %s", want, got)
		}
	}
}
