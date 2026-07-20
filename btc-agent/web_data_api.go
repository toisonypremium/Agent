package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/storage"
)

type webDataEnvelope struct {
	SchemaVersion   string    `json:"schema_version"`
	SnapshotID      string    `json:"snapshot_id"`
	DuLieu          any       `json:"du_lieu"`
	CapNhatLuc      time.Time `json:"cap_nhat_luc"`
	DoCuGiay        int64     `json:"do_cu_giay"`
	TrangThaiDuLieu string    `json:"trang_thai_du_lieu"`
	CanhBao         []string  `json:"canh_bao"`
	Nguon           []string  `json:"nguon"`
}

type webReport struct {
	DuLieu     any       `json:"du_lieu,omitempty"`
	CapNhatLuc time.Time `json:"cap_nhat_luc,omitempty"`
	DoCuGiay   int64     `json:"do_cu_giay"`
	TrangThai  string    `json:"trang_thai"`
	Nguon      string    `json:"nguon"`
}

func registerWebDataAPI(mux *http.ServeMux, cfg config.Config, db *storage.DB) {
	registerWebBootstrap(mux, cfg, db)
	registerWebGET(mux, "/api/v1/tong-quan", func() any {
		return map[string]any{
			"he_thong":          buildWebSnapshot(cfg, db),
			"ke_hoach_van_hanh": loadWebReport("operations_plan_latest.json", 35*time.Minute),
			"trang_thai_bot":    loadWebReport("bot_state_latest.json", 35*time.Minute),
			"kiem_tra_he_thong": loadWebReport("live_doctor_latest.json", 35*time.Minute),
			"kiem_toan":         loadWebReport("live_auto_audit_latest.json", 90*time.Minute),
			"nhip_song":         loadWebReport("scheduler_heartbeat_latest.json", 10*time.Minute),
			"doi_soat":          loadWebReport("live_reconcile_latest.json", 35*time.Minute),
		}
	})
	registerWebGET(mux, "/api/v1/von", func() any {
		positions, positionErr := db.LivePositions()
		capital, capitalErr := db.BuildCapitalAuthoritySnapshot(cfg, time.Now().UTC())
		return map[string]any{
			"von_authority":       capital,
			"loi_von_authority":   webErrorText(capitalErr),
			"ke_hoach_van_hanh":   loadWebReport("operations_plan_latest.json", 35*time.Minute),
			"san_sang_giao_dich":  loadWebReport("live_readiness_latest.json", 35*time.Minute),
			"ke_hoach_nghien_cuu": loadWebReport("capital_plan_research_latest.json", 90*time.Minute),
			"vi_the":              positions,
			"loi_vi_the":          webErrorText(positionErr),
		}
	})
	registerWebGET(mux, "/api/v1/thi-truong", func() any {
		return map[string]any{
			"ke_hoach_van_hanh":  loadWebReport("operations_plan_latest.json", 35*time.Minute),
			"phan_tich_moi_nhat": loadWebReport("latest.json", 35*time.Minute),
			"kich_ban":           loadWebReport("scenario_latest.json", 35*time.Minute),
			"vi_mo_thi_truong":   loadWebReport("microstructure_latest.json", 35*time.Minute),
		}
	})
	registerWebGET(mux, "/api/v1/tai-san", func() any {
		return map[string]any{
			"bang_diem_ky_thuat": loadWebReport("technical_scorecard_latest.json", 35*time.Minute),
			"vi_mo_thi_truong":   loadWebReport("microstructure_latest.json", 35*time.Minute),
			"ke_hoach_van_hanh":  loadWebReport("operations_plan_latest.json", 35*time.Minute),
		}
	})
	registerWebGET(mux, "/api/v1/vi-the", func() any {
		positions, positionErr := db.LivePositions()
		orders, orderErr := db.OpenLiveOrdersDetailed()
		return map[string]any{
			"vi_the":         positions,
			"lenh_dang_mo":   orders,
			"bao_cao_vi_the": loadWebReport("live_position_latest.json", 35*time.Minute),
			"doi_soat":       loadWebReport("live_reconcile_latest.json", 35*time.Minute),
			"loi_vi_the":     webErrorText(positionErr),
			"loi_lenh":       webErrorText(orderErr),
		}
	})
	registerWebGET(mux, "/api/v1/rui-ro", func() any {
		return map[string]any{
			"san_sang_giao_dich":  loadWebReport("live_readiness_latest.json", 35*time.Minute),
			"kiem_tra_he_thong":   loadWebReport("live_doctor_latest.json", 35*time.Minute),
			"kiem_toan":           loadWebReport("live_auto_audit_latest.json", 90*time.Minute),
			"bang_chung_thuc_thi": loadWebReport("execution_evidence_latest.json", 90*time.Minute),
		}
	})
	registerWebGET(mux, "/api/v1/hermes", func() any {
		management := loadWebReport("hermes_report_latest.json", 90*time.Minute)
		shadow := loadWebReport("hermes_shadow_decision_latest.json", 35*time.Minute)
		warnings := []string{}
		if management.CapNhatLuc.IsZero() {
			warnings = append(warnings, "báo cáo quản lý Hermes chưa có thời điểm cập nhật")
		}
		if shadow.CapNhatLuc.IsZero() {
			warnings = append(warnings, "quyết định bóng Hermes chưa có thời điểm cập nhật")
		}
		latestSource := "báo cáo quản lý Hermes"
		if !shadow.CapNhatLuc.IsZero() && (management.CapNhatLuc.IsZero() || shadow.CapNhatLuc.After(management.CapNhatLuc)) {
			latestSource = "quyết định bóng Hermes"
		}
		return map[string]any{
			"bao_cao_quan_ly":  management,
			"quyet_dinh_bong":  shadow,
			"canh_bao_do_tuoi": warnings,
			"nguon_moi_nhat":   latestSource,
			"ghi_chu_lich":     "Báo cáo quản lý cập nhật mỗi 60 phút; quyết định bóng cập nhật trước chu kỳ thực thi và là nguồn mới nhất cho quyết định Hermes.",

			"bo_giam_sat":         loadWebReport("live_supervisor_latest.json", 35*time.Minute),
			"bang_quyet_dinh":     loadWebReport("decision_dashboard_latest.json", 35*time.Minute),
			"trang_thai_bot":      loadWebReport("bot_state_latest.json", 35*time.Minute),
			"bang_chung_thuc_thi": loadWebReport("execution_evidence_latest.json", 90*time.Minute),
			"bao_cao_vi_the":      loadWebReport("live_position_latest.json", 35*time.Minute),
		}
	})
	registerWebGET(mux, "/api/v1/canary", func() any {
		return map[string]any{
			"he_thong":          buildWebSnapshot(cfg, db),
			"san_sang_ky_thuat": loadWebReport("hermes_canary_readiness_latest.json", 30*time.Minute),
			"dien_tap":          loadWebReport("canary_rehearsal_latest.json", 35*time.Minute),
			"do_tuoi_critical": map[string]any{
				"doctor":    loadWebReport("live_doctor_latest.json", 35*time.Minute),
				"doi_soat":  loadWebReport("live_reconcile_latest.json", 35*time.Minute),
				"heartbeat": loadWebReport("scheduler_heartbeat_latest.json", 10*time.Minute),
			},
		}
	})
	registerWebGET(mux, "/api/v1/van-hanh", func() any {
		return map[string]any{
			"nhip_song":          loadWebReport("scheduler_heartbeat_latest.json", 10*time.Minute),
			"kiem_tra_nhip_song": loadWebReport("scheduler_heartbeat_check_latest.json", 10*time.Minute),
			"kiem_tra_he_thong":  loadWebReport("live_doctor_latest.json", 35*time.Minute),
			"doi_soat":           loadWebReport("live_reconcile_latest.json", 35*time.Minute),
			"bao_tri":            loadWebReport("maintenance_latest.json", 26*time.Hour),
			"phien_ban":          map[string]string{"ten": version, "commit": commit, "thoi_gian_build": buildTime},
		}
	})
	registerWebGET(mux, "/api/v1/du-lieu", func() any {
		return map[string]any{
			"free_api": loadWebReport("freeapi_latest.json", 90*time.Minute),
		}
	})
	registerWebGET(mux, "/api/v1/nghien-cuu", func() any {
		return map[string]any{
			"bo_nao":       loadWebReport("hermes_brain_audit_latest.json", 90*time.Minute),
			"gia_thuyet":   loadWebReport("hermes_hypothesis_audit_latest.json", 90*time.Minute),
			"research_run": loadWebReport("hermes_research_audit_latest.json", 90*time.Minute),
			"hieu_chuan":   loadWebReport("hermes_calibration_latest.json", 90*time.Minute),
			"policy": map[string]string{
				"authority":        "research_only",
				"memory_authority": "evidence_only",
				"execution":        "deterministic_engine_only",
			},
		}
	})
}

func registerWebBootstrap(mux *http.ServeMux, cfg config.Config, db *storage.DB) {
	mux.HandleFunc("/api/v2/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "phương thức không được hỗ trợ", http.StatusMethodNotAllowed)
			return
		}
		now := time.Now().UTC()
		operations := loadWebReport("operations_plan_latest.json", 35*time.Minute)
		if object, ok := operations.DuLieu.(map[string]any); ok {
			operations.DuLieu = map[string]any{"capital": object["capital"], "market": object["market"]}
		}
		snapshot := buildWebSnapshot(cfg, db)
		compactSystem := map[string]any{
			"generated_at": snapshot.GeneratedAt, "version": snapshot.Version,
			"service_status": snapshot.ServiceStatus, "service_meta": snapshot.ServiceMeta,
			"open_orders": snapshot.OpenOrders, "heartbeat_age": snapshot.HeartbeatAge,
			"authority": snapshot.Authority, "authority_reason": snapshot.AuthorityReason,
			"halted": snapshot.Halted,
		}
		audit := compactWebReport(loadWebReport("live_auto_audit_latest.json", 90*time.Minute), "generated_at", "verdict", "summary", "current_market_authority", "real_order_approved", "dry_run_approved")
		botState := compactWebReport(loadWebReport("bot_state_latest.json", 35*time.Minute), "generated_at", "mode", "scheduler_status", "supervisor_status", "doctor_status", "operator_halt", "open_live_orders", "live_positions", "plan_state", "btc_permission", "can_submit_live_order")
		payload := map[string]any{
			"he_thong": compactSystem, "ke_hoach_van_hanh": operations,
			"trang_thai_bot":    botState,
			"kiem_tra_he_thong": loadWebReport("live_doctor_latest.json", 35*time.Minute),
			"kiem_toan":         audit,
			"nhip_song":         loadWebReport("scheduler_heartbeat_latest.json", 10*time.Minute),
			"doi_soat":          loadWebReport("live_reconcile_latest.json", 35*time.Minute),
		}
		status, age, warnings := summarizeWebPayload(payload)
		fingerprint := bootstrapFingerprint(payload)
		etag := `"` + fingerprint + `"`
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		writeWebJSON(w, webDataEnvelope{SchemaVersion: "web-v2", SnapshotID: fingerprint, DuLieu: payload, CapNhatLuc: now, DoCuGiay: age, TrangThaiDuLieu: status, CanhBao: warnings, Nguon: []string{"SQLite", "báo cáo production compact"}})
	})
}

func compactWebReport(report webReport, keys ...string) webReport {
	object, ok := report.DuLieu.(map[string]any)
	if !ok {
		return report
	}
	compact := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, exists := object[key]; exists {
			compact[key] = value
		}
	}
	report.DuLieu = compact
	return report
}

func bootstrapFingerprint(payload map[string]any) string {
	data, _ := json.Marshal(payload)
	var stable any
	_ = json.Unmarshal(data, &stable)
	stable = stableBootstrapValue(stable)
	data, _ = json.Marshal(stable)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:12])
}

func stableBootstrapValue(value any) any {
	switch current := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(current))
		for key, child := range current {
			if key == "do_cu_giay" || key == "heartbeat_age" || (key == "generated_at" && current["service_status"] != nil) {
				continue
			}
			out[key] = stableBootstrapValue(child)
		}
		return out
	case []any:
		out := make([]any, len(current))
		for i, child := range current {
			out[i] = stableBootstrapValue(child)
		}
		return out
	default:
		return value
	}
}

func registerWebGET(mux *http.ServeMux, path string, value func() any) {
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "phương thức không được hỗ trợ", http.StatusMethodNotAllowed)
			return
		}
		now := time.Now().UTC()
		payload := value()
		status, age, warnings := summarizeWebPayload(payload)
		writeWebJSON(w, webDataEnvelope{SchemaVersion: "web-v1.1", SnapshotID: now.Format("20060102T150405.000000000Z"), DuLieu: payload, CapNhatLuc: now, DoCuGiay: age, TrangThaiDuLieu: status, CanhBao: warnings, Nguon: []string{"SQLite", "báo cáo production"}})
	})
}

func loadWebReport(name string, maxAge time.Duration) webReport {
	maxAge = freshnessPolicy(name, maxAge).MaxAge
	path := filepath.Join("reports", filepath.Base(name))
	out := webReport{TrangThai: "THIẾU", Nguon: name}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var value any
	if json.Unmarshal(data, &value) != nil {
		out.TrangThai = "LỖI"
		return out
	}
	out.DuLieu = value
	out.CapNhatLuc = reportGeneratedAt(value)
	if out.CapNhatLuc.IsZero() {
		// A malformed explicit timestamp must not be hidden by file mtime.
		if !reportHasTimestampField(value) {
			if info, statErr := os.Stat(path); statErr == nil {
				out.CapNhatLuc = info.ModTime().UTC()
			}
		} else {
			out.TrangThai = "LỖI"
			return out
		}
	}
	age := time.Since(out.CapNhatLuc)
	if age < -time.Minute {
		out.TrangThai = "LỖI"
		return out
	}
	out.DoCuGiay = maxInt64(0, int64(age.Seconds()))
	out.TrangThai = "MỚI"
	if maxAge > 0 && age > maxAge {
		out.TrangThai = "CŨ"
	}
	return out
}

func reportGeneratedAt(value any) time.Time {
	object, ok := value.(map[string]any)
	if !ok {
		return time.Time{}
	}
	for _, key := range []string{"generated_at", "updated_at", "timestamp"} {
		text, ok := object[key].(string)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func webErrorText(err error) string {
	if err == nil {
		return ""
	}
	return "không đọc được dữ liệu"
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func reportHasTimestampField(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range []string{"generated_at", "updated_at", "timestamp"} {
		if _, exists := object[key]; exists {
			return true
		}
	}
	return false
}

func summarizeWebPayload(value any) (string, int64, []string) {
	status, age := "MỚI", int64(0)
	warnings := []string{}
	priority := map[string]int{"MỚI": 0, "CŨ": 1, "THIẾU": 2, "LỖI": 3}
	var walk func(reflect.Value)
	walk = func(v reflect.Value) {
		if !v.IsValid() {
			return
		}
		if v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
			if !v.IsNil() {
				walk(v.Elem())
			}
			return
		}
		if v.Type() == reflect.TypeOf(webReport{}) {
			r := v.Interface().(webReport)
			if priority[r.TrangThai] > priority[status] {
				status = r.TrangThai
			}
			if r.DoCuGiay > age {
				age = r.DoCuGiay
			}
			if r.TrangThai != "MỚI" {
				warnings = append(warnings, r.Nguon+": "+r.TrangThai)
			}
			return
		}
		switch v.Kind() {
		case reflect.Map:
			it := v.MapRange()
			for it.Next() {
				walk(it.Value())
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if v.Field(i).CanInterface() {
					walk(v.Field(i))
				}
			}
		}
	}
	walk(reflect.ValueOf(value))
	return status, age, uniqueStrings(warnings)
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
