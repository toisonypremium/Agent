package research

import "fmt"

func DoctorMarkdown(result DoctorResult) string {
	md := fmt.Sprintf("RESEARCH DOCTOR\n\nGenerated: %s\nStatus: %s\nSummary: %s\n\n", result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Status, result.Summary)
	for _, ch := range result.Channels {
		md += fmt.Sprintf("- %s: enabled=%v usable=%v checked=%d", ch.Name, ch.Enabled, ch.Usable, ch.Checked)
		if ch.Error != "" {
			md += " error=" + ch.Error
		}
		md += "\n"
	}
	if len(result.Blockers) > 0 {
		md += "\nBlockers:\n"
		for _, item := range result.Blockers {
			md += "- " + item + "\n"
		}
	}
	if len(result.Warnings) > 0 {
		md += "\nWarnings:\n"
		for _, item := range result.Warnings {
			md += "- " + item + "\n"
		}
	}
	md += "\nResearch-only: no orders placed, no live safety gate changed.\n"
	return md
}

func BriefMarkdown(result BriefResult) string {
	md := fmt.Sprintf("RESEARCH BRIEF\n\nGenerated: %s\nStatus: %s\nSummary: %s\nSources checked: %d\n\n", result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Status, result.Summary, result.SourcesChecked)
	if len(result.Items) == 0 {
		md += "No research items collected.\n"
	}
	for i, item := range result.Items {
		md += fmt.Sprintf("%d) %s [%s]\n", i+1, item.Title, item.Risk)
		md += fmt.Sprintf("Source: %s\nURL: %s\n", item.Source, item.URL)
		if !item.PublishedAt.IsZero() {
			md += fmt.Sprintf("Published: %s\n", item.PublishedAt.Format("2006-01-02T15:04:05Z07:00"))
		}
		if len(item.Tags) > 0 {
			md += fmt.Sprintf("Tags: %v\n", item.Tags)
		}
		if item.Summary != "" {
			md += "Summary: " + item.Summary + "\n"
		}
		md += "\n"
	}
	if len(result.Warnings) > 0 {
		md += "Warnings:\n"
		for _, item := range result.Warnings {
			md += "- " + item + "\n"
		}
	}
	md += "\nResearch-only: no orders placed, no live safety gate changed, no override of Agent 1/2.\n"
	return md
}
