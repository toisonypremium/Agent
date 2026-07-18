package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/storage"
)

type webDataEnvelope struct {
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
		return map[string]any{
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
}

func registerWebGET(mux *http.ServeMux, path string, value func() any) {
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "phương thức không được hỗ trợ", http.StatusMethodNotAllowed)
			return
		}
		now := time.Now().UTC()
		writeWebJSON(w, webDataEnvelope{DuLieu: value(), CapNhatLuc: now, DoCuGiay: 0, TrangThaiDuLieu: "MỚI", CanhBao: []string{}, Nguon: []string{"SQLite", "báo cáo production"}})
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
		if info, statErr := os.Stat(path); statErr == nil {
			out.CapNhatLuc = info.ModTime().UTC()
		}
	}
	out.DoCuGiay = maxInt64(0, int64(time.Since(out.CapNhatLuc).Seconds()))
	out.TrangThai = "MỚI"
	if maxAge > 0 && time.Since(out.CapNhatLuc) > maxAge {
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
