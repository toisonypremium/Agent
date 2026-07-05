package research

import "time"

const (
	StatusOK    = "RESEARCH_OK"
	StatusWarn  = "RESEARCH_WARN"
	StatusBlock = "RESEARCH_BLOCK"

	BriefOK   = "RESEARCH_BRIEF_OK"
	BriefWarn = "RESEARCH_BRIEF_WARN"

	RiskInfo = "INFO"
	RiskWarn = "WARN"
)

type ChannelStatus struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Usable  bool   `json:"usable"`
	Checked int    `json:"checked"`
	Error   string `json:"error,omitempty"`
}

type DoctorResult struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Status      string          `json:"status"`
	Channels    []ChannelStatus `json:"channels,omitempty"`
	Blockers    []string        `json:"blockers,omitempty"`
	Warnings    []string        `json:"warnings,omitempty"`
	Summary     string          `json:"summary"`
}

type BriefResult struct {
	GeneratedAt    time.Time      `json:"generated_at"`
	Status         string         `json:"status"`
	SourcesChecked int            `json:"sources_checked"`
	Items          []ResearchItem `json:"items,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
	Summary        string         `json:"summary"`
}

type ResearchItem struct {
	Source      string    `json:"source"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"published_at,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Risk        string    `json:"risk"`
	Summary     string    `json:"summary,omitempty"`
}

func (r *DoctorResult) RefreshSummary() {
	if r.GeneratedAt.IsZero() {
		r.GeneratedAt = time.Now()
	}
	r.Blockers = uniqueStrings(r.Blockers)
	r.Warnings = uniqueStrings(r.Warnings)
	if len(r.Blockers) > 0 {
		r.Status = StatusBlock
	} else if len(r.Warnings) > 0 {
		r.Status = StatusWarn
	} else if r.Status == "" {
		r.Status = StatusOK
	}
	if r.Summary == "" {
		r.Summary = r.Status + ": channels=" + itoa(len(r.Channels)) + " blockers=" + itoa(len(r.Blockers)) + " warnings=" + itoa(len(r.Warnings))
	}
}

func (r *BriefResult) RefreshSummary() {
	if r.GeneratedAt.IsZero() {
		r.GeneratedAt = time.Now()
	}
	r.Warnings = uniqueStrings(r.Warnings)
	if len(r.Warnings) > 0 {
		r.Status = BriefWarn
	} else if r.Status == "" {
		r.Status = BriefOK
	}
	if r.Summary == "" {
		r.Summary = r.Status + ": sources=" + itoa(r.SourcesChecked) + " items=" + itoa(len(r.Items)) + " warnings=" + itoa(len(r.Warnings))
	}
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, item := range in {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	digits := []byte{}
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}
