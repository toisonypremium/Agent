package main

import (
	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/opsplan"
	"btc-agent/internal/reportio"
	"btc-agent/internal/storage"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func loadOperationsExposure(cfg config.Config, db *storage.DB) (opsplan.ExposureSnapshot, error) {
	exposure := opsplan.ExposureSnapshot{Assets: map[string]opsplan.AssetExposure{}, Source: "no live exposure (paper/report mode)"}
	if !cfg.Live.Enabled && !cfg.Execution.RealTradingEnabled {
		return exposure, nil
	}
	exposure.Source = "live position ledger + open live orders"
	positions, err := db.LivePositions()
	if err != nil {
		return exposure, fmt.Errorf("load live positions for operations plan: %w", err)
	}
	for _, position := range positions {
		symbol := strings.ToUpper(position.Symbol)
		if symbol == "" {
			symbol = strings.ToUpper(live.InternalSymbol(position.InstID))
		}
		cost := math.Max(0, position.CostBasis)
		item := exposure.Assets[symbol]
		item.PositionCostUSDT += cost
		exposure.Assets[symbol] = item
		exposure.PositionCostUSDT += cost
	}
	orders, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		return exposure, fmt.Errorf("load open live orders for operations plan: %w", err)
	}
	for _, order := range orders {
		symbol := strings.ToUpper(order.Symbol)
		if symbol == "" {
			symbol = strings.ToUpper(live.InternalSymbol(order.InstID))
		}
		notional := math.Max(0, order.Notional)
		if notional == 0 && order.Price > 0 && order.Quantity > 0 {
			notional = order.Price * order.Quantity
		}
		item := exposure.Assets[symbol]
		item.OpenOrderNotionalUSDT += notional
		exposure.Assets[symbol] = item
		exposure.OpenOrderNotionalUSDT += notional
	}
	return exposure, nil
}

func saveOperationsPlan(cfg config.Config, db *storage.DB, analysis agent1.MarketAnalysis, p agent2.Plan) (opsplan.Report, error) {
	exposure, err := loadOperationsExposure(cfg, db)
	if err != nil {
		return opsplan.Report{}, err
	}
	report := opsplan.Build(cfg, analysis, p, exposure)
	if err := reportio.WriteJSON("reports", "operations_plan_latest.json", report); err != nil {
		return report, err
	}
	if err := reportio.WriteMarkdown("reports", "operations_plan_latest.md", opsplan.Markdown(report)); err != nil {
		return report, err
	}
	return report, nil
}

func runOperationsPlan(cfg config.Config, db *storage.DB) error {
	analysis, err := db.LatestAnalysis()
	if err != nil {
		return fmt.Errorf("load latest analysis: %w", err)
	}
	p, err := db.LatestPlan()
	if err != nil {
		return fmt.Errorf("load latest plan: %w", err)
	}
	report, err := saveOperationsPlan(cfg, db, analysis, p)
	if err != nil {
		return err
	}
	fmt.Println(opsplan.Markdown(report))
	return nil
}

type marketWatchState struct {
	Fingerprint                   string    `json:"fingerprint"`
	NotifiedFingerprint           string    `json:"notified_fingerprint,omitempty"`
	MarketNotifiedFingerprint     string    `json:"market_notified_fingerprint,omitempty"`
	CriticalNotifiedFingerprint   string    `json:"critical_notified_fingerprint,omitempty"`
	NearUnlockNotifiedFingerprint string    `json:"near_unlock_notified_fingerprint,omitempty"`
	RealReadyNotifiedFingerprint  string    `json:"real_ready_notified_fingerprint,omitempty"`
	UpdatedAt                     time.Time `json:"updated_at"`
	LastCriticalAt                time.Time `json:"last_critical_at,omitempty"`
}

func loadMarketWatchState() marketWatchState {
	var state marketWatchState
	b, err := os.ReadFile(filepath.Join("reports", "market_watch_state.json"))
	if err == nil {
		_ = json.Unmarshal(b, &state)
	}
	return state
}

func runMarketWatch(ctx context.Context, cfg config.Config, db *storage.DB, notifyStateChange bool) (opsplan.Report, error) {
	if err := fetch(ctx, cfg, db); err != nil {
		return opsplan.Report{}, err
	}
	if cfg.Microstructure.Enabled && cfg.Microstructure.FetchOnMarketWatch {
		if summary, err := fetchMicrostructureSummary(ctx, cfg, db); err != nil {
			saveRuntimeEventJSON(db, "microstructure", "MICROSTRUCTURE_FETCH_ERROR", "warning", "fetch-error", map[string]any{"error": err.Error()})
		} else {
			_ = writeMicrostructureReport(summary)
			saveMicrostructureRuntimeEvents(db, summary)
		}
	}
	analysis, err := analyze(ctx, cfg, db)
	if err != nil {
		return opsplan.Report{}, err
	}
	p, err := monitorPlan(cfg, db)
	if err != nil {
		return opsplan.Report{}, err
	}
	marketReport := agent1.DailyReport(analysis, agent2.Summary(p))
	if err := db.SaveReport("market_watch", marketReport); err != nil {
		return opsplan.Report{}, err
	}
	if err := storage.SaveReportFiles("reports", analysis, p, marketReport); err != nil {
		return opsplan.Report{}, err
	}
	report, err := saveOperationsPlan(cfg, db, analysis, p)
	if err != nil {
		return report, err
	}

	previous := loadMarketWatchState()
	changed := previous.Fingerprint == "" || previous.Fingerprint != report.Fingerprint
	marketNotified := firstNonEmpty(previous.MarketNotifiedFingerprint, previous.NotifiedFingerprint)
	notificationDue := marketNotified != report.Fingerprint
	now := time.Now().UTC()
	criticalDue := false
	if report.Market.Urgency == opsplan.UrgencyRiskAlert && cfg.Monitoring.NotifyOnCritical {
		repeat := time.Duration(report.Monitoring.CriticalRepeatMinutes) * time.Minute
		criticalDue = notificationDue || changed || previous.LastCriticalAt.IsZero() || now.Sub(previous.LastCriticalAt) >= repeat
	}
	saveMarketWatchRuntimeEvents(db, report, changed, criticalDue)
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		switch {
		case criticalDue:
			sendScheduledTelegram(ctx, cfg, "market-critical", opsplan.CriticalTelegram(report))
			previous.LastCriticalAt = now
			previous.CriticalNotifiedFingerprint = "critical:" + report.Fingerprint
		case notifyStateChange && cfg.Monitoring.NotifyOnStateChange && liveAutoNearUnlockTelegram(report) != "" && previous.NearUnlockNotifiedFingerprint != liveAutoNearUnlockFingerprint(report):
			sendScheduledTelegram(ctx, cfg, "live-auto-near-unlock", liveAutoNearUnlockTelegram(report))
			previous.NearUnlockNotifiedFingerprint = liveAutoNearUnlockFingerprint(report)
		case notifyStateChange && cfg.Monitoring.NotifyOnStateChange && notificationDue:
			sendScheduledTelegram(ctx, cfg, "market-state", opsplan.TelegramDigest(report))
			previous.MarketNotifiedFingerprint = report.Fingerprint
			previous.NotifiedFingerprint = report.Fingerprint
		}
	}
	previous.Fingerprint = report.Fingerprint
	previous.UpdatedAt = now
	if err := reportio.WriteJSON("reports", "market_watch_state.json", previous); err != nil {
		return report, err
	}
	return report, nil
}
