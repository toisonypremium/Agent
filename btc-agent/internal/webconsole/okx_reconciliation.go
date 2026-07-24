package webconsole

import "btc-agent/internal/okxassets"

// OKXReconciliation is an observation-only discrepancy report. It never maps
// a currency/symbol to thesis_id and cannot repair ledger, reserve capital or
// authorize an order.
type OKXReconciliation struct {
	State          string   `json:"trang_thai"`
	UnlinkedAssets []string `json:"tai_san_chua_gan_thesis"`
	Warnings       []string `json:"canh_bao"`
}

func (s *Service) OKXReconciliation() (OKXReconciliation, error) {
	assets, err := s.OKXAssets()
	if err != nil || assets.State != okxassets.StateVerified {
		return OKXReconciliation{State: "khong_kha_dung", UnlinkedAssets: []string{}, Warnings: []string{"Không có ảnh chụp OKX Spot đã xác minh để đối soát."}}, nil
	}
	out := OKXReconciliation{State: "khop_chua_duoc_xac_nhan", UnlinkedAssets: []string{}, Warnings: []string{}}
	for _, asset := range assets.Assets {
		if asset.ThesisLink == okxassets.ThesisUnlinked {
			out.UnlinkedAssets = append(out.UnlinkedAssets, asset.Currency)
		}
	}
	if len(out.UnlinkedAssets) > 0 {
		out.State = "can_ra_soat"
		out.Warnings = append(out.Warnings, "Tài sản chưa gắn thesis không được suy diễn thành quyền mua, bán hoặc vốn DCA.")
	}
	return out, nil
}
