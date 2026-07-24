package webconsole

import (
	"btc-agent/internal/okxassets"
)

type OKXAssetSource interface {
	Load() (okxassets.Artifact, error)
}
type fixedOKXAssetSource struct {
	dir string
	now Clock
}

func NewOKXAssetArtifact(dir string, now Clock) OKXAssetSource {
	return fixedOKXAssetSource{dir: dir, now: now}
}
func (s fixedOKXAssetSource) Load() (okxassets.Artifact, error) {
	return okxassets.LoadArtifact(s.dir, s.now().UTC(), runtimeHealthFreshFor)
}

type OKXAssets struct {
	Source     string            `json:"nguon"`
	ObservedAt string            `json:"thoi_diem_quan_sat,omitempty"`
	State      string            `json:"trang_thai"`
	Assets     []okxassets.Asset `json:"tai_san"`
	Warnings   []string          `json:"canh_bao"`
}

func (s *Service) SetOKXAssetSource(source OKXAssetSource) { s.okxAssets = source }
func (s *Service) OKXAssets() (OKXAssets, error) {
	out := OKXAssets{State: okxassets.StateUnavailable, Assets: []okxassets.Asset{}, Warnings: []string{"Chưa có dữ liệu tài sản Spot OKX đã xác minh."}}
	if s.okxAssets == nil {
		return out, nil
	}
	a, err := s.okxAssets.Load()
	if err != nil {
		return out, nil
	}
	return OKXAssets{Source: a.Source, ObservedAt: a.ObservedAt, State: a.State, Assets: a.Assets, Warnings: a.Warnings}, nil
}
