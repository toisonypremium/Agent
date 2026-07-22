package agent2

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/config"
	"btc-agent/internal/flow"
	"btc-agent/internal/liquidity"
	"btc-agent/internal/market"
)

type State string

const (
	StateNoTrade     State = "NO_TRADE"
	StateWatch       State = "WATCH"
	StateScout       State = "SCOUT"
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
	SetupScore       float64           `json:"setup_score,omitempty"`
	SetupGates       []SetupGateResult `json:"setup_gates,omitempty"`
	Reasons          []DecisionReason  `json:"reasons,omitempty"`
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
	assetSymbols := accumulationAssets(cfg)
	benchmark := benchmarkCandles(cfg, benchmarks)
	p.Rotation = RankAssets(cfg, targetAssetCandles(assetSymbols, candles), benchmark)
	rotationBySymbol := map[string]AssetRotationScore{}
	for _, r := range p.Rotation {
		rotationBySymbol[r.Symbol] = r
	}
	useAssetFlowEntry := len(benchmark) > 0

	anyActive := false
	anyArmed := false
	anyScout := false
	anyHard := false
	for _, sym := range assetSymbols {
		ap := planAsset(cfg, sym, candles[sym], benchmark, rotationBySymbol[sym], useAssetFlowEntry)
		ap = applyBTCGateToAsset(cfg, a, ap)
		p.Assets = append(p.Assets, ap)
		switch ap.State {
		case StateActiveLimit:
			anyActive = true
		case StateArmed:
			anyArmed = true
		case StateScout:
			anyScout = true
		case StateNoTrade:
			if HasHardBlock(ap.Reasons) {
				anyHard = true
			}
		}
	}
	p.Watchlist = BuildWatchlist(cfg, candles, benchmark, p.Rotation, p.Assets)
	switch {
	case anyActive:
		p.State = StateActiveLimit
		p.Summary = "Có paper limit plan hợp lệ."
	case anyArmed:
		p.State = StateArmed
		p.Summary = "BTC ARMED và có asset candidate đủ mạnh; SCOUT/ARMED không tự tạo order."
	case anyScout:
		p.State = StateScout
		p.Summary = "Có SCOUT candidate gần đạt; chưa tạo order vì còn soft wait."
	case anyHard:
		p.State = StateNoTrade
		p.Summary = "Có hard blocker; không tạo order."
	default:
		p.State = StateWatch
		p.Summary = "Chưa có asset đủ discount/reward-risk."
	}
	return p
}

func btcHardBlocks(a agent1.MarketAnalysis) bool {
	return a.MarketRegime == "PANIC_SELLING" || a.FallingKnifeRisk == agent1.High
}

func btcGateReasons(a agent1.MarketAnalysis) []DecisionReason {
	reasons := []DecisionReason{}
	if a.MarketRegime == "PANIC_SELLING" {
		reasons = AddReason(reasons, NewDecisionReason(ReasonBTCPanic, ReasonHardBlock, ReasonScopeBTC, "BTC panic selling hard block"))
	}
	if a.FallingKnifeRisk == agent1.High {
		reasons = AddReason(reasons, NewDecisionReason(ReasonFallingKnife, ReasonHardBlock, ReasonScopeBTC, "BTC falling knife high hard block"))
	}
	if a.FomoRisk == agent1.High {
		reasons = AddReason(reasons, NewDecisionReason(ReasonFOMO, ReasonHardBlock, ReasonScopeBTC, "BTC FOMO high hard block"))
	}
	if a.MarketRegime == "DOWNTREND" {
		reasons = AddReason(reasons, NewDecisionReason(ReasonBTCDowntrend, ReasonSoftWait, ReasonScopeBTC, "BTC downtrend; chỉ scout/watch, không full deploy"))
	}
	if btcPermissionNeedsSoftWait(a) {
		reasons = AddReason(reasons, NewDecisionReason(ReasonBTCPermission, ReasonSoftWait, ReasonScopeBTC, "BTC permission "+string(a.ActionPermission)+"; chưa cho phép ACTIVE_LIMIT"))
	}
	return reasons
}

func btcPermissionNeedsSoftWait(a agent1.MarketAnalysis) bool {
	return !(a.ActionPermission == agent1.Allowed)
}

func applyBTCGateToAsset(cfg config.Config, a agent1.MarketAnalysis, ap AssetPlan) AssetPlan {
	gateReasons := btcGateReasons(a)
	ap.Reasons = append(ap.Reasons, gateReasons...)
	ap.HardBlockers = ReasonMessages(ReasonsBySeverity(ap.Reasons, ReasonHardBlock))
	ap.SoftBlockers = ReasonMessages(ReasonsBySeverity(ap.Reasons, ReasonSoftWait))
	if HasHardBlock(gateReasons) {
		// Exceptional RR Bypass: neu chi falling knife (khong phai PANIC_SELLING/FOMO)
		// va asset co RR cuc cao, ha xuong SCOUT thay vi NO_TRADE.
		if cfg.Risk.ExceptionalRRBypassFallingKnife > 0 &&
			a.FallingKnifeRisk == agent1.High &&
			a.MarketRegime != "PANIC_SELLING" &&
			a.FomoRisk != agent1.High &&
			ap.RewardRisk >= cfg.Risk.ExceptionalRRBypassFallingKnife {
			ap.State = StateScout
			ap.Layers = nil
			ap.Reason = fmt.Sprintf("exceptional RR %.2f >= %.1f: falling knife SCOUT, khong tao lenh", ap.RewardRisk, cfg.Risk.ExceptionalRRBypassFallingKnife)
			ap.NextTrigger = "Exceptional RR bypass: theo doi entry tot hon; khong tao lenh den khi BTC het falling knife."
			return ap
		}
		ap.State = StateNoTrade
		ap.Layers = nil
		ap.Reason = firstNonEmptyMM(PrimaryReason(gateReasons), ap.Reason, "BTC hard blocker")
		ap.NextTrigger = "Chờ BTC hết panic/falling knife/FOMO hard risk trước khi tạo setup."
		return ap
	}
	btcAccumulationConfirmed := string(a.BTCAccumulation.Phase) == "ACCUMULATION_CONFIRMED"
	if a.ActionPermission == agent1.Allowed && btcAccumulationConfirmed && (a.MarketRegime != "DOWNTREND" || cfg.Risk.AllowScoutInDowntrend) {
		if a.MarketRegime != "DOWNTREND" {
			return ap
		}
	}
	if ap.State == StateActiveLimit {
		if a.ActionPermission == agent1.Armed {
			ap.State = StateArmed
			ap.Reason = "BTC ARMED và coin setup đủ gate; giữ ARMED candidate, không tự tạo order"
			ap.NextTrigger = "Chờ BTC chuyển ALLOWED để full ladder; SCOUT/ARMED không tạo order."
			ap.Layers = ap.Layers[:min(1, len(ap.Layers))]
			return ap
		}
		ap.State = StateScout
		ap.Layers = nil
		ap.Reason = firstNonEmptyMM(PrimaryReason(gateReasons), "setup đủ asset gate nhưng BTC chưa ALLOWED/CONFIRMED")
		if a.ActionPermission == agent1.Allowed && !btcAccumulationConfirmed {
			ap.Reason = "BTC chưa ACCUMULATION_CONFIRMED; không cho ACTIVE_LIMIT"
		}
		ap.NextTrigger = "SCOUT: chờ BTC chuyển ALLOWED và ACCUMULATION_CONFIRMED; không tạo order."
		return ap
	}
	if ap.State == StateWatch && !HasHardBlock(ap.Reasons) && nearActionableSetup(cfg, ap) {
		ap.State = StateScout
		ap.Reason = firstNonEmptyMM(ap.Reason, "setup gần đạt; chỉ scout")
		ap.NextTrigger = firstNonEmptyMM(ap.NextTrigger, "SCOUT: theo dõi trigger còn thiếu; không tạo order.")
	}
	return ap
}

func nearActionableSetup(cfg config.Config, ap AssetPlan) bool {
	minNear := cfg.Risk.MinWatchReadinessForProbe
	if minNear <= 0 {
		minNear = 0.70
	}
	return ap.SetupScore >= minNear
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
	if config.LiveAutoMaxNotionalUSDT(cfg) > 0 {
		return config.LiveAutoMaxNotionalUSDT(cfg)
	}
	if cfg.Live.MaxOrderNotionalUSDT > 0 {
		return cfg.Live.MaxOrderNotionalUSDT
	}
	return 1
}

func planAsset(cfg config.Config, sym string, c []market.Candle, benchmark []market.Candle, rotation AssetRotationScore, useAssetFlowEntry bool) AssetPlan {
	ap, eval := evaluateAssetSetup(cfg, sym, c, benchmark, rotation, useAssetFlowEntry)
	if len(eval.HardBlockers) > 0 {
		ap.State = StateNoTrade
		ap.Reason = firstNonEmptyMM(PrimaryReason(ap.Reasons), firstReason(eval.HardBlockers), "setup hard blocker")
		ap.NextTrigger = eval.NextTrigger
		return ap
	}
	if len(eval.SoftBlockers) > 0 {
		ap.State = StateWatch
		ap.Reason = firstNonEmptyMM(primarySetupReason(eval.SoftBlockers), PrimaryReason(ap.Reasons), "setup chưa đủ gate")
		ap.NextTrigger = eval.NextTrigger
		return ap
	}
	budget := cfg.Portfolio.TotalCapital * cfg.Portfolio.Allocation[sym] * cfg.Risk.MaxTotalDeploymentPerCycle
	if maxBudget := cfg.Portfolio.TotalCapital * cfg.Risk.MaxSingleAssetDeployment; budget > maxBudget {
		budget = maxBudget
	}
	ap.Layers = buildEntryLayers(cfg, ap.DiscountZone, ap.RewardRiskDetailToResistance(), ap.Invalidation, budget)
	if len(ap.Layers) == 0 {
		ap.State = StateWatch
		ap.Reason = "không có layer nào đạt reward/risk tối thiểu"
		ap.Reasons = AddReason(ap.Reasons, NewDecisionReason(ReasonExecutionLayer, ReasonSoftWait, ReasonScopeExecution, ap.Reason))
		ap.SoftBlockers = ReasonMessages(ReasonsBySeverity(ap.Reasons, ReasonSoftWait))
		ap.NextTrigger = "Chờ entry sâu hơn để layer đạt reward/risk."
		return ap
	}
	ap.State = StateActiveLimit
	ap.Reason = "đủ setup gates, discount zone và reward/risk; tạo paper limit layers"
	ap.NextTrigger = "Layer hợp lệ; live manager kiểm tra preflight/caps trước khi đặt post-only."
	return ap
}

func (ap AssetPlan) RewardRiskDetailToResistance() market.Zone {
	if ap.RewardRiskDetail.Target > 0 {
		return market.Zone{Low: ap.RewardRiskDetail.Target, High: ap.RewardRiskDetail.Target, Name: "target"}
	}
	return market.Zone{}
}

func firstReason(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func primarySetupReason(items []string) string {
	return firstReason(items)
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
	// Adaptive layer spacing based on support zone width.
	// Narrow zone (<2%): create artificial spacing to avoid clustering orders.
	// Wide zone (>8%): compress layers to top 60% to avoid over-discounted entries.
	// Normal zone: use High/Mid/Low as-is.
	var prices []float64
	if support.Valid() && support.Mid() > 0 {
		zoneW := (support.High - support.Low) / support.Mid()
		switch {
		case zoneW < 0.02:
			// Zone too narrow — spread layers artificially at ±0.5%, ±1.5%
			mid := support.Mid()
			prices = []float64{mid * 1.005, mid, mid * 0.985}
		case zoneW > 0.08:
			// Zone too wide — compress to upper 60% to avoid chasing deep discounts
			prices = []float64{support.High, support.High * 0.97, support.Mid()}
		default:
			prices = []float64{support.High, support.Mid(), support.Low}
		}
	} else {
		prices = []float64{support.High, support.Mid(), support.Low}
	}
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

func accumulationAssets(cfg config.Config) []string {
	btc := strings.ToUpper(strings.TrimSpace(cfg.Data.Symbols.BTC))
	seen := map[string]bool{}
	out := []string{}
	for _, sym := range cfg.Data.Symbols.Assets {
		normalized := strings.ToUpper(strings.TrimSpace(sym))
		if normalized == "" || normalized == btc || seen[normalized] {
			continue
		}
		seen[normalized] = true
		out = append(out, sym)
	}
	return out
}

func targetAssetCandles(symbols []string, candles map[string][]market.Candle) map[string][]market.Candle {
	out := map[string][]market.Candle{}
	for _, sym := range symbols {
		out[sym] = candles[sym]
	}
	return out
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
	if config.LiveAutoMaxNotionalUSDT(cfg) > 0 {
		return config.LiveAutoMaxNotionalUSDT(cfg)
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
	return 0.10
}

func minScoutRewardRisk(cfg config.Config) float64 {
	if cfg.Risk.MinScoutRewardRisk > 0 {
		return cfg.Risk.MinScoutRewardRisk
	}
	return 1.5
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
	s := fmt.Sprintf("- Trạng thái: %s\n- BTC là market gate/benchmark, không phải target gom.\n- Có đặt lệnh không? %v\n", p.State, p.State == StateActiveLimit)
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
