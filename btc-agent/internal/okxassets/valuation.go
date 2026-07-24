package okxassets

import (
	"fmt"
	"math/big"
	"sort"
)

const (
	ValuationVerified    = "dinh_gia"
	ValuationUnavailable = "chua_dinh_gia"
)

// ApplyUSDTPrices enriches an already validated Spot observation. Missing public
// prices are explicit and never become zero-value estimates.
func ApplyUSDTPrices(snapshot Snapshot, prices map[string]string) (Snapshot, []string, error) {
	if err := ValidateSnapshot(snapshot); err != nil {
		return Snapshot{}, nil, err
	}
	warnings := []string{}
	for i := range snapshot.Assets {
		asset := &snapshot.Assets[i]
		price := prices[asset.Currency]
		if asset.Currency == "USDT" {
			price = "1"
		}
		if price == "" {
			asset.ValuationState = ValuationUnavailable
			warnings = append(warnings, fmt.Sprintf("%s chưa có giá USDT đã xác minh.", asset.Currency))
			continue
		}
		p, err := decimal(price)
		if err != nil || p.Sign() <= 0 {
			return Snapshot{}, nil, fmt.Errorf("%s USDT price invalid", asset.Currency)
		}
		quantity, err := decimal(asset.Total)
		if err != nil {
			return Snapshot{}, nil, err
		}
		asset.PriceUSDT, asset.ValueUSDT, asset.ValuationState = formatDecimal(p), formatDecimal(new(big.Rat).Mul(quantity, p)), ValuationVerified
	}
	sort.Slice(snapshot.Assets, func(i, j int) bool {
		left, _ := decimal(snapshot.Assets[i].ValueUSDT)
		right, _ := decimal(snapshot.Assets[j].ValueUSDT)
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		return left.Cmp(right) > 0
	})
	return snapshot, warnings, nil
}
