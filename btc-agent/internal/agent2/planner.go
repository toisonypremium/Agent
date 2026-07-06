package agent2

import (
	"encoding/json"
	"fmt"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
	"btc-agent/internal/flow"
	"btc-agent/internal/indicators"
	"btc-agent/internal/liquidity"
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
	Index        int       `json:"index"`
	Fraction     float64   `json:"fraction"`
	Price        float64   `json:"price"`
	Notional     float64   `json:"notional"`
	Quantity     float64   `json:"quantity"`
	Invalidation float64   `json:"invalidation,omitempty"`
	Target       float64   `json:"target,omitempty"`
	RewardRisk   float64   `json:"reward_risk,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Reason       string    `json:"reason,omitempty"`
}

type AssetPlan struct {
	Symbol           string            `json:"symbol"`
	State            State             `json:"state"`
	DiscountZone     market.Zone       `json:"discount_zone"`
	Invalidation     float64           `json:"invalidation"`
	RewardRisk       float64           `json:"reward_risk"`
	RewardRiskDetail RewardRiskResult  `json:"reward_risk_detail,omitempty"`
	ZoneWidthPct     float64           `json:"zone_width_pct,omitempty"`
	DiscountGapPct   float64           `json:"discount_gap_pct,omitempty"`
	ZoneQuality      string            `json:"zone_quality,omitempty"`
	RotationRank     int               `json:"rotation_rank,omitempty"`
	RotationScore    float64           `json:"rotation_score,omitempty"`
	AssetFlowBias    flow.Bias         `json:"asset_flow_bias,omitempty"`
	AssetFlowScore   float64           `json:"asset_flow_score,omitempty"`
	MMCase           MMCase            `json:"mm_case,omitempty"`
	MMScore          float64           `json:"mm_score,omitempty"`
	MMReasons        []string          `json:"mm_reasons,omitempty"`
	MMMissing        []string          `json:"mm_missing,omitempty"`
	LiquidityQuality liquidity.Quality `json:"liquidity_quality,omitempty"`
	Layers           []Layer           `json:"layers"`
	Reason           string            `json:"reason"`
	HardBlockers     []string          `json:"hard_blockers,omitempty"`
	SoftBlockers     []string          `json:"soft_blockers,omitempty"`
	NextTrigger      string            `json:"next_trigger,omitempty"`
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
		if a.ActionPermission == agent1.Armed {
			anyProbe := false
			for _, sym := range cfg.Data.Symbols.Assets {
				ap := planProbeAsset(cfg, sym, candles[sym], benchmark, rotationBySymbol[sym], useAssetFlowEntry)
				p.Assets = append(p.Assets, ap)
				if ap.State == StateArmed {
					anyProbe = true
				}
			}
			p.Watchlist = BuildWatchlist(cfg, candles, benchmark, p.Rotation, p.Assets)
			AddWatchlistMissing(&p.Watchlist, "BTC permission ARMED; chỉ cho phép probe nhỏ khi mọi gate con đạt", cfg)
			if anyProbe {
				p.State = StateArmed
				p.Summary = "BTC ARMED và có probe candidate chất lượng cao."
			} else {
				p.State = StateWatch
				p.Summary = "BTC ARMED nhưng chưa có coin đủ đẹp để tạo probe."
			}
			return p
		}
		p.Summary = "BTC permission WATCH/NO_TRADE; không tạo probe."
		p.State = StateWatch
		for _, sym := range cfg.Data.Symbols.Assets {
			ap := AssetPlan{Symbol: sym, State: StateWatch, Reason: "BTC permission WATCH; không tạo probe", HardBlockers: []string{"BTC permission WATCH; không tạo probe"}, NextTrigger: "Chờ BTC chuyển ARMED hoặc ALLOWED trước khi tạo live order."}
			if c := candles[sym]; len(c) >= 25 {
				mm := AnalyzeMMAccumulation(sym, c)
				ap.MMCase = mm.Case
				ap.MMScore = mm.Score
				ap.MMReasons = mm.Reasons
				ap.MMMissing = mm.Missing
				ap.AssetFlowBias = mm.FlowBias
				ap.AssetFlowScore = mm.BullScore
				ap.LiquidityQuality = liquidity.EvaluateCandleProxy(cfg, sym, c, desiredLiquidityNotional(cfg, sym))
			}
			p.Assets = append(p.Assets, ap)
		}
		p.Watchlist = BuildWatchlist(cfg, candles, benchmark, p.Rotation, p.Assets)
		AddWatchlistMissing(&p.Watchlist, "BTC permission WATCH; không tạo probe", cfg)
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
	ap := planAsset(cfg, sym, c, benchmark, rotation, useAssetFlowEntry)
	if ap.State != StateActiveLimit || len(ap.Layers) == 0 {
		if ap.Symbol == "" {
			ap.Symbol = sym
		}
		return ap
	}
	ap.State = StateArmed
	ap.Reason = "BTC ARMED và coin setup đủ gate; tạo 1 probe layer nhỏ"
	ap.SoftBlockers = uniqueStrings(append(ap.SoftBlockers, "BTC mới ARMED nên chỉ sizing probe"))
	ap.NextTrigger = "Probe post-only nhỏ; chỉ mở rộng ladder khi BTC chuyển ALLOWED."
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
	ap := AssetPlan{Symbol: sym, State: StateWatch, Reason: "chưa đủ dữ liệu hoặc chưa vào discount zone", NextTrigger: "Chờ đủ dữ liệu và giá về discount zone."}
	if len(c) < 60 {
		ap.HardBlockers = []string{"chưa đủ dữ liệu 1D"}
		ap.NextTrigger = "Chờ đủ dữ liệu nến 1D trước khi đánh giá candidate."
		return ap
	}
	price := market.LastClose(c)
	support, resistance := actionSupportResistanceZones(c)
	closes := make([]float64, len(c))
	for i, candle := range c {
		closes[i] = candle.Close
	}
	ema20 := indicators.Last(indicators.EMA(closes, 20))
	rsi := indicators.Last(indicators.RSI(closes, 14))

	ap.DiscountZone = support
	ap.ZoneWidthPct = zoneWidthPct(support)
	ap.DiscountGapPct = discountGapPct(price, support)
	ap.ZoneQuality = zoneQuality(support)
	mm := AnalyzeMMAccumulation(sym, c)
	ap.MMCase = mm.Case
	ap.MMScore = mm.Score
	ap.MMReasons = mm.Reasons
	ap.MMMissing = mm.Missing
	ap.LiquidityQuality = liquidity.EvaluateCandleProxy(cfg, sym, c, desiredLiquidityNotional(cfg, sym))
	if ap.ZoneQuality != "ZONE_OK" {
		ap.State = StateWatch
		ap.Reason = zoneQualityReason(ap.ZoneQuality, ap.ZoneWidthPct)
		ap.SoftBlockers = append(ap.SoftBlockers, ap.Reason)
		ap.NextTrigger = "Chờ vùng support/discount hẹp và rõ hơn để tính entry/RR."
		return ap
	}

	if FallingKnife(c) {
		ap.State = StateNoTrade
		ap.Reason = "falling knife filter chặn asset"
		ap.HardBlockers = append(ap.HardBlockers, "falling knife risk")
		ap.NextTrigger = "Chờ cấu trúc ngừng lower-low và reclaim support rõ."
		return ap
	}
	if FOMO(c, ema20, rsi, resistance) {
		ap.State = StateNoTrade
		ap.Reason = "FOMO filter chặn asset"
		ap.HardBlockers = append(ap.HardBlockers, "FOMO risk")
		ap.NextTrigger = "Không đuổi giá; chờ pullback về value/support."
		return ap
	}
	if enabled, lookback, minRelative, minMomentum := relativeStrengthParams(cfg); enabled && len(benchmark) > 0 {
		rs := RelativeStrength(c, benchmark, lookback, minRelative, minMomentum)
		if !rs.Pass {
			ap.State = StateNoTrade
			ap.Reason = rs.Reason
			ap.HardBlockers = append(ap.HardBlockers, rs.Reason)
			ap.NextTrigger = "Chờ asset ngừng underperform BTC trong lookback."
			return ap
		}
	}
	if enabled, minScore, maxRank := rotationParams(cfg); enabled && rotation.Symbol != "" {
		ap.RotationRank = rotation.Rank
		ap.RotationScore = rotation.Score
		if !rotation.Eligible || rotation.Score < minScore || (maxRank > 0 && rotation.Rank > maxRank) {
			ap.State = StateWatch
			ap.Reason = fmt.Sprintf("rotation score filter chặn asset: rank=%d score=%.2f reason=%s", rotation.Rank, rotation.Score, rotation.Reason)
			ap.SoftBlockers = append(ap.SoftBlockers, ap.Reason)
			ap.NextTrigger = "Chờ rotation score tăng hoặc rank vào top được phép."
			return ap
		}
	}
	if mm.HardBlock {
		ap.State = StateNoTrade
		ap.Reason = mmReason(mm)
		ap.HardBlockers = append(ap.HardBlockers, ap.Reason)
		ap.NextTrigger = mm.NextTrigger
		return ap
	}
	if enabled, minBull, allowNeutral := assetFlowEntryParams(cfg); enabled && useAssetFlowEntry {
		entry := AssetFlowEntryFromMM(mm, minBull, allowNeutral)
		ap.AssetFlowBias = entry.Bias
		ap.AssetFlowScore = entry.BullScore
		if !entry.Pass {
			if entry.HardBlock {
				ap.State = StateNoTrade
				ap.HardBlockers = append(ap.HardBlockers, entry.Reason)
			} else {
				ap.State = StateWatch
				ap.SoftBlockers = append(ap.SoftBlockers, entry.Reason)
			}
			ap.Reason = entry.Reason
			ap.NextTrigger = firstNonEmptyMM(entry.NextTrigger, "Chờ sweep low + reclaim support hoặc absorption volume gần support.")
			return ap
		}
	}
	if cfg.Live.LiquidityGateEnabled && ap.LiquidityQuality.Enabled && !ap.LiquidityQuality.Pass {
		ap.State = StateWatch
		ap.Reason = "liquidity gate blocked: " + liquidity.FirstReason(ap.LiquidityQuality.Reasons)
		ap.SoftBlockers = append(ap.SoftBlockers, ap.Reason)
		ap.NextTrigger = "Chờ spread/depth/volume đủ dày trước khi tạo live layer."
		return ap
	}
	if !support.Valid() || price > support.High*(1+discountZonePremiumPct(cfg)) {
		ap.State = StateWatch
		ap.DiscountZone = support
		ap.Reason = fmt.Sprintf("giá chưa vào discount zone: cao hơn support %.2f%%", ap.DiscountGapPct*100)
		ap.SoftBlockers = append(ap.SoftBlockers, ap.Reason)
		ap.NextTrigger = "Chờ giá về support/discount zone mà không tạo falling knife."
		return ap
	}

	invalidation := support.Low * 0.985
	rr := RewardRiskBreakdown(RewardRiskInput{Entry: price, Invalidation: invalidation, Target: resistance.High})
	ap.RewardRiskDetail = rr
	if !rr.Valid {
		ap.Reason = "reward/risk không hợp lệ: " + rr.Reason
		ap.SoftBlockers = append(ap.SoftBlockers, ap.Reason)
		ap.NextTrigger = "Chờ entry sâu hơn hoặc target rõ hơn để tính RR."
		return ap
	}
	ap.DiscountZone = support
	ap.Invalidation = invalidation
	ap.RewardRisk = rr.Ratio
	if ap.RewardRisk < cfg.Risk.MinRewardRisk {
		ap.State = StateWatch
		ap.Reason = fmt.Sprintf("reward/risk %.2f thấp hơn %.2f; risk %.4f reward %.4f", ap.RewardRisk, cfg.Risk.MinRewardRisk, rr.Risk, rr.Reward)
		ap.SoftBlockers = append(ap.SoftBlockers, ap.Reason)
		ap.NextTrigger = "Chờ entry sâu hơn hoặc resistance mở rộng để RR đạt ngưỡng."
		return ap
	}

	budget := cfg.Portfolio.TotalCapital * cfg.Portfolio.Allocation[sym] * cfg.Risk.MaxTotalDeploymentPerCycle
	if maxBudget := cfg.Portfolio.TotalCapital * cfg.Risk.MaxSingleAssetDeployment; budget > maxBudget {
		budget = maxBudget
	}
	ap.Layers = buildEntryLayers(cfg, support, resistance, invalidation, budget)
	if len(ap.Layers) == 0 {
		ap.State = StateWatch
		ap.Reason = "không có layer nào đạt reward/risk tối thiểu"
		ap.SoftBlockers = append(ap.SoftBlockers, ap.Reason)
		ap.NextTrigger = "Chờ entry sâu hơn để layer đạt reward/risk."
		return ap
	}
	ap.State = StateActiveLimit
	ap.Reason = "đủ discount zone và reward/risk; tạo paper limit layers"
	ap.NextTrigger = "Layer hợp lệ; live manager kiểm tra preflight/caps trước khi đặt post-only."
	return ap
}

func actionSupportResistanceZones(c []market.Candle) (market.Zone, market.Zone) {
	support, resistance := market.RangeZone(c, 60)
	return market.CapZoneWidth(support, 1.25), resistance
}

func zoneWidthPct(z market.Zone) float64 {
	return market.ZoneWidthPct(z)
}

func discountGapPct(price float64, support market.Zone) float64 {
	if price <= 0 || !support.Valid() {
		return 0
	}
	if price > support.High {
		return price/support.High - 1
	}
	if price < support.Low {
		return price/support.Low - 1
	}
	return 0
}

func zoneQuality(z market.Zone) string {
	if !z.Valid() {
		return "ZONE_INVALID"
	}
	if zoneWidthPct(z) > 0.25 {
		return "ZONE_WARN_WIDE"
	}
	return "ZONE_OK"
}

func zoneQualityReason(quality string, width float64) string {
	switch quality {
	case "ZONE_INVALID":
		return "support zone invalid"
	case "ZONE_WARN_WIDE":
		return fmt.Sprintf("support zone rộng %.1f%%; cần vùng entry hẹp hơn", width*100)
	default:
		return "support zone OK"
	}
}

func buildEntryLayers(cfg config.Config, support, resistance market.Zone, invalidation, budget float64) []Layer {
	prices := []float64{support.High, support.Mid(), support.Low}
	expires := time.Time{}
	if cfg.Execution.OrderExpiryHours > 0 {
		expires = time.Now().Add(time.Duration(cfg.Execution.OrderExpiryHours) * time.Hour)
	}
	layers := []Layer{}
	for i, fraction := range cfg.Execution.LayerDistribution {
		px := prices[min(i, len(prices)-1)]
		notional := budget * fraction
		rr := RewardRiskBreakdown(RewardRiskInput{Entry: px, Invalidation: invalidation, Target: resistance.High})
		if !rr.Valid || rr.Ratio < cfg.Risk.MinRewardRisk || px <= 0 || notional <= 0 {
			continue
		}
		layers = append(layers, Layer{
			Index:        i + 1,
			Fraction:     fraction,
			Price:        px,
			Notional:     notional,
			Quantity:     notional / px,
			Invalidation: invalidation,
			Target:       resistance.High,
			RewardRisk:   rr.Ratio,
			ExpiresAt:    expires,
			Reason:       fmt.Sprintf("layer %d tại support %.4f, target %.4f, RR %.2f", i+1, px, resistance.High, rr.Ratio),
		})
	}
	return layers
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

func desiredLiquidityNotional(cfg config.Config, sym string) float64 {
	if cfg.Live.MaxLiveNotionalPerOrderUSDT > 0 {
		return cfg.Live.MaxLiveNotionalPerOrderUSDT
	}
	if cfg.Live.CanaryMaxNotionalUSDT > 0 {
		return cfg.Live.CanaryMaxNotionalUSDT
	}
	if cfg.Live.MaxOrderNotionalUSDT > 0 {
		return cfg.Live.MaxOrderNotionalUSDT
	}
	if cfg.Portfolio.TotalCapital > 0 && cfg.Portfolio.Allocation != nil && cfg.Portfolio.Allocation[sym] > 0 && cfg.Risk.MaxTotalDeploymentPerCycle > 0 {
		return cfg.Portfolio.TotalCapital * cfg.Portfolio.Allocation[sym] * cfg.Risk.MaxTotalDeploymentPerCycle
	}
	return 1
}

func firstNonEmptyMM(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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
