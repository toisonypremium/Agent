// Package okxassets normalizes read-only Spot balance observations. It has no
// exchange order, transfer, withdrawal, ledger, or execution authority.
package okxassets

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

const (
	SourceOKXSpotReadOnly = "okx_spot_read_only"
	ThesisNotApplicable   = "khong_ap_dung"
	ThesisUnlinked        = "chua_gan_thesis"
)

type Asset struct {
	Currency   string `json:"ma_tai_san"`
	Available  string `json:"kha_dung"`
	Frozen     string `json:"dang_khoa"`
	Total      string `json:"tong"`
	ThesisLink string `json:"trang_thai_gan_thesis"`
}

type Snapshot struct {
	Source string  `json:"nguon"`
	Assets []Asset `json:"tai_san"`
}

// ParseSpotBalance accepts only the constrained account/balance response data.
// It validates exact decimal arithmetic before any observer can publish it.
func ParseSpotBalance(body []byte) (Snapshot, error) {
	var raw struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Data []struct {
			Details []struct {
				Currency  string `json:"ccy"`
				Available string `json:"availBal"`
				Frozen    string `json:"frozenBal"`
				Total     string `json:"cashBal"`
			} `json:"details"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return Snapshot{}, fmt.Errorf("okx balance decode: %w", err)
	}
	if raw.Code != "0" {
		return Snapshot{}, fmt.Errorf("okx balance code %s", raw.Code)
	}
	if len(raw.Data) != 1 {
		return Snapshot{}, fmt.Errorf("okx balance data cardinality invalid")
	}
	assets := make([]Asset, 0, len(raw.Data[0].Details))
	for _, d := range raw.Data[0].Details {
		ccy := strings.ToUpper(strings.TrimSpace(d.Currency))
		total, err := decimal(d.Total)
		if err != nil {
			return Snapshot{}, fmt.Errorf("%s total: %w", ccy, err)
		}
		if total.Sign() == 0 {
			continue
		}
		link := ThesisUnlinked
		if ccy == "USDT" {
			link = ThesisNotApplicable
		}
		assets = append(assets, Asset{Currency: ccy, Available: d.Available, Frozen: d.Frozen, Total: d.Total, ThesisLink: link})
	}
	snapshot := Snapshot{Source: SourceOKXSpotReadOnly, Assets: assets}
	if err := ValidateSnapshot(snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

// ValidateSnapshot validates data both when it arrives from OKX and when a
// persisted artifact is later loaded. No artifact bypasses balance invariants.
func ValidateSnapshot(snapshot Snapshot) error {
	if snapshot.Source != SourceOKXSpotReadOnly {
		return fmt.Errorf("unexpected snapshot source")
	}
	seen := map[string]bool{}
	for _, asset := range snapshot.Assets {
		ccy := strings.ToUpper(strings.TrimSpace(asset.Currency))
		if ccy == "" || seen[ccy] {
			return fmt.Errorf("okx balance currency invalid")
		}
		seen[ccy] = true
		a, err := decimal(asset.Available)
		if err != nil {
			return fmt.Errorf("%s available: %w", ccy, err)
		}
		f, err := decimal(asset.Frozen)
		if err != nil {
			return fmt.Errorf("%s frozen: %w", ccy, err)
		}
		t, err := decimal(asset.Total)
		if err != nil {
			return fmt.Errorf("%s total: %w", ccy, err)
		}
		if new(big.Rat).Add(a, f).Cmp(t) != 0 {
			return fmt.Errorf("%s balance total mismatch", ccy)
		}
		want := ThesisUnlinked
		if ccy == "USDT" {
			want = ThesisNotApplicable
		}
		if asset.ThesisLink != want {
			return fmt.Errorf("%s thesis link invalid", ccy)
		}
	}
	sort.Slice(snapshot.Assets, func(i, j int) bool { return snapshot.Assets[i].Currency < snapshot.Assets[j].Currency })
	return nil
}

func decimal(value string) (*big.Rat, error) {
	value = strings.TrimSpace(value)
	v, ok := new(big.Rat).SetString(value)
	if !ok || v.Sign() < 0 {
		return nil, fmt.Errorf("non-negative decimal required")
	}
	return v, nil
}
