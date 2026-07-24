package webconsole

import (
	"database/sql"
	"time"
)

type DCAAllocationStatus struct {
	State              string          `json:"trang_thai"`
	ObservedAt         string          `json:"thoi_diem_quan_sat,omitempty"`
	AvailableUSDT      float64         `json:"usdt_kha_dung"`
	EnvelopeUSDT       float64         `json:"dca_envelope_usdt"`
	NetNewUSDT         float64         `json:"von_tang_rong_usdt"`
	BufferPercent      int             `json:"ty_le_dem_phan_tram"`
	GlobalCapPercent   float64         `json:"global_cap_phan_tram"`
	AutoHaltReason     string          `json:"ly_do_auto_halt,omitempty"`
	Allocations        []DCAAllocation `json:"phan_bo"`
	Warnings           []string        `json:"canh_bao"`
}
type DCAAllocation struct {
	ThesisID     string  `json:"thesis_id"`
	Symbol       string  `json:"symbol"`
	RatioPercent int     `json:"ty_le_phan_tram"`
	AmountUSDT   float64 `json:"amount_usdt"`
}

// DCAAllocationStatus is read-only; an epoch cannot authorize an order.
func (s *Service) DCAAllocationStatus() (DCAAllocationStatus, error) {
	out := DCAAllocationStatus{State: "chua_cap_von", BufferPercent: 20, Allocations: []DCAAllocation{}, Warnings: []string{"Chưa có allocation epoch đã xác minh."}}
	if epoch, err := s.db.LatestDCAAllocationEpoch(); err != nil {
		if err != sql.ErrNoRows {
			return out, err
		}
	} else {
		out.State = "da_cap_von"
		out.ObservedAt = epoch.ObservedAt.UTC().Format(time.RFC3339)
		out.AvailableUSDT = epoch.ObservedAvailableUSDT
		out.EnvelopeUSDT = epoch.EnvelopeUSDT
		out.NetNewUSDT = epoch.NetNewUSDT
		out.Warnings = []string{"Allocation epoch không tạo quyền BUY; operator halt và các DCA gate vẫn phải đạt."}
		for _, a := range epoch.Allocations {
			out.Allocations = append(out.Allocations, DCAAllocation{a.ThesisID, a.Symbol, int(a.Ratio * 100), a.AmountUSDT})
		}
	}
	cap, err := s.db.DCAExecutionState()
	if err != nil { return out, err }
	out.GlobalCapPercent = cap.GlobalCapPercent
	safety, err := s.db.DCASafetyState()
	if err != nil { return out, err }
	out.AutoHaltReason = safety.AutoHaltReason
	return out, nil
}
