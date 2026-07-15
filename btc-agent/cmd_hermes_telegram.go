package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"btc-agent/internal/config"
	"btc-agent/internal/hermesagent"
)

func telegramCommandHermes(report hermesagent.HermesReport) string {
	if report.TelegramText != "" {
		return report.TelegramText
	}
	age := ""
	if !report.GeneratedAt.IsZero() {
		age = fmt.Sprintf(" (generated %s)", report.GeneratedAt.Format("15:04 MST"))
	}
	return fmt.Sprintf("HERMES BOT MANAGER%s\nGate: %s\nAssets: %s\nExits: %s\n%s",
		age, report.GateSummary, report.AssetSummary, report.ExitSummary, report.ActionLine)
}

func telegramCommandExits(snap hermesagent.HermesSnapshot) string {
	if len(snap.Exits) == 0 {
		return fmt.Sprintf("EXIT SIGNALS\n\nKhông có exit signal.\nPositions: %d\n\nREAD_ONLY — no order placed.", len(snap.Positions))
	}
	var b strings.Builder
	b.WriteString("EXIT SIGNALS\n\n")
	for _, ex := range snap.Exits {
		b.WriteString(fmt.Sprintf("- %s → %s PnL=%.2f%%\n  %s\n", ex.Symbol, ex.Action, ex.PnLPct*100, ex.Reason))
	}
	b.WriteString("\n⚠ Report-only. Operator review required trước khi execute.\nREAD_ONLY — no order placed.")
	return b.String()
}

func telegramCommandAudit() string {
	b, err := os.ReadFile(filepath.Join("reports", "live_auto_audit_latest.md"))
	if err != nil {
		return "Chưa có audit report. Chạy: ./bin/btc-agent live-auto-audit --config config.yaml"
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) > 25 {
		lines = lines[:25]
		lines = append(lines, "... (xem đầy đủ tại reports/live_auto_audit_latest.md)")
	}
	return strings.Join(lines, "\n")
}

// buildHermesSnapshotFromReports builds a minimal snapshot from report files only (no cfg needed).
// Used by Telegram command handlers that don't have cfg in scope.
func buildHermesSnapshotFromReports() hermesagent.HermesSnapshot {
	return buildHermesSnapshot(config.Config{})
}
