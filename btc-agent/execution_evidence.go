package main

import (
	"btc-agent/internal/storage"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type horizonEvidence struct {
	HorizonMinutes    int     `json:"horizon_minutes"`
	Samples           int     `json:"samples"`
	AverageMarkoutPct float64 `json:"average_markout_pct"`
}
type versionEvidence struct {
	StrategyVersion string `json:"strategy_version"`
	ConfigHash      string `json:"config_hash"`
	Orders          int    `json:"orders"`
	Fills           int    `json:"fills"`
}
type executionEvidenceReport struct {
	GeneratedAt                                                           time.Time `json:"generated_at"`
	Status                                                                string    `json:"status"`
	TotalOrders, FilledOrders, PartialOrders, FillEvents                  int
	WeightedFillRatio, AveragePriceSlippagePct, AverageFillLatencySeconds float64
	ExpectedMarkouts, CompletedMarkouts, MarkoutBacklog                   int
	Markouts                                                              []horizonEvidence `json:"markouts"`
	ClosedSellFills                                                       int               `json:"closed_sell_fills"`
	RealizedPnL, RealizedExpectancy                                       float64
	Versions                                                              []versionEvidence `json:"versions"`
	UnversionedOrders                                                     int               `json:"unversioned_orders"`
	Limitations                                                           []string          `json:"limitations,omitempty"`
	Summary                                                               string            `json:"summary"`
}

func buildExecutionEvidenceReport(db *storage.DB, now time.Time) (executionEvidenceReport, error) {
	r := executionEvidenceReport{GeneratedAt: now, Status: "EVIDENCE_COLLECTING", Markouts: []horizonEvidence{}, Versions: []versionEvidence{}, Limitations: []string{}}
	if err := db.QueryRow(`SELECT COUNT(*),SUM(CASE WHEN status='FILLED' THEN 1 ELSE 0 END),SUM(CASE WHEN status IN ('PARTIAL_FILL','PARTIALLY_FILLED') THEN 1 ELSE 0 END) FROM live_orders WHERE source='HERMES_OPERATOR'`).Scan(&r.TotalOrders, &r.FilledOrders, &r.PartialOrders); err != nil {
		return r, err
	}
	var ordered, filled, slippage, latency float64
	if err := db.QueryRow(`SELECT COALESCE(SUM(o.quantity),0),COALESCE(SUM(MIN(o.quantity,f.filled_quantity)),0),COALESCE(AVG(CASE WHEN o.price>0 AND f.avg_price>0 THEN CASE WHEN UPPER(o.side)='BUY' THEN (f.avg_price-o.price)/o.price ELSE (o.price-f.avg_price)/o.price END END),0),COALESCE(AVG(CASE WHEN f.updated_at>=o.submitted_at THEN f.updated_at-o.submitted_at END),0),COUNT(f.client_order_id) FROM live_orders o LEFT JOIN live_fills f ON f.client_order_id=o.client_order_id WHERE o.source='HERMES_OPERATOR'`).Scan(&ordered, &filled, &slippage, &latency, &r.FillEvents); err != nil {
		return r, err
	}
	if ordered > 0 {
		r.WeightedFillRatio = filled / ordered
	}
	r.AveragePriceSlippagePct = slippage
	r.AverageFillLatencySeconds = latency
	rows, err := db.Query(`SELECT horizon_minutes,COUNT(*),AVG(markout_pct) FROM execution_markouts GROUP BY horizon_minutes ORDER BY horizon_minutes`)
	if err != nil {
		return r, err
	}
	for rows.Next() {
		var x horizonEvidence
		if err = rows.Scan(&x.HorizonMinutes, &x.Samples, &x.AverageMarkoutPct); err != nil {
			rows.Close()
			return r, err
		}
		r.Markouts = append(r.Markouts, x)
		r.CompletedMarkouts += x.Samples
	}
	if err = rows.Close(); err != nil {
		return r, err
	}
	var eligible int
	_ = db.QueryRow(`SELECT COUNT(*) FROM live_position_events e JOIN live_orders o ON o.client_order_id=e.client_order_id WHERE o.source='HERMES_OPERATOR' AND e.timestamp<=?`, now.Add(-time.Minute).Unix()).Scan(&eligible)
	for _, h := range []int{1, 5, 15, 60} {
		var n int
		_ = db.QueryRow(`SELECT COUNT(*) FROM live_position_events e JOIN live_orders o ON o.client_order_id=e.client_order_id WHERE o.source='HERMES_OPERATOR' AND e.timestamp<=?`, now.Add(-time.Duration(h)*time.Minute).Unix()).Scan(&n)
		r.ExpectedMarkouts += n
	}
	r.MarkoutBacklog = r.ExpectedMarkouts - r.CompletedMarkouts
	if r.MarkoutBacklog < 0 {
		r.MarkoutBacklog = 0
	}
	_ = eligible
	perf, err := db.HermesLossProtectionSnapshot(time.Unix(0, 0))
	if err != nil {
		return r, err
	}
	r.ClosedSellFills = perf.ClosedSellFills
	r.RealizedPnL = perf.RollingRealizedPnL
	closedCost := 0.0
	for _, p := range perf.BySymbol {
		closedCost += p.ClosedCost
	}
	if closedCost > 0 {
		r.RealizedExpectancy = r.RealizedPnL / closedCost
	}
	vr, err := db.Query(`SELECT COALESCE(NULLIF(strategy_version,''),'UNVERSIONED'),COALESCE(NULLIF(config_hash,''),'UNKNOWN'),COUNT(*),SUM(CASE WHEN status='FILLED' THEN 1 ELSE 0 END) FROM live_orders WHERE source='HERMES_OPERATOR' GROUP BY 1,2 ORDER BY 1,2`)
	if err != nil {
		return r, err
	}
	for vr.Next() {
		var x versionEvidence
		if err = vr.Scan(&x.StrategyVersion, &x.ConfigHash, &x.Orders, &x.Fills); err != nil {
			vr.Close()
			return r, err
		}
		r.Versions = append(r.Versions, x)
		if x.StrategyVersion == "UNVERSIONED" {
			r.UnversionedOrders += x.Orders
		}
	}
	if err = vr.Close(); err != nil {
		return r, err
	}
	if r.TotalOrders < 30 {
		r.Limitations = append(r.Limitations, "live sample below 30 orders; do not tune strategy")
	}
	if r.ClosedSellFills == 0 {
		r.Limitations = append(r.Limitations, "no realized Hermes exit outcome yet")
	}
	if r.UnversionedOrders > 0 {
		r.Limitations = append(r.Limitations, "historical orders predate strategy version persistence")
	}
	if r.MarkoutBacklog == 0 && r.CompletedMarkouts > 0 {
		r.Status = "EVIDENCE_HEALTHY"
	}
	r.Summary = fmt.Sprintf("%s: orders=%d fills=%d fill_ratio=%.1f%% partial=%d markouts=%d backlog=%d closed_exits=%d realized_pnl=%.4f unversioned=%d", r.Status, r.TotalOrders, r.FillEvents, r.WeightedFillRatio*100, r.PartialOrders, r.CompletedMarkouts, r.MarkoutBacklog, r.ClosedSellFills, r.RealizedPnL, r.UnversionedOrders)
	return r, nil
}

func saveExecutionEvidenceReport(db *storage.DB, now time.Time) error {
	r, err := buildExecutionEvidenceReport(db, now)
	if err != nil {
		return err
	}
	if err = saveJSONFile("reports", "execution_evidence_latest.json", r); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "EXECUTION EVIDENCE SCORECARD\n\nGenerated: %s\nStatus: %s\nSummary: %s\n\n", r.GeneratedAt.Format(time.RFC3339), r.Status, r.Summary)
	fmt.Fprintf(&b, "Fill quality: orders=%d fills=%d weighted_fill_ratio=%.2f%% partial_orders=%d avg_price_slippage=%.4f%% avg_fill_latency=%.1fs\n", r.TotalOrders, r.FillEvents, r.WeightedFillRatio*100, r.PartialOrders, r.AveragePriceSlippagePct*100, r.AverageFillLatencySeconds)
	fmt.Fprintf(&b, "Markouts: completed=%d expected=%d backlog=%d\n", r.CompletedMarkouts, r.ExpectedMarkouts, r.MarkoutBacklog)
	for _, x := range r.Markouts {
		fmt.Fprintf(&b, "- %dm samples=%d avg=%.4f%%\n", x.HorizonMinutes, x.Samples, x.AverageMarkoutPct*100)
	}
	fmt.Fprintf(&b, "Realized exits: closed_sell_fills=%d pnl=%.6f expectancy=%.4f%%\n", r.ClosedSellFills, r.RealizedPnL, r.RealizedExpectancy*100)
	sort.Slice(r.Versions, func(i, j int) bool { return r.Versions[i].StrategyVersion < r.Versions[j].StrategyVersion })
	for _, x := range r.Versions {
		fmt.Fprintf(&b, "Version: %s config=%s orders=%d fills=%d\n", x.StrategyVersion, x.ConfigHash, x.Orders, x.Fills)
	}
	for _, x := range r.Limitations {
		fmt.Fprintf(&b, "- LIMITATION: %s\n", x)
	}
	if err = os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join("reports", "execution_evidence_latest.md"), []byte(b.String()), 0600); err != nil {
		return err
	}
	payload, _ := json.Marshal(r)
	_, err = db.Exec(`INSERT INTO hermes_runtime_state(key,updated_at,payload_json) VALUES('execution_evidence',?,?) ON CONFLICT(key) DO UPDATE SET updated_at=excluded.updated_at,payload_json=excluded.payload_json`, now.Unix(), string(payload))
	return err
}

var _ = sql.ErrNoRows
