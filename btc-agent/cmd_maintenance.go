package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/reportio"
	"btc-agent/internal/storage"
)

func runMaintenance(cfg config.Config, db *storage.DB) error {
	mcfg := storage.MaintenanceConfig{
		ReportRetentionDays:         cfg.Maintenance.ReportRetentionDays,
		EventRetentionDays:          cfg.Maintenance.EventRetentionDays,
		MaxReportFiles:              cfg.Maintenance.MaxReportFiles,
		MaxClosedPaperOrders:        cfg.Maintenance.MaxClosedPaperOrders,
		MaxCandlesPerSymbolInterval: cfg.Maintenance.MaxCandlesPerSymbolInterval,
		MaxAnalysisRows:             cfg.Maintenance.MaxAnalysisRows,
		MaxPlanRows:                 cfg.Maintenance.MaxPlanRows,
		WALCheckpoint:               cfg.Maintenance.WALCheckpoint,
	}
	if !cfg.Maintenance.Enabled {
		result := storage.MaintenanceResult{Enabled: false, GeneratedAt: time.Now(), Config: storage.NormalizeMaintenanceConfig(mcfg)}
		result.RefreshSummary()
		fmt.Println(result.Summary)
		return nil
	}
	result, err := db.PruneMaintenance(mcfg, time.Now())
	if err != nil {
		return err
	}
	deletedFiles, err := storage.PruneReportFiles("reports", result.Config.MaxReportFiles, protectedReportFiles())
	if err != nil {
		return fmt.Errorf("prune report files: %w", err)
	}
	result.ReportFilesDeleted = deletedFiles
	result.RefreshSummary()
	if err := saveJSONFile("reports", "maintenance_latest.json", result); err != nil {
		return err
	}
	md := maintenanceMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "maintenance_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	fmt.Println(md)
	return nil
}

func protectedReportFiles() []string {
	return []string{
		"latest.md",
		"latest.json",
		"backtest_latest.md",
		"backtest_latest.json",
		"ai_eval_latest.md",
		"ai_eval_latest.json",
		"ai_watch_latest.md",
		"ai_watch_latest.json",
		"live_proof_latest.md",
		"live_proof_latest.json",
		"live_readiness_latest.md",
		"live_readiness_latest.json",
		"live_doctor_latest.md",
		"live_doctor_latest.json",
		"research_doctor_latest.md",
		"research_doctor_latest.json",
		"research_brief_latest.md",
		"research_brief_latest.json",
		"live_order_proof_latest.md",
		"live_order_proof_latest.json",
		"live_position_latest.md",
		"live_position_latest.json",
		"live_reconcile_latest.md",
		"live_reconcile_latest.json",
		"auto_live_management_latest.md",
		"auto_live_management_latest.json",
		"live_supervisor_latest.md",
		"live_supervisor_latest.json",
		"maintenance_latest.md",
		"maintenance_latest.json",
		"learning_latest.md",
		"learning_latest.json",
		"real_data_survey_latest.md",
		"real_data_survey_latest.json",
		"live_manager_history_latest.md",
		"live_manager_history_latest.json",
		"live_manager_simulation_latest.md",
		"live_manager_simulation_latest.json",
		"shadow_probe_latest.json",
		"shadow_probe_journal.jsonl",
		"technical_scorecard_latest.md",
		"technical_scorecard_latest.json",
		"capital_plan_research_latest.md",
		"capital_plan_research_latest.json",
		"coin_universe_research_latest.md",
		"coin_universe_research_latest.json",
		"decision_dashboard_latest.md",
		"decision_dashboard_latest.json",
		"cancel_all_live_orders_latest.md",
		"cancel_all_live_orders_latest.json",
		"telegram_state.json",
		"paper_manager_latest.md",
		"paper_manager_latest.json",
		"operations_plan_latest.md",
		"operations_plan_latest.json",
		"market_watch_state.json",
	}
}

func maintenanceMarkdown(result storage.MaintenanceResult) string {
	md := fmt.Sprintf("MAINTENANCE REPORT\n\nGenerated: %s\nSummary: %s\n\n", result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Summary)
	md += fmt.Sprintf("Retention: reports=%dd events=%dd max_report_files=%d max_closed_paper_orders=%d max_candles_per_pair=%d max_analysis_rows=%d max_plan_rows=%d wal_checkpoint=%v\n", result.Config.ReportRetentionDays, result.Config.EventRetentionDays, result.Config.MaxReportFiles, result.Config.MaxClosedPaperOrders, result.Config.MaxCandlesPerSymbolInterval, result.Config.MaxAnalysisRows, result.Config.MaxPlanRows, result.Config.WALCheckpoint)
	md += fmt.Sprintf("Deleted: reports=%d live_order_events=%d live_position_events=%d runtime_events=%d closed_paper_orders=%d candles=%d analyses=%d plans=%d report_files=%d\n", result.ReportsDeleted, result.LiveOrderEventsDeleted, result.LivePositionEventsDeleted, result.RuntimeEventsDeleted, result.ClosedPaperOrdersDeleted, result.CandlesDeleted, result.AnalysesDeleted, result.PlansDeleted, result.ReportFilesDeleted)
	if result.WALCheckpointed {
		md += "WAL checkpoint: done\n"
	}
	md += "Live orders, live fills, live positions, and operator settings were not pruned.\n"
	return md
}

func saveJSONFile(dir, name string, v any) error {
	return reportio.WriteJSON(dir, name, v)
}
