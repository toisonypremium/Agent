package macroflow

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type Regime string
type Action string

const (
	RegimeUnknown            Regime = "UNKNOWN"
	RegimeRiskOff            Regime = "RISK_OFF"
	RegimeBTCLed             Regime = "BTC_LED"
	RegimeEarlyAltRotation   Regime = "EARLY_ALT_ROTATION"
	RegimeSelectiveRiskOn    Regime = "SELECTIVE_RISK_ON"
	RegimeConfirmedAltseason Regime = "CONFIRMED_ALTSEASON"
	ActionBlockNewExposure   Action = "BLOCK_NEW_EXPOSURE"
	ActionBTCOnlyWatch       Action = "BTC_ONLY_WATCH"
	ActionWaitPullback       Action = "WAIT_PULLBACK"
	ActionRankAltCandidates  Action = "RANK_ALT_CANDIDATES"
)

type Input struct {
	BTCDominancePct                float64 `json:"btc_dominance_pct"`
	BTCDominanceChange24hPctPoint  float64 `json:"btc_dominance_change_24h_pct_point"`
	USDTDominancePct               float64 `json:"usdt_dominance_pct"`
	USDTDominanceChange24hPctPoint float64 `json:"usdt_dominance_change_24h_pct_point"`
	BTCChange24hPct                float64 `json:"btc_change_24h_pct"`
	ETHChange24hPct                float64 `json:"eth_change_24h_pct"`
	BTCChange7dPct                 float64 `json:"btc_change_7d_pct"`
	ETHChange7dPct                 float64 `json:"eth_change_7d_pct"`
	BreadthSampleSize              int     `json:"breadth_sample_size"`
	BreadthAdvancers24h            int     `json:"breadth_advancers_24h"`
	MedianReturn24hPct             float64 `json:"median_return_24h_pct"`
	AltWeightedReturn24hPct        float64 `json:"alt_weighted_return_24h_pct"`
	GlobalVolumeChange24hPct       float64 `json:"global_volume_change_24h_pct"`
	USDTSupplyChange30dPct         float64 `json:"usdt_supply_change_30d_pct"`
	StableSupplyKnown              bool    `json:"stable_supply_known"`
	Total2MarketCapUSD             float64 `json:"total2_market_cap_usd"`
	Total3MarketCapUSD             float64 `json:"total3_market_cap_usd"`
	Total2Change24hPct             float64 `json:"total2_change_24h_pct"`
	Total3Change24hPct             float64 `json:"total3_change_24h_pct"`
	Fresh                          bool    `json:"fresh"`
}

type Result struct {
	Input                Input    `json:"input"`
	Regime               Regime   `json:"regime"`
	Action               Action   `json:"action"`
	Confidence           float64  `json:"confidence"`
	BreadthRatio         float64  `json:"breadth_ratio"`
	ETHLeading           bool     `json:"eth_leading"`
	StableLiquidityUp    bool     `json:"stable_liquidity_up"`
	StableLiquidityKnown bool     `json:"stable_liquidity_known"`
	CanGrantExecution    bool     `json:"can_grant_execution"`
	Blocks               []string `json:"blocks,omitempty"`
	Evidence             []string `json:"evidence,omitempty"`
}

func Evaluate(in Input) Result {
	r := Result{Input: in, Regime: RegimeUnknown, Action: ActionBlockNewExposure, CanGrantExecution: false}
	if in.BreadthSampleSize > 0 {
		r.BreadthRatio = float64(in.BreadthAdvancers24h) / float64(in.BreadthSampleSize)
	}
	r.ETHLeading = in.ETHChange24hPct > in.BTCChange24hPct && in.ETHChange7dPct > in.BTCChange7dPct
	r.StableLiquidityKnown = in.StableSupplyKnown
	r.StableLiquidityUp = in.StableSupplyKnown && in.USDTSupplyChange30dPct > 0
	if !in.Fresh || in.BreadthSampleSize < 30 || !finite(in.BTCDominancePct, in.USDTDominancePct) {
		r.Blocks = []string{"macro-flow data missing, stale, or insufficient breadth"}
		return r
	}
	r.Evidence = append(r.Evidence,
		fmt.Sprintf("breadth=%d/%d", in.BreadthAdvancers24h, in.BreadthSampleSize),
		fmt.Sprintf("btc_d_change=%.2fpp usdt_d_change=%.2fpp", in.BTCDominanceChange24hPctPoint, in.USDTDominanceChange24hPctPoint),
		fmt.Sprintf("eth_minus_btc_24h=%.2fpp volume_change=%.2f%%", in.ETHChange24hPct-in.BTCChange24hPct, in.GlobalVolumeChange24hPct),
	)
	if r.BreadthRatio < .40 || in.MedianReturn24hPct < -1 {
		r.Regime, r.Action, r.Confidence = RegimeRiskOff, ActionBlockNewExposure, .75
		r.Blocks = []string{"weak market breadth"}
		return r
	}
	if in.BTCDominanceChange24hPctPoint > .15 && !r.ETHLeading {
		r.Regime, r.Action, r.Confidence = RegimeBTCLed, ActionBTCOnlyWatch, .65
		return r
	}
	early := r.BreadthRatio >= .65 && in.MedianReturn24hPct > 0 && in.BTCDominanceChange24hPctPoint < 0 && in.USDTDominanceChange24hPctPoint < 0 && r.ETHLeading
	if early {
		r.Regime, r.Action, r.Confidence = RegimeEarlyAltRotation, ActionWaitPullback, .72
		if !r.StableLiquidityKnown {
			r.Evidence = append(r.Evidence, "USDT 30d supply history unavailable")
		} else if !r.StableLiquidityUp {
			r.Evidence = append(r.Evidence, "USDT 30d supply not expanding; rotation may be recycled liquidity")
		}
		return r
	}
	r.Regime, r.Action, r.Confidence = RegimeSelectiveRiskOn, ActionRankAltCandidates, .55
	return r
}

func finite(vs ...float64) bool {
	for _, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
			return false
		}
	}
	return true
}

type GlobalSnapshot struct {
	MarketCapUSD          float64
	MarketCapChange24hPct float64
	VolumeChange24hPct    float64
	UpdatedAtUnix         int64
}

type AssetSnapshot struct {
	Symbol                string
	MarketCapUSD          float64
	MarketCapChange24hPct float64
	Change24hPct          float64
	Change7dPct           float64
	Stable                bool
}

type SupplyPoint struct {
	Unix int64
	USD  float64
}

func BuildInput(g GlobalSnapshot, assets []AssetSnapshot, supply []SupplyPoint, nowUnix int64, maxAgeSeconds int64) (Input, error) {
	if g.MarketCapUSD <= 0 {
		return Input{}, fmt.Errorf("global market cap must be positive")
	}
	in := Input{GlobalVolumeChange24hPct: g.VolumeChange24hPct, Fresh: nowUnix >= g.UpdatedAtUnix && nowUnix-g.UpdatedAtUnix <= maxAgeSeconds}
	returns := make([]float64, 0, len(assets))
	var altWeighted, altCap float64
	var total2Cap, total3Cap, total2Change, total3Change float64
	var total2ChangeCap, total3ChangeCap float64
	for _, a := range assets {
		s := strings.ToLower(strings.TrimSpace(a.Symbol))
		switch s {
		case "btc":
			in.BTCDominancePct = 100 * a.MarketCapUSD / g.MarketCapUSD
			in.BTCChange24hPct, in.BTCChange7dPct = a.Change24hPct, a.Change7dPct
			in.BTCDominanceChange24hPctPoint = dominanceChange(in.BTCDominancePct, a.MarketCapChange24hPct, g.MarketCapChange24hPct)
		case "eth":
			in.ETHChange24hPct, in.ETHChange7dPct = a.Change24hPct, a.Change7dPct
		case "usdt":
			in.USDTDominancePct = 100 * a.MarketCapUSD / g.MarketCapUSD
			in.USDTDominanceChange24hPctPoint = dominanceChange(in.USDTDominancePct, a.MarketCapChange24hPct, g.MarketCapChange24hPct)
		}
		if s != "btc" {
			total2Cap += a.MarketCapUSD
			total2Change += a.MarketCapUSD * a.MarketCapChange24hPct
			total2ChangeCap += a.MarketCapUSD
		}
		if s != "btc" && s != "eth" {
			total3Cap += a.MarketCapUSD
			total3Change += a.MarketCapUSD * a.MarketCapChange24hPct
			total3ChangeCap += a.MarketCapUSD
		}
		if a.Stable {
			continue
		}
		returns = append(returns, a.Change24hPct)
		in.BreadthSampleSize++
		if a.Change24hPct > 0 {
			in.BreadthAdvancers24h++
		}
		if s != "btc" && a.MarketCapUSD > 0 {
			altWeighted += a.MarketCapUSD * a.Change24hPct
			altCap += a.MarketCapUSD
		}
	}
	in.Total2MarketCapUSD, in.Total3MarketCapUSD = total2Cap, total3Cap
	if total2ChangeCap > 0 {
		in.Total2Change24hPct = total2Change / total2ChangeCap
	}
	if total3ChangeCap > 0 {
		in.Total3Change24hPct = total3Change / total3ChangeCap
	}
	if len(returns) > 0 {
		sort.Float64s(returns)
		n := len(returns)
		if n%2 == 1 {
			in.MedianReturn24hPct = returns[n/2]
		} else {
			in.MedianReturn24hPct = (returns[n/2-1] + returns[n/2]) / 2
		}
	}
	if altCap > 0 {
		in.AltWeightedReturn24hPct = altWeighted / altCap
	}
	if known, change := SupplyChange30d(supply); known {
		in.StableSupplyKnown = true
		in.USDTSupplyChange30dPct = change
	}
	return in, nil
}

func SupplyChange30d(points []SupplyPoint) (bool, float64) {
	if len(points) < 2 {
		return false, 0
	}
	newest := points[len(points)-1]
	if newest.USD <= 0 {
		return false, 0
	}
	target := newest.Unix - 30*86400
	old := points[0]
	best := abs64(old.Unix - target)
	for _, p := range points {
		if p.USD > 0 {
			if d := abs64(p.Unix - target); d < best {
				old, best = p, d
			}
		}
	}
	if old.USD <= 0 {
		return false, 0
	}
	return true, 100 * (newest.USD/old.USD - 1)
}

func dominanceChange(current, componentChange, totalChange float64) float64 {
	previous := current * (1 + totalChange/100) / (1 + componentChange/100)
	return current - previous
}
func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// BlocksNewExposure is deliberately one-way: fresh, explicit RISK_OFF evidence
// may veto a new BUY, while bullish or unavailable macro data can never grant it.
func BlocksNewExposure(r Result) bool {
	return r.Input.Fresh && r.Regime == RegimeRiskOff
}
