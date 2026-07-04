package aiagent

import (
	"context"
	"testing"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
)

type fakeCaller struct{ report Report }

func (f fakeCaller) ChatJSON(ctx context.Context, prompt string, out any) error {
	*rptr(out) = f.report
	return nil
}

func rptr(out any) *Report { return out.(*Report) }

func TestSafetyAllowsVietnameseNegation(t *testing.T) {
	got := CheckSafety("Không vào lệnh, không đặt lệnh thật, no futures, no leverage.", false)
	if !got.Pass {
		t.Fatalf("expected safe negation pass: %+v", got)
	}
}

func TestSafetyBlocksUnsafeEntry(t *testing.T) {
	got := CheckSafety("mua ngay, vào lệnh ngay với leverage", false)
	if got.Pass || len(got.Reasons) == 0 {
		t.Fatalf("expected unsafe text blocked: %+v", got)
	}
}

func TestGenerateFallsBackOnOverride(t *testing.T) {
	snap := Snapshot{Analysis: agent1.MarketAnalysis{ActionPermission: agent1.Watch, MarketRegime: "RANGE", RiskLevel: agent1.Medium}}
	caller := fakeCaller{report: Report{DeterministicDecision: string(agent1.Watch), TelegramText: "override deterministic and buy now", OverrideEngine: true}}
	got, err := Generate(context.Background(), caller, snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.OverrideEngine || got.DeterministicDecision != string(agent1.Watch) {
		t.Fatalf("expected fallback deterministic report: %+v", got)
	}
}

func TestPromptIncludesDeterministicDecision(t *testing.T) {
	snap := Snapshot{Analysis: agent1.MarketAnalysis{ActionPermission: agent1.NoTrade}, Plan: agent2.Plan{State: agent2.StateWatch}}
	got := Prompt(snap)
	if got == "" || !contains(got, "NO_TRADE") {
		t.Fatalf("prompt missing deterministic decision: %s", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
