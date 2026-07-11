package survey

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Markdown(s RealDataSurvey) string {
	var b strings.Builder
	b.WriteString("REAL DATA SURVEY\n\n")
	b.WriteString("Summary: " + s.Summary + "\n\n")

	b.WriteString("1. Data coverage\n")
	b.WriteString(fmt.Sprintf("- %s\n", s.DataCoverage.Summary))
	b.WriteString("- Evidence confidence: " + s.DataCoverage.Confidence + "\n\n")

	writeSection(&b, "2. BTC gate", s.BTCGate)
	writeSection(&b, "3. Agent2 gate", s.Agent2Gate)
	writeSection(&b, "4. Managed live history", s.ManagedLive)

	b.WriteString("5. Learning actions\n")
	if len(s.LearningActions) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, action := range s.LearningActions {
			b.WriteString(fmt.Sprintf("- [%s/%s] %s: %s\n", action.Severity, action.Confidence, action.Area, action.Title))
			b.WriteString("  Recommendation: " + action.Recommendation + "\n")
			b.WriteString("  Manual action: " + action.ManualAction + "\n")
		}
	}
	b.WriteString("\n")

	b.WriteString("6. Safety\n")
	for _, note := range s.RiskNotes {
		b.WriteString("- " + note + "\n")
	}
	b.WriteString("- No real order was placed or canceled. Survey is report-only.\n")
	return b.String()
}

func writeSection(b *strings.Builder, title string, section SurveySection) {
	b.WriteString(title + "\n")
	b.WriteString(fmt.Sprintf("- Verdict: %s | confidence=%s\n", section.Verdict, section.Confidence))
	b.WriteString("- " + section.Summary + "\n")
	if len(section.Evidence) > 0 {
		b.WriteString("- Evidence:\n")
		for _, ev := range section.Evidence {
			line := fmt.Sprintf("  - %s=%s", ev.Metric, ev.Value)
			if ev.Note != "" {
				line += " (" + ev.Note + ")"
			}
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\n")
}

func SaveReports(dir string, result RealDataSurvey, markdown string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "real_data_survey_latest.md"), []byte(markdown), 0600); err != nil {
		return err
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "real_data_survey_latest.json"), b, 0600)
}
