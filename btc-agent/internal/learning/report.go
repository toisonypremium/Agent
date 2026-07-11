package learning

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Markdown(result RecommendationResult) string {
	var b strings.Builder
	b.WriteString("LEARNING RECOMMENDATIONS REPORT\n\n")
	b.WriteString("1. Summary\n")
	b.WriteString("- " + result.Summary + "\n")
	if result.SurveySummary != "" {
		b.WriteString("- Survey: " + result.SurveySummary + "\n")
	}
	if result.EvidenceQuality != "" {
		b.WriteString("- Evidence quality: " + result.EvidenceQuality + "\n")
	}
	b.WriteString("- Deterministic engine remains authority. Manual review required before any rule/config/code change.\n")
	b.WriteString("- No order placement, config write, exchange call, or LLM call is performed by learn.\n\n")

	if len(result.SurveyActions) > 0 {
		b.WriteString("2. Survey actions\n")
		for _, action := range result.SurveyActions {
			b.WriteString(fmt.Sprintf("- [%s/%s] %s: %s\n", action.Severity, action.Confidence, action.Area, action.Title))
		}
		b.WriteString("\n")
	}

	b.WriteString("2. Top recommendations\n")
	b.WriteString("| Area | Severity | Confidence | Title | Manual action |\n")
	b.WriteString("|---|---|---|---|---|\n")
	if len(result.Recommendations) == 0 {
		b.WriteString("| - | - | - | none | - |\n")
	} else {
		for _, rec := range result.Recommendations {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n", rec.Area, rec.Severity, rec.Confidence, rec.Title, rec.ManualAction))
		}
	}
	b.WriteString("\n")

	b.WriteString("3. Evidence\n")
	for _, rec := range result.Recommendations {
		b.WriteString(fmt.Sprintf("\n%s — %s\n", rec.Area, rec.Title))
		b.WriteString("- Recommendation: " + rec.Recommendation + "\n")
		if len(rec.Evidence) == 0 {
			b.WriteString("- Evidence: none\n")
			continue
		}
		for _, ev := range rec.Evidence {
			if ev.Note != "" {
				b.WriteString(fmt.Sprintf("- %s: %s (%s)\n", ev.Metric, ev.Value, ev.Note))
			} else {
				b.WriteString(fmt.Sprintf("- %s: %s\n", ev.Metric, ev.Value))
			}
		}
	}
	b.WriteString("\n4. Safety\n")
	b.WriteString("- Recommendations are diagnostic only.\n")
	b.WriteString("- Do not auto-tune production config from this report.\n")
	b.WriteString("- Do not place live orders from this report.\n")
	return b.String()
}

func SaveReports(dir string, result RecommendationResult, markdown string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "learning_latest.md"), []byte(markdown), 0600); err != nil {
		return err
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "learning_latest.json"), b, 0600)
}
