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
			b.WriteString(fmt.Sprintf("  research-only liquidation=%s avwap=%.8f profile_poc=%.8f\n", snap.Research.LiquidationProxy.Direction, snap.Research.AnchoredVWAP, snap.Research.VolumeProfile.POC))
		}
	}
	b.WriteString("\nSafety: microstructure feeds Hermes confidence sizing; stale/missing data blocks new exposure. No futures execution.\n")
	if len(s.MMFootprint) > 0 {
		b.WriteString(FootprintMarkdown(s.MMFootprint))
	}
	return b.String()
}

// FootprintMarkdown renders MM footprint signals as a markdown section.
func FootprintMarkdown(fp map[string]MMFootprintSignal) string {
	if len(fp) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nMM FOOTPRINT ANALYSIS\n")
	for sym, sig := range fp {
		b.WriteString(fmt.Sprintf("- %s verdict=%s score=%.2f snapshots=%d\n",
			sym, sig.Verdict, sig.FootprintScore, sig.SnapshotCount))
		b.WriteString(fmt.Sprintf("  cvd_slope=%.0f price_delta=%.2f%% cvd_divergence=%v taker_anomaly=%v bid_streak=%d funding_ok=%v\n",
			sig.CVDSlope, sig.PriceDeltaPct, sig.CVDPriceDivergence, sig.TakerBuyAnomaly,
			sig.BidSupportStreak, sig.FundingFavorable))
		for _, r := range sig.Reasons {
			b.WriteString("  → " + r + "\n")
		}
	}
	return b.String()
}
