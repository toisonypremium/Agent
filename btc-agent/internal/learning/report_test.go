package learning

import (
	"strings"
	"testing"
)

func TestMarkdownContainsSafetyAndEvidence(t *testing.T) {
	result := RecommendationResult{Summary: "Learning recommendations total=1 actionable=1 watch=0 manual_review_required=true", Recommendations: []Recommendation{{
		Area:           AreaExit,
		Title:          "Review exit settings",
		Recommendation: "Manual review candidate.",
		ManualAction:   "Review manually; do not place orders.",
		Confidence:     ConfidenceMedium,
		Severity:       SeverityActionable,
		Evidence:       []Evidence{{Metric: "take_profit", Value: "5.0%"}},
	}}}
	md := Markdown(result)
	for _, want := range []string{"LEARNING RECOMMENDATIONS REPORT", "Deterministic engine remains authority", "Manual review required", "No order placement", "take_profit", "Do not auto-tune production config", "Do not place live orders"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}
