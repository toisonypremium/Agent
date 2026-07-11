package storage

import (
	"database/sql"
	"fmt"
	"time"
)

const (
	DefaultReportRetentionDays         = 30
	DefaultEventRetentionDays          = 90
	DefaultMaxReportFiles              = 50
	DefaultMaxClosedPaperOrders        = 500
	DefaultMaxCandlesPerSymbolInterval = 1000
	DefaultMaxAnalysisRows             = 500
	DefaultMaxPlanRows                 = 500
	DefaultStaleOpenLiveOrderDays      = 7
)

type MaintenanceConfig struct {
	ReportRetentionDays         int  `json:"report_retention_days"`
	EventRetentionDays          int  `json:"event_retention_days"`
	MaxReportFiles              int  `json:"max_report_files"`
	MaxClosedPaperOrders        int  `json:"max_closed_paper_orders"`
	MaxCandlesPerSymbolInterval int  `json:"max_candles_per_symbol_interval"`
	MaxAnalysisRows             int  `json:"max_analysis_rows"`
	MaxPlanRows                 int  `json:"max_plan_rows"`
	WALCheckpoint               bool `json:"wal_checkpoint_on_maintenance"`
	// StaleOpenLiveOrderDays: expire LIVE_OPEN/SUBMITTED orders older than N days
	// that have no expires_at. 0 disables. Default 7.
	StaleOpenLiveOrderDays int `json:"stale_open_live_order_days"`
}

type MaintenanceResult struct {
	Enabled                   bool              `json:"enabled"`
	GeneratedAt               time.Time         `json:"generated_at"`
	Config                    MaintenanceConfig `json:"config"`
	ReportsDeleted            int64             `json:"reports_deleted"`
	LiveOrderEventsDeleted    int64             `json:"live_order_events_deleted"`
	LivePositionEventsDeleted int64             `json:"live_position_events_deleted"`
	RuntimeEventsDeleted      int64             `json:"runtime_events_deleted"`
	ClosedPaperOrdersDeleted  int64             `json:"closed_paper_orders_deleted"`
	CandlesDeleted            int64             `json:"candles_deleted"`
	AnalysesDeleted           int64             `json:"analyses_deleted"`
	PlansDeleted              int64             `json:"plans_deleted"`
	WALCheckpointed           bool              `json:"wal_checkpointed"`
	ReportFilesDeleted        int               `json:"report_files_deleted"`
	StaleOpenLiveOrdersClosed int64             `json:"stale_open_live_orders_closed"`
	Summary                   string            `json:"summary"`
}

func NormalizeMaintenanceConfig(cfg MaintenanceConfig) MaintenanceConfig {
	if cfg.ReportRetentionDays == 0 {
		cfg.ReportRetentionDays = DefaultReportRetentionDays
	}
	if cfg.EventRetentionDays == 0 {
		cfg.EventRetentionDays = DefaultEventRetentionDays
	}
	if cfg.MaxReportFiles == 0 {
		cfg.MaxReportFiles = DefaultMaxReportFiles
	}
	if cfg.MaxClosedPaperOrders == 0 {
		cfg.MaxClosedPaperOrders = DefaultMaxClosedPaperOrders
	}
	if cfg.MaxCandlesPerSymbolInterval == 0 {
		cfg.MaxCandlesPerSymbolInterval = DefaultMaxCandlesPerSymbolInterval
	}
	if cfg.MaxAnalysisRows == 0 {
		cfg.MaxAnalysisRows = DefaultMaxAnalysisRows
	}
	if cfg.MaxPlanRows == 0 {
		cfg.MaxPlanRows = DefaultMaxPlanRows
	}
	if cfg.StaleOpenLiveOrderDays == 0 {
		cfg.StaleOpenLiveOrderDays = DefaultStaleOpenLiveOrderDays
	}
	return cfg
}

func (d *DB) PruneMaintenance(cfg MaintenanceConfig, now time.Time) (MaintenanceResult, error) {
	cfg = NormalizeMaintenanceConfig(cfg)
	result := MaintenanceResult{Enabled: true, GeneratedAt: now, Config: cfg}

	reportCutoff := now.AddDate(0, 0, -cfg.ReportRetentionDays).Unix()
	reportsDeleted, err := execRows(d.DB, `DELETE FROM reports WHERE timestamp < ?`, reportCutoff)
	if err != nil {
		return result, fmt.Errorf("prune reports: %w", err)
	}
	result.ReportsDeleted = reportsDeleted

	eventCutoff := now.AddDate(0, 0, -cfg.EventRetentionDays).Unix()
	orderEventsDeleted, err := execRows(d.DB, `DELETE FROM live_order_events WHERE timestamp < ?`, eventCutoff)
	if err != nil {
		return result, fmt.Errorf("prune live order events: %w", err)
	}
	result.LiveOrderEventsDeleted = orderEventsDeleted

	positionEventsDeleted, err := execRows(d.DB, `DELETE FROM live_position_events WHERE timestamp < ?`, eventCutoff)
	if err != nil {
		return result, fmt.Errorf("prune live position events: %w", err)
	}
	result.LivePositionEventsDeleted = positionEventsDeleted

	runtimeEventsDeleted, err := execRows(d.DB, `DELETE FROM runtime_events WHERE timestamp < ?`, eventCutoff)
	if err != nil {
		return result, fmt.Errorf("prune runtime events: %w", err)
	}
	result.RuntimeEventsDeleted = runtimeEventsDeleted

	closedOrdersDeleted, err := pruneClosedPaperOrders(d.DB, cfg.MaxClosedPaperOrders)
	if err != nil {
		return result, fmt.Errorf("prune closed paper orders: %w", err)
	}
	result.ClosedPaperOrdersDeleted = closedOrdersDeleted

	candlesDeleted, err := pruneCandles(d.DB, cfg.MaxCandlesPerSymbolInterval)
	if err != nil {
		return result, fmt.Errorf("prune candles: %w", err)
	}
	result.CandlesDeleted = candlesDeleted

	analysesDeleted, err := pruneNewestRows(d.DB, "market_analyses", cfg.MaxAnalysisRows)
	if err != nil {
		return result, fmt.Errorf("prune analyses: %w", err)
	}
	result.AnalysesDeleted = analysesDeleted

	plansDeleted, err := pruneNewestRows(d.DB, "accumulation_plans", cfg.MaxPlanRows)
	if err != nil {
		return result, fmt.Errorf("prune plans: %w", err)
	}
	result.PlansDeleted = plansDeleted

	staleClosed, err := closeStaleOpenLiveOrders(d.DB, cfg.StaleOpenLiveOrderDays, now)
	if err != nil {
		return result, fmt.Errorf("close stale open live orders: %w", err)
	}
	result.StaleOpenLiveOrdersClosed = staleClosed

	if cfg.WALCheckpoint {
		if _, err := d.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
			return result, fmt.Errorf("wal checkpoint: %w", err)
		}
		result.WALCheckpointed = true
	}
	result.Summary = maintenanceSummary(result)
	return result, nil
}

func execRows(db *sql.DB, query string, args ...any) (int64, error) {
	var rows int64
	err := withSQLiteRetry(func() error {
		res, err := db.Exec(query, args...)
		if err != nil {
			return err
		}
		rows, err = res.RowsAffected()
		return err
	})
	return rows, err
}

func pruneClosedPaperOrders(db *sql.DB, keep int) (int64, error) {
	if keep < 0 {
		keep = 0
	}
	return execRows(db, `DELETE FROM paper_orders
		WHERE status <> 'OPEN'
		AND id NOT IN (
			SELECT id FROM paper_orders
			WHERE status <> 'OPEN'
			ORDER BY timestamp DESC, id DESC
			LIMIT ?
		)`, keep)
}

func pruneCandles(db *sql.DB, keep int) (int64, error) {
	if keep < 0 {
		keep = 0
	}
	return execRows(db, `DELETE FROM candles
		WHERE rowid IN (
			SELECT rowid FROM (
				SELECT rowid, ROW_NUMBER() OVER (PARTITION BY symbol, interval ORDER BY open_time DESC) AS rn
				FROM candles
			)
			WHERE rn > ?
		)`, keep)
}

func pruneNewestRows(db *sql.DB, table string, keep int) (int64, error) {
	if keep < 0 {
		keep = 0
	}
	return execRows(db, `DELETE FROM `+table+`
		WHERE id NOT IN (
			SELECT id FROM `+table+`
			ORDER BY id DESC
			LIMIT ?
		)`, keep)
}

func maintenanceSummary(r MaintenanceResult) string {
	if !r.Enabled {
		return "Maintenance disabled"
	}
	return fmt.Sprintf("Maintenance deleted reports=%d live_order_events=%d live_position_events=%d runtime_events=%d closed_paper_orders=%d candles=%d analyses=%d plans=%d report_files=%d stale_live_orders_closed=%d", r.ReportsDeleted, r.LiveOrderEventsDeleted, r.LivePositionEventsDeleted, r.RuntimeEventsDeleted, r.ClosedPaperOrdersDeleted, r.CandlesDeleted, r.AnalysesDeleted, r.PlansDeleted, r.ReportFilesDeleted, r.StaleOpenLiveOrdersClosed)
}

// closeStaleOpenLiveOrders marks LIVE_OPEN/SUBMITTED/PLANNED/PARTIAL_FILL orders as
// EXPIRED if they were submitted more than staledays ago and have no expires_at, or
// if their expires_at is before now. Safe to call every maintenance cycle.
// Only runs when staledays > 0.
func closeStaleOpenLiveOrders(db *sql.DB, staledays int, now time.Time) (int64, error) {
	if staledays <= 0 {
		return 0, nil
	}
	staleCutoff := now.AddDate(0, 0, -staledays).Unix()
	nowUnix := now.Unix()
	// Two conditions:
	// 1. Has expires_at > 0 and expires_at < now (order expired per its own timer).
	// 2. Has no expires_at (or expires_at=0) and submitted_at < stale cutoff (very old orphan).
	return execRows(db,
		`UPDATE live_orders
		 SET status='EXPIRED', updated_at=?, last_management_action='maintenance_stale_expire'
		 WHERE status IN ('PLANNED','SUBMITTED','PARTIAL_FILL','LIVE_OPEN','PARTIALLY_FILLED')
		 AND (
		   (expires_at IS NOT NULL AND expires_at > 0 AND expires_at < ?)
		   OR
		   ((expires_at IS NULL OR expires_at = 0) AND submitted_at IS NOT NULL AND submitted_at > 0 AND submitted_at < ?)
		 )`,
		nowUnix, nowUnix, staleCutoff,
	)
}

func (r *MaintenanceResult) RefreshSummary() {
	r.Summary = maintenanceSummary(*r)
}
