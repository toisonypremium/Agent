package paper

import (
	"btc-agent/internal/agent2"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Scorecard struct {
	GeneratedAt       time.Time         `json:"generated_at"`
	TotalOrders       int               `json:"total_orders"`
	OpenOrders        int               `json:"open_orders"`
	FilledOrders      int               `json:"filled_orders"`
	InvalidatedOrders int               `json:"invalidated_orders"`
	ExpiredOrders     int               `json:"expired_orders"`
	CancelledOrders   int               `json:"cancelled_orders"`
	TerminalOrders    int               `json:"terminal_orders"`
	FillRate          float64           `json:"fill_rate"`
	InvalidationRate  float64           `json:"invalidation_rate"`
	AverageOpenAge    time.Duration     `json:"average_open_age"`
	BySymbol          []SymbolScorecard `json:"by_symbol"`
	Readiness         string            `json:"readiness"`
	Blockers          []string          `json:"blockers"`
	Safety            string            `json:"safety"`
}
type SymbolScorecard struct {
	Symbol      string `json:"symbol"`
	Total       int    `json:"total"`
	Open        int    `json:"open"`
	Filled      int    `json:"filled"`
	Invalidated int    `json:"invalidated"`
	Expired     int    `json:"expired"`
	Cancelled   int    `json:"cancelled"`
}

func BuildScorecard(now time.Time, orders []agent2.PaperOrder) Scorecard {
	r := Scorecard{GeneratedAt: now, BySymbol: []SymbolScorecard{}, Safety: "Paper evidence only; no real order was placed, canceled, or authorized."}
	by := map[string]*SymbolScorecard{}
	var age time.Duration
	for _, o := range orders {
		r.TotalOrders++
		sym := strings.ToUpper(strings.TrimSpace(o.Symbol))
		if sym == "" {
			sym = "UNKNOWN"
		}
		row := by[sym]
		if row == nil {
			row = &SymbolScorecard{Symbol: sym}
			by[sym] = row
		}
		row.Total++
		switch strings.ToUpper(o.Status) {
		case StatusOpen:
			r.OpenOrders++
			row.Open++
			if !o.Timestamp.IsZero() && now.After(o.Timestamp) {
				age += now.Sub(o.Timestamp)
			}
		case StatusFilled:
			r.FilledOrders++
			r.TerminalOrders++
			row.Filled++
		case StatusInvalidated:
			r.InvalidatedOrders++
			r.TerminalOrders++
			row.Invalidated++
		case StatusExpired:
			r.ExpiredOrders++
			r.TerminalOrders++
			row.Expired++
		case StatusCancelled:
			r.CancelledOrders++
			r.TerminalOrders++
			row.Cancelled++
		default:
			r.Blockers = append(r.Blockers, fmt.Sprintf("unknown paper status %q", o.Status))
		}
	}
	if r.OpenOrders > 0 {
		r.AverageOpenAge = age / time.Duration(r.OpenOrders)
	}
	if r.TerminalOrders > 0 {
		r.FillRate = float64(r.FilledOrders) / float64(r.TerminalOrders)
		r.InvalidationRate = float64(r.InvalidatedOrders) / float64(r.TerminalOrders)
	}
	for _, row := range by {
		r.BySymbol = append(r.BySymbol, *row)
	}
	sort.Slice(r.BySymbol, func(i, j int) bool { return r.BySymbol[i].Symbol < r.BySymbol[j].Symbol })
	if r.TotalOrders == 0 {
		r.Readiness = "INSUFFICIENT_EVIDENCE"
		r.Blockers = append(r.Blockers, "no paper orders recorded")
	} else if r.TerminalOrders == 0 {
		r.Readiness = "INSUFFICIENT_EVIDENCE"
		r.Blockers = append(r.Blockers, "no terminal paper outcomes recorded")
	} else {
		r.Readiness = "PAPER_EVIDENCE_ONLY"
		r.Blockers = append(r.Blockers, "paper lifecycle metrics do not prove real-exchange execution quality")
	}
	return r
}
func ScorecardMarkdown(r Scorecard) string {
	var b strings.Builder
	fmt.Fprintf(&b, "PAPER PERFORMANCE SCORECARD\n\nGenerated: %s\nReadiness: %s\nOrders: total=%d terminal=%d open=%d filled=%d invalidated=%d expired=%d cancelled=%d\nLifecycle: fill_rate=%.1f%% invalidation_rate=%.1f%% avg_open_age=%s\n", r.GeneratedAt.UTC().Format(time.RFC3339), r.Readiness, r.TotalOrders, r.TerminalOrders, r.OpenOrders, r.FilledOrders, r.InvalidatedOrders, r.ExpiredOrders, r.CancelledOrders, r.FillRate*100, r.InvalidationRate*100, r.AverageOpenAge.Round(time.Second))
	for _, x := range r.BySymbol {
		fmt.Fprintf(&b, "- %s total=%d open=%d filled=%d invalidated=%d expired=%d cancelled=%d\n", x.Symbol, x.Total, x.Open, x.Filled, x.Invalidated, x.Expired, x.Cancelled)
	}
	for _, x := range r.Blockers {
		b.WriteString("- blocker: " + x + "\n")
	}
	b.WriteString("Safety: " + r.Safety + "\n")
	return b.String()
}
