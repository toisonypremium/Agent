package agent2

import (
	"encoding/json"
	"fmt"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
	"btc-agent/internal/flow"
	"btc-agent/internal/indicators"
	"btc-agent/internal/market"
)

type State string

const (
	StateNoTrade     State = "NO_TRADE"
	StateWatch       State = "WATCH"
	StateArmed       State = "ARMED"
	StateActiveLimit State = "ACTIVE_LIMIT"
)

type Layer struct {
	Index    int     `json:"index"`
	Fraction float64 `json:"fraction"`
	Price    float64 `json:"price"`
	Notional float64 `json:"notional"`
	Quantity float64 `json:"quantity"`
}

type AssetPlan struct {
	Symbol         string      `json:"symbol"`
	State          State       `json:"state"`
	DiscountZone   market.Zone `json:"discount_zone"`
	Invalidation   float64     `json:"invalidation"`
	RewardRisk     float64     `json:"reward_risk"`
	RotationRank   int         `json:"rotation_rank,omitempty"`
	RotationScore  float64     `json:"rotation_score,omitempty"`
	AssetFlowBias  flow.Bias   `json:"asset_flow_bias,omitempty"`
	AssetFlowScore float64     `json:"asset_flow_score,omitempty"`
	Layers         []Layer     `json:"layers"`
	Reason         string      `json:"reason"`
}

type Plan struct {
	Timestamp        time.Time            `json:"timestamp"`
	State            State                `json:"state"`
	ActionPermission agent1.Permission    `json:"action_permission"`
	Rotation         []AssetRotationScore `json:"rotation,omitempty"`
	Watchlist        WatchlistReport      `json:"watchlist,omitempty"`
	Assets           []AssetPlan          `json:"assets"`
	Warnings         []string             `json:"warnings"`
	Summary          string               `json:"summary"`
}

func BuildPlan(cfg config.Config, a agent1.MarketAnalysis, candles map[string][]market.Candle) Plan {
	return BuildPlanWithBenchmarks(cfg, a, candles, nil)
}

func BuildPlanWithBenchmarks(cfg config.Config, a agent1.MarketAnalysis, candles map[string][]market.Candle, benchmarks map[string][]market.Candle) Plan {
	p := Plan{Timestamp: time.Now(), ActionPermission: a.ActionPermission, State: StateNoTrade}
	benchmark := benchmarkCandles(cfg, benchmarks)
	p.Rotation = RankAssets(cfg, candles, benchmark)
	rotationBySymbol := map[string]AssetRotationScore{}
	for _, r := range p.Rotation {
		rotationBySymbol[r.Symbol] = r
	}
	useAssetFlowEntry := len(benchmark) > 0
	if a.FallingKnifeRisk == agent1.High || a.FomoRisk == agent1.High || a.MarketRegime == "PANIC_SELLING" {
		p.Summary = "Risk filter chặn gom."
		p.Watchlist = BuildWatchlist(cfg, candles, benchmark, p.Rotation, nil)
		AddWatchlistMissing(&p.Watchlist, "BTC risk/regime chưa cho phép gom", cfg)
		return p
	}
	if a.ActionPermission != agent1.Allowed {
		if a.ActionPermission == agent1.Armed || a.ActionPermission == agent1.Watch {
			anyProbe := false
			for _, sym := range cfg.Data.Symbols.Assets {
				ap := planProbeAsset(cfg, sym, candles[sym], benchmark, rotationBySymbol[sym], useAssetFlowEntry)
				p.Assets = append(p.Assets, ap)
				if ap.State == StateArmed {
					anyProbe = true
				}
			}
			p.Watchlist = BuildWatchlist(cfg, candles, benchmark, p.Rotation, p.Assets)
			AddWatchlistMissing(&p.Watchlist, "BTC permission chưa ALLOWED; chỉ cho phép ARMED probe nhỏ", cfg)
			if anyProbe {
				p.State = StateArmed
				p.Summary = "BTC chưa ALLOWED nhưng có ARMED probe candidate chất lượng cao."
			} else {
				p.State = StateWatch
				p.Summary = "BTC chưa ALLOWED; chưa có coin đủ đẹp để tạo probe."
			}
			return p
		}
		p.Summary = "Agent 1 chưa cho phép gom; giữ NO_TRADE/WATCH."
		p.State = StateWatch
		p.Watchlist = BuildWatchlist(cfg, candles, benchmark, p.Rotation, nil)
		AddWatchlistMissing(&p.Watchlist, "BTC permission chưa ALLOWED", cfg)
		return p
	}
	if a.MarketRegime == "DOWNTREND" {
		p.Summary = "Risk filter chặn gom."
		p.Watchlist = BuildWatchlist(cfg, candles, benchmark, p.Rotation, nil)
		AddWatchlistMissing(&p.Watchlist, "BTC risk/regime chưa cho phép gom", cfg)
		return p
	}

	anyActive := false
	for _, sym := range cfg.Data.Symbols.Assets {
		ap := planAsset(cfg, sym, candles[sym], benchmark, rotationBySymbol[sym], useAssetFlowEntry)
		p.Assets = append(p.Assets, ap)
		if ap.State == StateActiveLimit {
			anyActive = true
		}
	}
	p.Watchlist = BuildWatchlist(cfg, candles, benchmark, p.Rotation, p.Assets)
	if anyActive {
		p.State = StateActiveLimit
		p.Summary = "Có paper limit plan hợp lệ."
	} else {
		p.State = StateWatch
		p.Summary = "Chưa có asset đủ discount/reward-risk."
	}
	return p
}

func planProbeAsset(cfg config.Config, sym string, c []market.Candle, benchmark []market.Candle, rotation AssetRotationScore, useAssetFlowEntry bool) AssetPlan {
	probeCfg := cfg
	if probeCfg.Risk.MinRewardRisk > 0 {
		probeCfg.Risk.MinRewardRisk *= 0.80
	}
	ap := planAsset(probeCfg, sym, c, benchmark, rotation, useAssetFlowEntry)
	if ap.State != StateActiveLimit || len(ap.Layers) == 0 {
		if ap.Symbol == "" {
			ap.Symbol = sym
		}
		return ap
	}
	ap.State = StateArmed
	ap.Reason = "BTC chưa ALLOWED nhưng coin setup rất đẹp; tạo ARMED probe layer"
	ap.Layers = ap.Layers[:1]
	notional := probeNotional(cfg)
	ap.Layers[0].Fraction = 1
	ap.Layers[0].Notional = notional
	if ap.Layers[0].Price > 0 {
		ap.Layers[0].Quantity = notional / ap.Layers[0].Price
	}
	return ap
}

func probeNotional(cfg config.Config) float64 {
	if cfg.Live.MaxLiveNotionalPerOrderUSDT > 0 {
		return cfg.Live.MaxLiveNotionalPerOrderUSDT
	}
	if cfg.Live.CanaryMaxNotionalUSDT > 0 {
		return cfg.Live.CanaryMaxNotionalUSDT
	}
	if cfg.Live.MaxOrderNotionalUSDT > 0 {
		return cfg.Live.MaxOrderNotionalUSDT
	}
	return 1
}

func planAsset(cfg config.Config, sym string, c []market.Candle, benchmark []market.Candle, rotation AssetRotationScore, useAssetFlowEntry bool) AssetPlan {
	ap := AssetPlan{Symbol: sym, State: StateWatch, Reason: "chưa đủ dữ liệu hoặc chưa vào discount zone"}
	if len(c) < 60 {
		return ap
	}
	price := market.LastClose(c)
	support, resistance := market.RangeZone(c, 60)
	closes := make([]float64, len(c))
	for i, candle := range c {
		closes[i] = candle.Close
	}
	ema20 := indicators.Last(indicators.EMA(closes, 20))
	rsi := indicators.Last(indicators.RSI(closes, 14))

	if FallingKnife(c) {
		ap.State = StateNoTrade
		ap.Reason = "falling knife filter chặn asset"
		return ap
	}
	if FOMO(c, ema20, rsi, resistance) {
		ap.State = StateNoTrade
		ap.Reason = "FOMO filter chặn asset"
		return ap
	}
	if enabled, lookback, minRelative, minMomentum := relativeStrengthParams(cfg); enabled && len(benchmark) > 0 {
		rs := RelativeStrength(c, benchmark, lookback, minRelative, minMomentum)
		if !rs.Pass {
			ap.State = StateNoTrade
			ap.Reason = rs.Reason
			return ap
		}
	}
	if enabled, minScore, maxRank := rotationParams(cfg); enabled && rotation.Symbol != "" {
		ap.RotationRank = rotation.Rank
		ap.RotationScore = rotation.Score
		if !rotation.Eligible || rotation.Score < minScore || (maxRank > 0 && rotation.Rank > maxRank) {
			ap.State = StateWatch
			ap.Reason = fmt.Sprintf("rotation score filter chặn asset: rank=%d score=%.2f reason=%s", rotation.Rank, rotation.Score, rotation.Reason)
			return ap
		}
	}
	if enabled, minBull, allowNeutral := assetFlowEntryParams(cfg); enabled && useAssetFlowEntry {
		entry := AssetFlowEntry(sym, c, minBull, allowNeutral)
		ap.AssetFlowBias = entry.Bias
		ap.AssetFlowScore = entry.BullScore
		if !entry.Pass {
			if entry.HardBlock {
				ap.State = StateNoTrade
			} else {
				ap.State = StateWatch
			}
			ap.Reason = entry.Reason
			return ap
		}
	}
	if !support.Valid() || price > support.High*(1+discountZonePremiumPct(cfg)) {
		ap.State = StateWatch
		ap.DiscountZone = support
		ap.Reason = "giá chưa vào discount zone"
		return ap
	}

	invalidation := support.Low * 0.985
	rr := RewardRiskBreakdown(RewardRiskInput{Entry: price, Invalidation: invalidation, Target: resistance.High})
	if !rr.Valid {
		ap.Reason = "reward/risk không hợp lệ: " + rr.Reason
		return ap
	}
	ap.DiscountZone = support
	ap.Invalidation = invalidation
	ap.RewardRisk = rr.Ratio
	if ap.RewardRisk < cfg.Risk.MinRewardRisk {
		ap.State = StateWatch
		ap.Reason = fmt.Sprintf("reward/risk %.2f thấp hơn %.2f", ap.RewardRisk, cfg.Risk.MinRewardRisk)
		return ap
	}

	budget := cfg.Portfolio.TotalCapital * cfg.Portfolio.Allocation[sym] * cfg.Risk.MaxTotalDeploymentPerCycle
	if maxBudget := cfg.Portfolio.TotalCapital * cfg.Risk.MaxSingleAssetDeployment; budget > maxBudget {
		budget = maxBudget
	}
	prices := []float64{support.High, support.Mid(), support.Low}
	for i, fraction := range cfg.Execution.LayerDistribution {
		px := prices[min(i, len(prices)-1)]
		notional := budget * fraction
		ap.Layers = append(ap.Layers, Layer{Index: i + 1, Fraction: fraction, Price: px, Notional: notional, Quantity: notional / px})
	}
	ap.State = StateActiveLimit
	ap.Reason = "đủ discount zone và reward/risk; tạo paper limit layers"
	return ap
}

func benchmarkCandles(cfg config.Config, benchmarks map[string][]market.Candle) []market.Candle {
	if len(benchmarks) == 0 {
		return nil
	}
	if c := benchmarks[cfg.Data.Symbols.BTC]; len(c) > 0 {
		return c
	}
	return benchmarks["BTCUSDT"]
}

func relativeStrengthParams(cfg config.Config) (bool, int, float64, float64) {
	if cfg.Risk.DisableRelativeStrengthFilter {
		return false, 0, 0, 0
	}
	lookback := cfg.Risk.RelativeStrengthLookbackDays
	if lookback <= 0 {
		lookback = 14
	}
	minRelative := cfg.Risk.MinRelativeStrength
	if minRelative == 0 {
		minRelative = -0.03
	}
	minMomentum := cfg.Risk.MinAssetMomentum
	if minMomentum == 0 {
		minMomentum = -0.05
	}
	return true, lookback, minRelative, minMomentum
}

func discountZonePremiumPct(cfg config.Config) float64 {
	if cfg.Risk.DiscountZonePremiumPct > 0 {
		return cfg.Risk.DiscountZonePremiumPct
	}
	return 0.05
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (p Plan) JSON() string {
	b, _ := json.MarshalIndent(p, "", "  ")
	return string(b)
}

func Summary(p Plan) string {
	s := fmt.Sprintf("- Trạng thái: %s\n- Có đặt lệnh không? %v\n", p.State, p.State == StateActiveLimit)
	if len(p.Rotation) > 0 {
		s += "- Asset ranking:\n"
		for _, r := range p.Rotation {
			s += fmt.Sprintf("  - #%d %s score %.2f rel %.2f%% flow %s | %s\n", r.Rank, r.Symbol, r.Score, r.RelativeReturn*100, r.FlowBias, r.Reason)
		}
	}
	if len(p.Watchlist.Candidates) > 0 {
		s += "- Watchlist gần đạt điều kiện:\n"
		limit := len(p.Watchlist.Candidates)
		if limit > 3 {
			limit = 3
		}
		for _, c := range p.Watchlist.Candidates[:limit] {
			s += fmt.Sprintf("  - %s readiness %.2f tier %s actionable %v | checklist: %s | next: %s\n", c.Symbol, c.ReadinessScore, c.Tier, c.Actionable, ChecklistSummary(c.EntryChecklist), c.NextTrigger)
		}
	}
	for _, asset := range p.Assets {
		s += fmt.Sprintf("- %s: %s | rank %d score %.2f | asset flow %s %.2f | RR %.2f | %s\n", asset.Symbol, asset.State, asset.RotationRank, asset.RotationScore, asset.AssetFlowBias, asset.AssetFlowScore, asset.RewardRisk, asset.Reason)
	}
	return s
}
