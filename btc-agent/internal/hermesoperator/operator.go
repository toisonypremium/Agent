package hermesoperator

import (
	"context"
	"encoding/json"
	"fmt"
)

type JSONCaller interface {
	ChatJSON(context.Context, string, any) error
}

type Snapshot struct {
	GeneratedAt string `json:"generated_at"`
	Mode        string `json:"mode"`
	Market      any    `json:"market"`
	Assets      any    `json:"assets"`
	Positions   any    `json:"positions"`
	OpenOrders  any    `json:"open_orders"`
	Safety      any    `json:"safety"`
	Limits      any    `json:"limits"`
}

func Generate(ctx context.Context, caller JSONCaller, snapshot Snapshot, policy ValidationPolicy) (ValidationResult, error) {
	if caller == nil {
		return ValidationResult{Reasons: []string{"Hermes caller unavailable; no new exposure"}}, fmt.Errorf("Hermes caller unavailable")
	}
	var decision Decision
	if err := caller.ChatJSON(ctx, PromptWithTTL(snapshot, policyTTLSeconds(policy)), &decision); err != nil {
		return ValidationResult{Reasons: []string{"Hermes request failed; no new exposure"}}, err
	}
	return Validate(decision, policy), nil
}

func Prompt(snapshot Snapshot) string { return PromptWithTTL(snapshot, 120) }

func PromptWithTTL(snapshot Snapshot, ttlSeconds int) string {
	if ttlSeconds <= 0 {
		ttlSeconds = 120
	}
	payload, _ := json.Marshal(snapshot)
	return fmt.Sprintf(`Return exactly one JSON object and no markdown. You are Hermes, autonomous strategy operator for a spot-only bot. Choose only HOLD, WATCH, PROBE_LIMIT, OPEN_LIMIT, SCALE_LIMIT, CANCEL, REDUCE, or EXIT_LIMIT. You may choose strategy inside the supplied safety limits. Never request futures, leverage, market orders, shorts, withdrawals, symbols outside the universe, or notional above limits. In autonomous mode, legacy market labels such as NO_TRADE, WATCH, MARKDOWN, falling-knife, or blocked market authority are strategy and sizing inputs, not system execution locks: you may choose a confidence-sized PROBE_LIMIT when price/RR/liquidity and MM evidence justify it, and may OPEN_LIMIT or SCALE_LIMIT as confidence improves. HOLD is valid when evidence is weak. Operator halt, unhealthy/stale data, unclean reconciliation, unavailable exchange execution, ownership, spot-only/no-short constraints, and exposure caps remain non-overridable system safety. Risk-reducing actions are preferred when evidence, positions, or orders degrade. Use version=1, a unique decision_id, RFC3339 generated_at, and valid_until no later than generated_at + %d seconds. For HOLD or WATCH return actions as an empty JSON array, not null. Use concise reason_codes. Snapshot: %s`, ttlSeconds, payload)
}

func policyTTLSeconds(policy ValidationPolicy) int {
	seconds := int(policy.MaxDecisionTTL.Seconds())
	if seconds <= 0 {
		return 120
	}
	return seconds
}
