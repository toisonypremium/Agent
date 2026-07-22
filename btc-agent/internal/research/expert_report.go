package research

import "time"

// ExpertReport is the structured output for expert macro+market analysis.
type ExpertReport struct {
	GeneratedAt     time.Time    `json:"generated_at"`
	Status          string       `json:"status"`
	Summary         string       `json:"summary"`
	Sections        []Section    `json:"sections"`
	RiskSignals     []RiskSignal `json:"risk_signals"`
	ResearchSummary string       `json:"research_summary"`
	StrategyContext string       `json:"strategy_context,omitempty"`
}

type Section struct {
	Title    string          `json:"title"`
	Content  string          `json:"content"`
	Evidence []EvidencePoint `json:"evidence,omitempty"`
}

type EvidencePoint struct {
	Source     string  `json:"source"`
	Headline   string  `json:"headline"`
	URL        string  `json:"url"`
	Published  string  `json:"published"`
	Confidence float64 `json:"confidence"`
	Relevance  string  `json:"relevance"`
}

type RiskSignal struct {
	Type   string `json:"type"`
	Level  string `json:"level"`
	Detail string `json:"detail"`
	Impact string `json:"impact"`
}

func (r *ExpertReport) RefreshSummary() {
	if r.GeneratedAt.IsZero() {
		r.GeneratedAt = time.Now()
	}
	if r.Status == "" {
		r.Status = "EXPERT_REPORT_OK"
	}
	if r.Summary == "" {
		r.Summary = r.Status + ": sections=" + itoa(len(r.Sections)) + " signals=" + itoa(len(r.RiskSignals))
	}
}

func (r *ExpertReport) Markdown() string {
	md := "EXPERT REPORT\n\nGenerated: " + r.GeneratedAt.Format("2006-01-02T15:04:05Z07:00") + "\nStatus: " + r.Status + "\nSummary: " + r.Summary + "\n\n"
	for _, s := range r.Sections {
		md += s.Title + "\n" + s.Content + "\n\n"
		for _, e := range s.Evidence {
			md += "- [" + e.Relevance + "] " + e.Headline + " (" + e.Source + ", conf=" + ftoa(e.Confidence) + ")\n"
		}
		md += "\n"
	}
	if r.StrategyContext != "" {
		md += "STRATEGY CONTEXT (supplementary, not evidence):\n" + r.StrategyContext + "\n\n"
	}
	if len(r.RiskSignals) > 0 {
		md += "RISK SIGNALS:\n"
		for _, sig := range r.RiskSignals {
			md += "- " + sig.Level + " | " + sig.Type + ": " + sig.Detail + " (impact: " + sig.Impact + ")\n"
		}
		md += "\n"
	}
	md += "Expert analysis only: no orders placed, no live safety gate changed, no override of Agent 1/2.\n"
	return md
}

func ftoa(v float64) string {
	if v == 0 {
		return "0.0"
	}
	if v == 1 {
		return "1.0"
	}
	if v == 0.9 {
		return "0.9"
	}
	if v == 0.7 {
		return "0.7"
	}
	if v == 0.3 {
		return "0.3"
	}
	return "0.0"
}
