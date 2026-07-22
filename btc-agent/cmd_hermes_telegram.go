package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"btc-agent/internal/config"
	"btc-agent/internal/hermesagent"
	"btc-agent/internal/storage"
)

func telegramCommandHermes(report hermesagent.HermesReport) string {
	if report.TelegramText != "" {
		return report.TelegramText
	}
	age := ""
	if !report.GeneratedAt.IsZero() {
		age = fmt.Sprintf(" (generated %s)", report.GeneratedAt.Format("15:04 MST"))
	}
	return fmt.Sprintf("HERMES BOT MANAGER%s\nGate: %s\nAssets: %s\nExits: %s\n%s", age, report.GateSummary, report.AssetSummary, report.ExitSummary, report.ActionLine)
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

func parseTelegramHermesRequest(text string) (hermesagent.HermesTrigger, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return hermesagent.HermesTrigger{}, false
	}
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "/hermes") || strings.HasPrefix(lower, "/h") {
		parts := strings.Fields(text)
		question := ""
		if len(parts) > 1 {
			question = strings.Join(parts[1:], " ")
		}
		return hermesagent.HermesTrigger{Source: "telegram", Reason: "command", UserText: question, ForceReply: true, AllowNotify: true}, true
	}
	if strings.HasPrefix(lower, "/ask") {
		parts := strings.Fields(text)
		question := ""
		if len(parts) > 1 {
			question = strings.Join(parts[1:], " ")
		}
		return hermesagent.HermesTrigger{Source: "telegram", Reason: "ask", UserText: question, ForceReply: true, AllowNotify: true}, true
	}
	if strings.Contains(lower, "hermes") && strings.Contains(text, "?") {
		return hermesagent.HermesTrigger{Source: "telegram", Reason: "free-text", UserText: text, ForceReply: true, AllowNotify: true}, true
	}
	return hermesagent.HermesTrigger{}, false
}

func telegramCommandHermesFromLatest() string {
	report, ok := loadHermesReportFile()
	if !ok {
		return "Chua co Hermes report. Chay Hermes cycle hoac dung /ask de lenh anh xa ngay."
	}
	return telegramCommandHermes(report)
}

// buildHermesSnapshotFromReports builds a minimal snapshot from report files only.
func buildHermesSnapshotFromReports() hermesagent.HermesSnapshot {
	return buildHermesSnapshot(config.Config{})
}

// runHermesTelegramReply runs a Hermes cycle for an interactive Telegram trigger and returns the reply text.
func runHermesTelegramReply(ctx context.Context, cfg config.Config, db *storage.DB, trigger hermesagent.HermesTrigger) string {
	if err := runHermesCycleWithTrigger(ctx, cfg, db, trigger); err != nil {
		return fmt.Sprintf("Hermes cycle error: %v\nREAD_ONLY — no order placed.", err)
	}
	report, ok := loadHermesReportFile()
	if !ok {
		return "Hermes report unavailable.\nREAD_ONLY — no order placed."
	}
	return telegramCommandHermes(report)
}
