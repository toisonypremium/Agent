package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/reportio"
)

type FilterAttributionReport struct {
	GeneratedAt    time.Time                         `json:"generated_at"`
	PlanState      agent2.State                      `json:"plan_state,omitempty"`
	BTCPermission  string                            `json:"btc_permission,omitempty"`
	Aggregate      []FilterAttributionAggregateRow   `json:"aggregate,omitempty"`
	Coins          []FilterAttributionCoinRow        `json:"coins,omitempty"`
	NearActionable []FilterAttributionNearActionable `json:"near_actionable,omitempty"`
	Summary        string                            `json:"summary"`
	Safety         string                            `json:"safety"`
}

type FilterAttributionAggregateRow struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type FilterAttributionCoinRow struct {
	Symbol         string                         `json:"symbol"`
	State          agent2.State                   `json:"state"`
	ReadinessScore float64                        `json:"readiness_score,omitempty"`
	SetupScore     float64                        `json:"setup_score,omitempty"`
	TopBlockerKey  string                         `json:"top_blocker_key,omitempty"`
	TopBlocker     string                         `json:"top_blocker,omitempty"`
	FailedHard     int                            `json:"failed_hard"`
	FailedSoft     int                            `json:"failed_soft"`
	DesiredLayers  int                            `json:"desired_layers"`
	WhyNoOrder     []string                       `json:"why_no_order,omitempty"`
	NextTrigger    string                         `json:"next_trigger,omitempty"`
	GateRows       []agent2.FilterAttributionGate `json:"gate_rows,omitempty"`
}

type FilterAttributionNearActionable struct {
	Symbol        string       `json:"symbol"`
	State         agent2.State `json:"state"`
	SetupScore    float64      `json:"setup_score,omitempty"`
	TopBlockerKey string       `json:"top_blocker_key,omitempty"`
	TopBlocker    string       `json:"top_blocker,omitempty"`
	NextTrigger   string       `json:"next_trigger,omitempty"`
}

func writeFilterAttributionReportFromManaged(result liveguard.ManagedCycleResult) error {
	return writeFilterAttributionReportFile(buildFilterAttributionReportFromManaged(result))
}

func writeFilterAttributionReportFile(report FilterAttributionReport) error {
	if err := reportio.WriteJSON("reports", "filter_attribution_latest.json", report); err != nil {
		return err
	}
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join("reports", "filter_attribution_latest.md"), []byte(filterAttributionMarkdown(report)), 0600)
}

func buildFilterAttributionReport(snapshot BotRuntimeSnapshot) FilterAttributionReport {
	report := FilterAttributionReport{GeneratedAt: snapshot.GeneratedAt, PlanState: snapshot.PlanState, BTCPermission: string(snapshot.BTCPermission), Safety: safetyLine}
	coinBySymbol := map[string]liveguard.ManagedCoinSummary{}
	for _, coin := range snapshot.PerCoin {
		coinBySymbol[strings.ToUpper(coin.Symbol)] = coin
	}
	counts := map[string]int{}
	for _, asset := range snapshot.Plan.Assets {
		attr := agent2.BuildFilterAttribution(asset)
		coin := coinBySymbol[strings.ToUpper(asset.Symbol)]
		why := coin.WhyNoOrder
		if len(why) == 0 {
			why = append(append([]string{}, asset.HardBlockers...), asset.SoftBlockers...)
			why = agent2.CompactReasons(why, 6)
		}
		row := FilterAttributionCoinRow{Symbol: asset.Symbol, State: asset.State, ReadinessScore: coin.ReadinessScore, SetupScore: attr.SetupScore, TopBlockerKey: attr.TopBlockerKey, TopBlocker: attr.TopBlocker, FailedHard: attr.FailedHard, FailedSoft: attr.FailedSoft, DesiredLayers: coin.DesiredLayers, WhyNoOrder: why, NextTrigger: firstNonEmpty(asset.NextTrigger, coin.NextTrigger), GateRows: attr.GateRows}
		report.Coins = append(report.Coins, row)
		if attr.TopBlockerKey != "" {
			counts[attr.TopBlockerKey]++
		}
		if attr.FailedHard == 0 && attr.FailedSoft > 0 && attr.SetupScore >= nearTriggerReadinessThreshold() {
			report.NearActionable = append(report.NearActionable, FilterAttributionNearActionable{Symbol: asset.Symbol, State: asset.State, SetupScore: attr.SetupScore, TopBlockerKey: attr.TopBlockerKey, TopBlocker: attr.TopBlocker, NextTrigger: firstNonEmpty(asset.NextTrigger, coin.NextTrigger)})
		}
	}
	for key, count := range counts {
		report.Aggregate = append(report.Aggregate, FilterAttributionAggregateRow{Key: key, Count: count})
	}
	sortFilterAggregates(report.Aggregate)
	report.Summary = fmt.Sprintf("Filter attribution coins=%d near_actionable=%d top_blocker=%s", len(report.Coins), len(report.NearActionable), topFilterKey(report.Aggregate))
	return report
}

func buildFilterAttributionReportFromManaged(result liveguard.ManagedCycleResult) FilterAttributionReport {
	report := FilterAttributionReport{GeneratedAt: result.GeneratedAt, PlanState: result.PlanState, Safety: safetyLine}
	counts := map[string]int{}
	for _, coin := range result.PerCoin {
		attr := coin.FilterAttribution
		row := FilterAttributionCoinRow{Symbol: coin.Symbol, State: coin.State, ReadinessScore: coin.ReadinessScore, SetupScore: attr.SetupScore, TopBlockerKey: firstNonEmpty(coin.TopFilterBlockerKey, attr.TopBlockerKey), TopBlocker: firstNonEmpty(coin.TopFilterBlocker, attr.TopBlocker), FailedHard: attr.FailedHard, FailedSoft: attr.FailedSoft, DesiredLayers: coin.DesiredLayers, WhyNoOrder: coin.WhyNoOrder, NextTrigger: coin.NextTrigger, GateRows: attr.GateRows}
		report.Coins = append(report.Coins, row)
		if row.TopBlockerKey != "" {
			counts[row.TopBlockerKey]++
		}
		if attr.FailedHard == 0 && attr.FailedSoft > 0 && attr.SetupScore >= nearTriggerReadinessThreshold() {
			report.NearActionable = append(report.NearActionable, FilterAttributionNearActionable{Symbol: coin.Symbol, State: coin.State, SetupScore: attr.SetupScore, TopBlockerKey: row.TopBlockerKey, TopBlocker: row.TopBlocker, NextTrigger: coin.NextTrigger})
		}
	}
	for key, count := range counts {
		report.Aggregate = append(report.Aggregate, FilterAttributionAggregateRow{Key: key, Count: count})
	}
	sortFilterAggregates(report.Aggregate)
	report.Summary = fmt.Sprintf("Filter attribution coins=%d near_actionable=%d top_blocker=%s", len(report.Coins), len(report.NearActionable), topFilterKey(report.Aggregate))
	return report
}

func filterAttributionMarkdown(report FilterAttributionReport) string {
	var b strings.Builder
	b.WriteString("FILTER ATTRIBUTION SNAPSHOT\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n", report.GeneratedAt.Format("2006-01-02T15:04:05Z07:00")))
	b.WriteString(fmt.Sprintf("Summary: %s\n", report.Summary))
	b.WriteString(fmt.Sprintf("Plan state: %s | BTC permission: %s\n\n", report.PlanState, report.BTCPermission))
	if len(report.Aggregate) > 0 {
		b.WriteString("Aggregate blockers:\n")
		for _, row := range report.Aggregate {
			b.WriteString(fmt.Sprintf("- %s: %d\n", row.Key, row.Count))
		}
		b.WriteString("\n")
	}
	if len(report.NearActionable) > 0 {
		b.WriteString("Near-actionable research watchlist — no order authority changed:\n")
		for _, row := range report.NearActionable {
			b.WriteString(fmt.Sprintf("- %s state=%s score=%.2f top=%s next=%s\n", row.Symbol, row.State, row.SetupScore, emptyStringDefault(row.TopBlockerKey, "n/a"), emptyStringDefault(row.NextTrigger, "chờ trigger rõ hơn")))
		}
		b.WriteString("\n")
	}
	b.WriteString("Per coin:\n")
	for _, row := range report.Coins {
		b.WriteString(fmt.Sprintf("- %s state=%s setup=%.2f readiness=%.2f desired=%d top=%s hard=%d soft=%d\n", row.Symbol, row.State, row.SetupScore, row.ReadinessScore, row.DesiredLayers, emptyStringDefault(row.TopBlockerKey, "none"), row.FailedHard, row.FailedSoft))
		if len(row.WhyNoOrder) > 0 {
			b.WriteString("  why=" + strings.Join(firstStrings(row.WhyNoOrder, 3), "; ") + "\n")
		}
		if row.NextTrigger != "" {
			b.WriteString("  next=" + row.NextTrigger + "\n")
		}
	}
	b.WriteString("\nSafety: " + report.Safety + "\n")
	b.WriteString("Research only: report đo filter; không bypass ACTIVE_LIMIT; WATCH/SCOUT/ARMED không tạo normal live order.\n")
	return b.String()
}

func sortFilterAggregates(rows []FilterAttributionAggregateRow) {
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[j].Count > rows[i].Count || (rows[j].Count == rows[i].Count && agent2.ReasonPriority(rows[j].Key) < agent2.ReasonPriority(rows[i].Key)) {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
}

func topFilterKey(rows []FilterAttributionAggregateRow) string {
	if len(rows) == 0 {
		return "none"
	}
	return rows[0].Key
}
