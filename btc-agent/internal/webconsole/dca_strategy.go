package webconsole

// DCAStrategy is a static, read-only policy view. It is deliberately not
// derived from account holdings and cannot create a thesis, allocation, or order.
type DCAStrategy struct {
	Stage              string         `json:"giai_doan"`
	Mechanism          string         `json:"co_che"`
	ExecutionAuthority bool           `json:"co_quyen_thuc_thi"`
	Layers             []DCALayer     `json:"cac_lop"`
	Candidates         []DCACandidate `json:"ung_vien"`
	GlobalBlockers     []string       `json:"blocker_toan_cuc"`
}
type DCALayer struct {
	Name    string `json:"ten"`
	Percent int    `json:"ty_le_phan_tram"`
	Rule    string `json:"quy_tac"`
}
type DCACandidate struct {
	Symbol        string   `json:"ma_tai_san"`
	Status        string   `json:"trang_thai"`
	Risk          string   `json:"muc_rui_ro"`
	ThesisID      string   `json:"thesis_id"`
	AllocatedUSDT string   `json:"von_duoc_cap_usdt"`
	Thesis        string   `json:"luan_diem"`
	Blockers      []string `json:"blocker"`
}

func (s *Service) DCAStrategy() DCAStrategy {
	base := []string{"Chưa có thesis được phê duyệt và cấp vốn", "Operator halt đang bật", "Chưa đủ evidence để cấp quyền auto-live"}
	return DCAStrategy{Stage: "HALTED_SHADOW", Mechanism: "Limit/post-only · Ba lớp 25% / 35% / 40%", ExecutionAuthority: false,
		Layers:     []DCALayer{{"Lớp 1", 25, "Vào vị thế đầu tiên khi toàn bộ gate đạt"}, {"Lớp 2", 35, "Tích lũy khi thesis còn hợp lệ; không averaging mù"}, {"Lớp 3", 40, "Yêu cầu discount/drawdown và evidence mạnh"}},
		Candidates: []DCACandidate{{"ETH", "nghien_cuu", "trung_binh", "", "0", "Nền tảng smart-contract và thanh khoản core", append([]string{}, base...)}, {"LINK", "nghien_cuu", "trung_binh_cao", "", "0", "Hạ tầng oracle và CCIP", append([]string{}, base...)}, {"VIRTUAL", "nghien_cuu", "cao", "", "0", "Narrative giao thức AI-agent", append([]string{}, base...)}}, GlobalBlockers: base}
}
