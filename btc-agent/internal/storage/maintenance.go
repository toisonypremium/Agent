package storage

import (
	"database/sql"
	"fmt"
	"time"
)

const (
	DefaultReportRetentionDays  = 30
	DefaultEventRetentionDays   = 90
	DefaultMaxReportFiles       = 50
	DefaultMaxClosedPaperOrders = 500
)

type MaintenanceConfig struct {
	ReportRetentionDays  int `json:"report_retention_days"`
	EventRetentionDays   int `json:"event_retention_days"`
	MaxReportFiles       int `json:"max_report_files"`
	MaxClosedPaperOrders int `json:"max_closed_paper_orders"`
}

type MaintenanceResult struct {
	Enabled                   bool              `json:"enabled"`
	GeneratedAt               time.Time         `json:"generated_at"`
	Config                    MaintenanceConfig `json:"config"`
	ReportsDeleted            int64             `json:"reports_deleted"`
	LiveOrderEventsDeleted    int64             `json:"live_order_events_deleted"`
	LivePositionEventsDeleted int64             `json:"live_position_events_deleted"`
	ClosedPaperOrdersDeleted  int64             `json:"closed_paper_orders_deleted"`
	ReportFilesDeleted        int               `json:"report_files_deleted"`
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

	closedOrdersDeleted, err := pruneClosedPaperOrders(d.DB, cfg.MaxClosedPaperOrders)
	if err != nil {
		return result, fmt.Errorf("prune closed paper orders: %w", err)
	}
	result.ClosedPaperOrdersDeleted = closedOrdersDeleted
	result.Summary = maintenanceSummary(result)
	return result, nil
}

func execRows(db *sql.DB, query string, args ...any) (int64, error) {
	res, err := db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
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

func maintenanceSummary(r MaintenanceResult) string {
	if !r.Enabled {
		return "Maintenance disabled"
	}
	return fmt.Sprintf("Maintenance deleted reports=%d live_order_events=%d live_position_events=%d closed_paper_orders=%d report_files=%d", r.ReportsDeleted, r.LiveOrderEventsDeleted, r.LivePositionEventsDeleted, r.ClosedPaperOrdersDeleted, r.ReportFilesDeleted)
}

func (r *MaintenanceResult) RefreshSummary() {
	r.Summary = maintenanceSummary(*r)
}
