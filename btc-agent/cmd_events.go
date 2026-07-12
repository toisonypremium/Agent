package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/microstructure"
	"btc-agent/internal/opsplan"
	"btc-agent/internal/storage"
)

func runOpsEvents(cfg config.Config, db *storage.DB) error {
	events, err := db.PendingRuntimeEvents(50)
	if err != nil {
		return fmt.Errorf("load pending runtime events: %w", err)
	}
	fmt.Println(runtimeEventsMarkdown(events))
	return nil
}

func runtimeEventsMarkdown(events []storage.RuntimeEvent) string {
	var b strings.Builder
	b.WriteString("OPS RUNTIME EVENTS\n\n")
	if len(events) == 0 {
		b.WriteString("No pending runtime events.\n")
		return b.String()
	}
	b.WriteString(fmt.Sprintf("Pending: %d\n\n", len(events)))
	for _, event := range events {
		b.WriteString(fmt.Sprintf("- #%d %s [%s] %s/%s", event.ID, event.Timestamp.Format(time.RFC3339), event.Severity, event.Source, event.Type))
		if event.Fingerprint != "" {
			b.WriteString(" fp=" + event.Fingerprint)
		}
		if event.PayloadJSON != "" {
			b.WriteString(" payload=" + compactEventPayload(event.PayloadJSON))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func compactEventPayload(payload string) string {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(payload), &v); err == nil {
		if b, err := json.Marshal(v); err == nil {
			payload = string(b)
		}
	}
	if len(payload) > 240 {
		return payload[:240] + "..."
	}
	return payload
}

func saveRuntimeEventJSON(db *storage.DB, source, eventType, severity, fingerprint string, payload any) {
	if db == nil {
		return
	}
	b, err := json.Marshal(payload)
	if err != nil {
		b = []byte(fmt.Sprintf(`{"summary":%q}`, fmt.Sprintf("%v", payload)))
	}
	if err := db.SaveRuntimeEvent(storage.RuntimeEvent{
		Timestamp:   time.Now().UTC(),
		Source:      source,
		Type:        eventType,
		Severity:    severity,
		Fingerprint: fingerprint,
		PayloadJSON: string(b),
	}); err != nil {
		fmt.Printf("runtime event warning: %v\n", err)
	}
}

func saveMarketWatchRuntimeEvents(db *storage.DB, report opsplan.Report, changed bool, critical bool) {
	severity := "info"
	if report.Market.Urgency == opsplan.UrgencyElevated {
		severity = "warning"
	}
	payload := map[string]any{
		"summary":             report.Summary,
		"urgency":             report.Market.Urgency,
		"permission":          report.Market.Permission,
		"plan_state":          report.Market.PlanState,
		"accumulation_phase":  report.Market.AccumulationPhase,
		"executable_now_usdt": report.Capital.ExecutableNowUSDT,
		"committed_usdt":      report.Capital.AlreadyCommittedUSDT,
		"fingerprint":         report.Fingerprint,
		"runtime_authority":   report.Runtime.ExecutionAuthority,
	}
	if changed {
		saveRuntimeEventJSON(db, "market-watch", "MARKET_STATE_CHANGED", severity, report.Fingerprint, payload)
	}
	saveNearUnlockRuntimeEvents(db, report)
	if critical {
		saveRuntimeEventJSON(db, "market-watch", "MARKET_CRITICAL", "critical", "critical:"+report.Fingerprint, payload)
	}
}

func saveNearUnlockRuntimeEvents(db *storage.DB, report opsplan.Report) {
	if db == nil {
		return
	}
	payload := map[string]any{
		"permission":          report.Market.Permission,
		"plan_state":          report.Market.PlanState,
		"accumulation_phase":  report.Market.AccumulationPhase,
		"executable_now_usdt": report.Capital.ExecutableNowUSDT,
		"summary":             report.Summary,
	}
	eventType, severity, prefix := liveAutoNearUnlockEvent(report)
	if eventType == "" {
		return
	}
	fingerprint := fmt.Sprintf("%s:%s|%s|%s|%.2f", prefix, report.Market.Permission, report.Market.PlanState, report.Market.AccumulationPhase, report.Capital.ExecutableNowUSDT)
	saveRuntimeEventJSON(db, "live-auto", eventType, severity, fingerprint, payload)
}

func liveAutoNearUnlockEvent(report opsplan.Report) (eventType, severity, prefix string) {
	if report.Market.PlanState == "ACTIVE_LIMIT" && report.Market.Permission == "ALLOWED" && report.Market.AccumulationPhase == "ACCUMULATION_CONFIRMED" && report.Capital.ExecutableNowUSDT > 0 {
		return "LIVE_AUTO_REAL_ORDER_READY", "critical", "real-ready"
	}
	if report.Market.PlanState == "ACTIVE_LIMIT" || report.Capital.ExecutableNowUSDT > 0 {
		return "LIVE_AUTO_READY_DRY_RUN_REQUIRED", "warning", "dry-run"
	}
	if report.Market.PlanState == "ARMED" || report.Market.Permission == "ARMED" || report.Market.Permission == "ALLOWED" || report.Market.AccumulationPhase == "RECLAIM" || report.Market.AccumulationPhase == "ACCUMULATION_CONFIRMED" {
		return "LIVE_AUTO_NEAR_UNLOCK", "warning", "near"
	}
	return "", "", ""
}

func liveAutoNearUnlockTelegram(report opsplan.Report) string {
	eventType, _, _ := liveAutoNearUnlockEvent(report)
	if eventType == "" {
		return ""
	}
	return fmt.Sprintf("LIVE-AUTO ALERT\n%s\nBTC permission=%s accumulation=%s plan=%s executable=%.2f USDT\nDry-run audit required before any real order.", eventType, report.Market.Permission, report.Market.AccumulationPhase, report.Market.PlanState, report.Capital.ExecutableNowUSDT)
}

func saveLiveSupervisorRuntimeEvent(db *storage.DB, result liveguard.SupervisorResult) {
	if !shouldRecordSupervisorEvent(result) {
		return
	}
	payload := map[string]any{
		"status":             result.Status,
		"action":             result.Action,
		"summary":            result.Summary,
		"auto_halted":        result.AutoHalted,
		"consecutive_errors": result.ConsecutiveErrors,
		"reasons":            result.Reasons,
	}
	fingerprint := fmt.Sprintf("%s|%s|%s|%v|%d", result.Status, result.Action, result.Summary, result.AutoHalted, result.ConsecutiveErrors)
	severity := "info"
	if result.AutoHalted || result.Status == liveguard.SupervisorWarn || result.Status == liveguard.SupervisorHalted {
		severity = "critical"
	} else if result.Managed != nil && (result.Managed.Status == liveguard.ManagedCycleBlocked || result.Managed.Status == liveguard.ManagedCyclePartial || len(result.Managed.Blocked) > 0) {
		severity = "warning"
	}
	if result.Managed != nil {
		payload["managed_status"] = result.Managed.Status
		payload["desired"] = len(result.Managed.Desired)
		payload["placed"] = len(result.Managed.Placed)
		payload["canceled"] = len(result.Managed.Canceled)
		payload["replaced"] = len(result.Managed.Replaced)
		payload["blocked"] = len(result.Managed.Blocked)
		payload["audit"] = managedAuditSummary(result.Managed)
		fingerprint = fmt.Sprintf("%s|managed=%s|d=%d|p=%d|c=%d|r=%d|b=%d", fingerprint, result.Managed.Status, len(result.Managed.Desired), len(result.Managed.Placed), len(result.Managed.Canceled), len(result.Managed.Replaced), len(result.Managed.Blocked))
	}
	saveRuntimeEventJSON(db, "live-supervisor", "LIVE_SUPERVISOR_EVENT", severity, fingerprint, payload)
}

func managedAuditSummary(result *liveguard.ManagedCycleResult) []string {
	if result == nil {
		return nil
	}
	out := []string{}
	add := func(items []liveguard.ManagedOrderDecision) {
		for _, item := range items {
			if len(item.AuditTrail) > 0 {
				out = append(out, item.AuditTrail...)
			}
		}
	}
	add(result.Placed)
	add(result.Blocked)
	if len(out) > 12 {
		return out[:12]
	}
	return out
}

func shouldRecordSupervisorEvent(result liveguard.SupervisorResult) bool {
	if result.AutoHalted || result.Status == liveguard.SupervisorWarn || result.Status == liveguard.SupervisorHalted {
		return true
	}
	if result.Managed == nil {
		return false
	}
	return result.Managed.Status == liveguard.ManagedCycleBlocked ||
		result.Managed.Status == liveguard.ManagedCyclePartial ||
		len(result.Managed.Placed) > 0 ||
		len(result.Managed.Canceled) > 0 ||
		len(result.Managed.Replaced) > 0 ||
		len(result.Managed.Blocked) > 0
}

func saveMicrostructureRuntimeEvents(db *storage.DB, summary microstructure.Summary) {
	if !summary.Enabled {
		return
	}
	payload := map[string]any{
		"status":         summary.Status,
		"summary":        summary.Summary,
		"fresh_symbols":  summary.FreshSymbols,
		"required_fresh": summary.RequiredFresh,
		"blockers":       summary.Blockers,
		"warnings":       summary.Warnings,
		"fingerprint":    summary.Fingerprint,
	}
	if summary.Status == microstructure.StatusBlock {
		saveRuntimeEventJSON(db, "microstructure", "MICROSTRUCTURE_STALE", "warning", "stale:"+summary.Fingerprint, payload)
	}
	saveRuntimeEventJSON(db, "microstructure", "MICROSTRUCTURE_STATE_CHANGED", "info", summary.Fingerprint, payload)
}
