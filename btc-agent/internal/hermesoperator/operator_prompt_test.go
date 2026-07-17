package hermesoperator

import (
	"strings"
	"testing"
)

func TestPromptUsesQuantReasoningAsDefaultProposal(t *testing.T) {
	p := Prompt(Snapshot{Assets: []map[string]any{{"symbol": "RENDERUSDT", "quant_reasoning": map[string]any{"eligible": true, "recommendation": "PROBE_LIMIT"}}}})
	for _, want := range []string{"quant_reasoning.eligible=true", "suggested_notional_usdt", "never as safety authority"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing %q: %s", want, p)
		}
	}
}
