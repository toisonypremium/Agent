package main

import (
	"btc-agent/internal/circuitresearch"
	"btc-agent/internal/config"
	"btc-agent/internal/operatorcapability"
	"btc-agent/internal/storage"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type dashboardDomain struct {
	Status        string    `json:"status"`
	GeneratedAt   time.Time `json:"generated_at,omitempty"`
	ObservedAt    time.Time `json:"observed_at"`
	AgeSeconds    int64     `json:"age_seconds"`
	MaxAgeSeconds int64     `json:"max_age_seconds"`
	Source        string    `json:"source"`
	Warnings      []string  `json:"warnings"`
	ReasonCodes   []string  `json:"reason_codes"`
	Data          any       `json:"data,omitempty"`
}
type dashboardActivity struct {
	ID       string    `json:"id"`
	Time     time.Time `json:"time"`
	Speaker  string    `json:"speaker"`
	Severity string    `json:"severity"`
	Title    string    `json:"title"`
	Summary  string    `json:"summary"`
	Source   string    `json:"source"`
}
type dashboardV3 struct {
	SchemaVersion string                     `json:"schema_version"`
	SnapshotID    string                     `json:"snapshot_id"`
	GeneratedAt   time.Time                  `json:"generated_at"`
	Authority     any                        `json:"authority"`
	Domains       map[string]dashboardDomain `json:"domains"`
	Capabilities  any                        `json:"capabilities"`
}

func normalizeDomainStatus(s string) string {
	switch s {
	case "MỚI":
		return "FRESH"
	case "CŨ":
		return "STALE"
	case "THIẾU":
		return "MISSING"
	case "LỖI":
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}
func reportDomain(r webReport, maxAge time.Duration, now time.Time) dashboardDomain {
	warnings := []string{}
	if r.TrangThai != "MỚI" {
		warnings = append(warnings, r.Nguon+": "+r.TrangThai)
	}
	return dashboardDomain{Status: normalizeDomainStatus(r.TrangThai), GeneratedAt: r.CapNhatLuc, ObservedAt: now, AgeSeconds: r.DoCuGiay, MaxAgeSeconds: int64(maxAge.Seconds()), Source: r.Nguon, Warnings: warnings, ReasonCodes: []string{}, Data: r.DuLieu}
}
func compositeDomain(now time.Time, source string, reports map[string]webReport, data any) dashboardDomain {
	status := "FRESH"
	rank := map[string]int{"FRESH": 0, "STALE": 1, "MISSING": 2, "ERROR": 3, "UNKNOWN": 4}
	warnings := []string{}
	var generated time.Time
	var maxAge int64
	for name, r := range reports {
		s := normalizeDomainStatus(r.TrangThai)
		if rank[s] > rank[status] {
			status = s
		}
		if r.CapNhatLuc.After(generated) {
			generated = r.CapNhatLuc
		}
		if r.DoCuGiay > maxAge {
			maxAge = r.DoCuGiay
		}
		if s != "FRESH" {
			warnings = append(warnings, name+": "+s)
		}
	}
	return dashboardDomain{Status: status, GeneratedAt: generated, ObservedAt: now, AgeSeconds: maxAge, Source: source, Warnings: warnings, ReasonCodes: []string{}, Data: data}
}
func buildDashboardV3(cfg config.Config, db *storage.DB) dashboardV3 {
	now := time.Now().UTC()
	s := buildWebSnapshot(cfg, db)
	heartbeat := loadWebReport("scheduler_heartbeat_latest.json", 10*time.Minute)
	operations := loadWebReport("operations_plan_latest.json", 35*time.Minute)
	doctor := loadWebReport("live_doctor_latest.json", 35*time.Minute)
	reconcile := loadWebReport("live_reconcile_latest.json", 35*time.Minute)
	bot := loadWebReport("bot_state_latest.json", 35*time.Minute)
	supervisor := loadWebReport("live_supervisor_latest.json", 35*time.Minute)
	hermesReport := loadWebReport("hermes_report_latest.json", 90*time.Minute)
	hermesShadow := loadWebReport("hermes_shadow_decision_latest.json", 35*time.Minute)
	decision := loadWebReport("decision_dashboard_latest.json", 35*time.Minute)
	evidence := loadWebReport("execution_evidence_latest.json", 90*time.Minute)
	readiness := loadWebReport("live_readiness_latest.json", 35*time.Minute)
	audit := loadWebReport("live_auto_audit_latest.json", 90*time.Minute)
	canary := loadWebReport("hermes_canary_readiness_latest.json", 30*time.Minute)
	technical := loadWebReport("technical_scorecard_latest.json", 35*time.Minute)
	micro := loadWebReport("microstructure_latest.json", 35*time.Minute)
	scenario := loadWebReport("scenario_latest.json", 35*time.Minute)
	maintenance := loadWebReport("maintenance_latest.json", 26*time.Hour)
	positions, posErr := db.LivePositions()
	orders, ordErr := db.OpenLiveOrdersDetailed()
	capital, capErr := db.BuildCapitalAuthoritySnapshot(cfg, now)
	op := object(operations.DuLieu)
	hb := object(heartbeat.DuLieu)
	hermesSource := "hermes_shadow_decision_latest.json"
	hermesData := hermesShadow.DuLieu
	if hermesShadow.TrangThai == "THIẾU" {
		hermesSource = "fallback: hermes_report + live_supervisor + bot_state + decision_dashboard"
		hermesData = map[string]any{"configured_mode": cfg.HermesOperator.NormalizedMode(), "management": hermesReport, "supervisor": supervisor, "bot_state": bot, "decision_dashboard": decision, "execution_evidence": evidence, "fallback_reason": "Hermes shadow decision chưa được tạo; đang dùng nguồn canonical hiện có."}
	}
	domains := map[string]dashboardDomain{}
	domains["tong_quan"] = compositeDomain(now, "operations_plan + canonical snapshot", map[string]webReport{"operations": operations, "heartbeat": heartbeat, "doctor": doctor, "reconcile": reconcile}, map[string]any{"system": s, "operations": operations.DuLieu})
	domains["thi_truong"] = compositeDomain(now, "operations_plan + microstructure + scenario + scorecard", map[string]webReport{"operations": operations, "microstructure": micro, "scenario": scenario, "technical": technical}, map[string]any{"market": op["market"], "microstructure": micro.DuLieu, "scenario": scenario.DuLieu, "technical_scorecard": technical.DuLieu})
	domains["danh_muc"] = compositeDomain(now, "SQLite live ledger + capital authority + reconcile", map[string]webReport{"reconcile": reconcile}, map[string]any{"positions": positions, "orders": orders, "capital_authority": capital, "position_error": webErrorText(posErr), "order_error": webErrorText(ordErr), "capital_error": webErrorText(capErr), "operations_capital": op["capital"], "reconcile": reconcile.DuLieu})
	domains["hermes"] = compositeDomain(now, hermesSource, map[string]webReport{"management": hermesReport, "supervisor": supervisor, "bot": bot, "decision": decision}, hermesData)
	domains["rui_ro"] = compositeDomain(now, "doctor + readiness + audit + reconcile + canary", map[string]webReport{"doctor": doctor, "readiness": readiness, "audit": audit, "reconcile": reconcile, "canary": canary}, map[string]any{"doctor": doctor.DuLieu, "readiness": readiness.DuLieu, "audit": audit.DuLieu, "reconcile": reconcile.DuLieu, "canary": canary.DuLieu})
	domains["circuit"] = loadCircuitDashboardDomain(now)
	domains["van_hanh"] = compositeDomain(now, "heartbeat + scheduler check + maintenance", map[string]webReport{"heartbeat": heartbeat, "maintenance": maintenance}, map[string]any{"heartbeat": heartbeat.DuLieu, "maintenance": maintenance.DuLieu, "release": map[string]string{"version": version, "commit": commit, "build_time": buildTime}})
	scheduleDomain := reportDomain(heartbeat, 10*time.Minute, now)
	scheduleDomain.Source = "scheduler_heartbeat_latest.json"
	scheduleDomain.Data = map[string]any{"timezone": hb["timezone"], "status": hb["status"], "last_event": hb["last_event"], "last_event_at": hb["last_event_at"], "next_daily_run": hb["next_daily_run"], "next_maintenance_run": hb["next_maintenance_run"], "next_research_brief": hb["next_research_brief"], "next_market_watch": hb["next_market_watch"], "next_reconcile": hb["next_reconcile"], "next_live_supervisor_cycle": hb["next_live_supervisor_cycle"], "configured_scan_minutes": nested(op, "monitoring", "configured_scan_minutes"), "recommended_scan_minutes": nested(op, "monitoring", "recommended_scan_minutes"), "reconcile_interval_minutes": nested(op, "runtime", "reconcile_interval_minutes"), "management_interval_minutes": nested(op, "runtime", "management_interval_minutes")}
	domains["lich_trinh"] = scheduleDomain
	domains["nhat_ky"] = activityDomain(db, now, operations, doctor, reconcile, supervisor, hermesReport, domains["circuit"])
	caps := operatorcapability.Evaluate(operatorcapability.State{})
	out := dashboardV3{SchemaVersion: "dashboard-v3.1", GeneratedAt: now, Authority: map[string]any{"market": s.Authority, "reason": s.AuthorityReason, "halted": s.Halted, "execution": "DETERMINISTIC_ENGINE_ONLY", "real_order": "NOT_APPROVED", "circuit": "RESEARCH_ONLY"}, Domains: domains, Capabilities: caps}
	b, _ := json.Marshal(out)
	h := sha256.Sum256(b)
	out.SnapshotID = hex.EncodeToString(h[:12])
	return out
}
func object(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}
func nested(m map[string]any, k1, k2 string) any { return object(m[k1])[k2] }
func activityDomain(db *storage.DB, now time.Time, reports ...any) dashboardDomain {
	events, err := db.RecentRuntimeEvents(40, 0)
	warnings := []string{}
	if err != nil {
		warnings = append(warnings, "Không đọc được nhật ký runtime")
	}
	messages := []dashboardActivity{}
	seen := map[string]bool{}
	for _, e := range events {
		key := e.Source + "|" + e.Type + "|" + e.Fingerprint
		if e.Fingerprint != "" && seen[key] {
			continue
		}
		seen[key] = true
		summary := eventSummary(e.PayloadJSON)
		messages = append(messages, dashboardActivity{ID: fmt.Sprintf("event-%d", e.ID), Time: e.Timestamp, Speaker: speaker(e.Source), Severity: strings.ToUpper(e.Severity), Title: eventTitle(e.Type), Summary: summary, Source: e.Source})
		if len(messages) >= 20 {
			break
		}
	}
	return dashboardDomain{Status: func() string {
		if err != nil {
			return "ERROR"
		}
		if len(messages) == 0 {
			return "MISSING"
		}
		return "FRESH"
	}(), GeneratedAt: func() time.Time {
		if len(messages) > 0 {
			return messages[0].Time
		}
		return time.Time{}
	}(), ObservedAt: now, AgeSeconds: func() int64 {
		if len(messages) > 0 {
			return maxInt64(0, int64(now.Sub(messages[0].Time).Seconds()))
		}
		return 0
	}(), MaxAgeSeconds: 3600, Source: "SQLite runtime_events (read-only)", Warnings: warnings, ReasonCodes: []string{}, Data: map[string]any{"messages": messages}}
}
func eventSummary(raw string) string {
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return "Đã ghi nhận sự kiện vận hành."
	}
	for _, k := range []string{"summary", "reason", "action", "status"} {
		if v, ok := m[k]; ok && fmt.Sprint(v) != "" {
			return truncate(fmt.Sprint(v), 360)
		}
	}
	return "Đã ghi nhận sự kiện vận hành."
}
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
func speaker(source string) string {
	s := strings.ToLower(source)
	switch {
	case strings.Contains(s, "market") || strings.Contains(s, "microstructure"):
		return "THỊ TRƯỜNG"
	case strings.Contains(s, "hermes"):
		return "HERMES"
	case strings.Contains(s, "supervisor"):
		return "GIÁM SÁT"
	default:
		return "HỆ THỐNG"
	}
}
func eventTitle(t string) string {
	switch t {
	case "MARKET_STATE_CHANGED":
		return "Trạng thái thị trường thay đổi"
	case "LIVE_SUPERVISOR_EVENT":
		return "Kết quả chu kỳ giám sát"
	case "MICROSTRUCTURE_STATE_CHANGED":
		return "Vi cấu trúc thị trường cập nhật"
	case "operator_halt_request":
		return "Yêu cầu dừng khẩn cấp"
	default:
		return strings.ReplaceAll(strings.ToLower(t), "_", " ")
	}
}
func loadCircuitDashboardDomain(now time.Time) dashboardDomain {
	out := dashboardDomain{Status: "MISSING", ObservedAt: now, MaxAgeSeconds: 900, Source: "reports/circuit validated artifacts", Warnings: []string{"Chưa có bằng chứng Circuit"}, ReasonCodes: []string{"CIRCUIT_EVIDENCE_MISSING"}}
	raw, err := os.ReadFile(filepath.Join("reports", "circuit", "evidence_latest.json"))
	if err != nil {
		return out
	}
	var e circuitresearch.Evidence
	if json.Unmarshal(raw, &e) != nil {
		out.Status = "ERROR"
		out.ReasonCodes = []string{"CIRCUIT_OUTPUT_INVALID_JSON"}
		return out
	}
	if e.SchemaVersion != circuitresearch.EvidenceSchemaVersion || e.Authority != "RESEARCH_ONLY" || e.ExecutionIntent != nil || (e.ResearchAction != circuitresearch.ActionWatch && e.ResearchAction != circuitresearch.ActionNoTrade && e.ResearchAction != circuitresearch.ActionInvestigate) {
		out.Status = "ERROR"
		out.ReasonCodes = []string{"CIRCUIT_AUTHORITY_OR_SCHEMA_INVALID"}
		return out
	}
	out.Status = "FRESH"
	if !now.Before(e.ValidUntil) {
		out.Status = "STALE"
		out.Warnings = []string{"Bằng chứng Circuit mới nhất đã hết hạn"}
	} else {
		out.Warnings = []string{}
	}
	var soak any
	if b, er := os.ReadFile(filepath.Join("reports", "circuit", "soak_status.json")); er == nil {
		_ = json.Unmarshal(b, &soak)
	}
	out.GeneratedAt = e.GeneratedAt
	out.AgeSeconds = maxInt64(0, int64(now.Sub(e.GeneratedAt).Seconds()))
	out.ReasonCodes = []string{}
	out.Data = map[string]any{"run_id": e.RunID, "status": e.Status, "action": e.ResearchAction, "confidence": e.Confidence, "producer_commit": e.ProducerCommit, "input_sha256": e.InputSHA256, "output_sha256": e.OutputSHA256, "valid_until": e.ValidUntil, "authority": "RESEARCH_ONLY", "execution_intent": nil, "limitations": e.Limitations, "soak": soak}
	return out
}
