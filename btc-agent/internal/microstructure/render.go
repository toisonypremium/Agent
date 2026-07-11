package microstructure

import (
	"fmt"
	"strings"
)

func Markdown(s Summary) string {
	var b strings.Builder
	b.WriteString("MICROSTRUCTURE REPORT\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n", s.GeneratedAt.Format("2006-01-02T15:04:05Z07:00")))
	b.WriteString(fmt.Sprintf("Status: %s\n", s.Status))
	b.WriteString("Summary: " + s.Summary + "\n")
	b.WriteString(fmt.Sprintf("Fingerprint: %s\n\n", s.Fingerprint))
	if len(s.Blockers) > 0 {
		b.WriteString("Blockers:\n")
		for _, blocker := range s.Blockers {
			b.WriteString("- " + blocker + "\n")
		}
		b.WriteString("\n")
	}
	if len(s.Warnings) > 0 {
		b.WriteString("Warnings:\n")
		for _, warning := range s.Warnings {
			b.WriteString("- " + warning + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Snapshots:\n")
	if len(s.Snapshots) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, snap := range s.Snapshots {
			b.WriteString(fmt.Sprintf("- %s fresh=%v age=%s taker_buy=%.1f%% cvd=%.2f spread=%.2fbps ob=%s funding=%.4f basis=%.2f%% supportive=%v risky=%v\n", snap.Symbol, snap.Health.Fresh, snap.Health.Age.Round(0), snap.SpotFlow.TakerBuyRatio*100, snap.SpotFlow.CVDQuoteUSDT, snap.OrderBook.SpreadBps, snap.Signals.OrderBookBias, snap.Futures.FundingRate, snap.Futures.BasisPct, snap.Signals.Supportive, snap.Signals.Risky))
		}
	}
	b.WriteString("\nSafety: report-only; stale/missing microstructure can only reduce permission, never place orders. No futures execution.\n")
	return b.String()
}
