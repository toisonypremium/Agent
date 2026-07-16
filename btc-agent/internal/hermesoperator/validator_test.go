package hermesoperator

import (
	"strings"
	"testing"
	"time"
)

func validDecision(intent Intent) Decision {
	now := time.Date(2026, 7, 16, 5, 30, 0, 0, time.UTC)
	return Decision{
		Version: 1, DecisionID: "decision-1", GeneratedAt: now,
		ValidUntil: now.Add(2 * time.Minute), PortfolioRiskTier: RiskDefensive,
		Actions: []Action{{Symbol: "RENDERUSDT", Intent: intent, Confidence: 0.8, EntryPrice: 1.5, RequestedNotionalUSDT: 5, MaxLayers: 1}},
	}
}

func testPolicy() ValidationPolicy {
	return ValidationPolicy{Now: time.Date(2026, 7, 16, 5, 30, 30, 0, time.UTC), MaxDecisionTTL: 5 * time.Minute, MinConfidence: 0.6, MaxActions: 2, MaxProbeNotionalUSDT: 10, MaxActionNotionalUSDT: 100, AllowedSymbols: map[string]bool{"RENDERUSDT": true}}
}

func TestValidateAcceptsProbeWithinEnvelope(t *testing.T) {
	result := Validate(validDecision(IntentProbeLimit), testPolicy())
	if len(result.Reasons) != 0 {
		t.Fatalf("unexpected reasons: %v", result.Reasons)
	}
	if len(result.Actions) != 1 || result.Actions[0].Intent != IntentProbeLimit {
		t.Fatalf("unexpected actions: %+v", result.Actions)
	}
}

func TestValidateRejectsUnsafeOrInvalidAction(t *testing.T) {
	d := validDecision(IntentOpenLimit)
	d.Actions[0].Symbol = "BTCUSDT"
	d.Actions[0].RequestedNotionalUSDT = 101
	d.Actions[0].Confidence = 0.2
	result := Validate(d, testPolicy())
	joined := strings.Join(result.Reasons, " | ")
	if len(result.Actions) != 0 {
		t.Fatalf("invalid action must not pass validation: %+v", result.Actions)
	}
	for _, want := range []string{"symbol not allowed", "confidence below floor", "notional exceeds action cap"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %q", want, joined)
		}
	}
}

func TestValidateKeepsRiskReducingActions(t *testing.T) {
	d := validDecision(IntentCancel)
	d.Actions[0].EntryPrice = 0
	d.Actions[0].RequestedNotionalUSDT = 0
	result := Validate(d, testPolicy())
	if len(result.Reasons) != 0 {
		t.Fatalf("cancel should not need entry/notional: %v", result.Reasons)
	}
	if len(result.Actions) != 1 || result.Actions[0].Intent != IntentCancel {
		t.Fatalf("cancel action was dropped: %+v", result.Actions)
	}
}

func TestValidateRejectsExpiredDecision(t *testing.T) {
	d := validDecision(IntentProbeLimit)
	d.ValidUntil = testPolicy().Now
	result := Validate(d, testPolicy())
	if !strings.Contains(strings.Join(result.Reasons, " | "), "decision expired") {
		t.Fatalf("expected expiry reason: %v", result.Reasons)
	}
}

func TestPromptIncludesPolicyTTLAndEmptyArrayRule(t *testing.T) {
	prompt := PromptWithTTL(Snapshot{}, 90)
	if !strings.Contains(prompt, "+ 90 seconds") || !strings.Contains(prompt, "empty JSON array, not null") {
		t.Fatalf("prompt missing contract constraints: %s", prompt)
	}
}

func TestValidateReduceRequiresFiniteCappedNotionalAndLimitPrice(t *testing.T) {
	now := time.Now().UTC()
	base := Decision{Version: 1, DecisionID: "reduce-1", GeneratedAt: now, ValidUntil: now.Add(time.Minute), Actions: []Action{{Symbol: "BTCUSDT", Intent: IntentReduce, Confidence: 0.8}}}
	policy := ValidationPolicy{Now: now, MaxDecisionTTL: time.Hour, MaxActions: 2, MaxActionNotionalUSDT: 100, AllowedSymbols: map[string]bool{"BTCUSDT": true}}
	bad := Validate(base, policy)
	if len(bad.Reasons) != 2 {
		t.Fatalf("expected missing reduce sizing reasons: %+v", bad)
	}
	base.Actions[0].RequestedNotionalUSDT = 101
	base.Actions[0].EntryPrice = 60000
	capped := Validate(base, policy)
	if len(capped.Reasons) != 1 || !strings.Contains(capped.Reasons[0], "exceeds") {
		t.Fatalf("expected reduce cap: %+v", capped)
	}
	base.Actions[0].RequestedNotionalUSDT = 50
	good := Validate(base, policy)
	if len(good.Reasons) != 0 || len(good.Actions) != 1 {
		t.Fatalf("valid reduce rejected: %+v", good)
	}
}

func TestValidateCancelDoesNotRequireReduceSizing(t *testing.T) {
	now := time.Now().UTC()
	d := Decision{Version: 1, DecisionID: "cancel-1", GeneratedAt: now, ValidUntil: now.Add(time.Minute), Actions: []Action{{Symbol: "BTCUSDT", Intent: IntentCancel, Confidence: 0.8}}}
	got := Validate(d, ValidationPolicy{Now: now, MaxActions: 1, AllowedSymbols: map[string]bool{"BTCUSDT": true}})
	if len(got.Reasons) != 0 || len(got.Actions) != 1 {
		t.Fatalf("cancel sizing regression: %+v", got)
	}
}
