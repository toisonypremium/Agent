package agent2

import (
	"fmt"
	"sort"
	"strings"

	"btc-agent/internal/config"
	"btc-agent/internal/flow"
	"btc-agent/internal/liquidity"
	"btc-agent/internal/market"
)

const (
	WatchTierActionable = "ACTIONABLE_WATCH"
	WatchTierEarly      = "EARLY_WATCH"
	WatchTierBlocked    = "BLOCKED"
	WatchTierDataWait   = "DATA_WAIT"
)

const (
	EntryCheckData             = "DATA"
	EntryCheckBTCPermission    = "BTC_PERMISSION"
	EntryCheckFallingKnife     = "FALLING_KNIFE"
	EntryCheckFOMO             = "FOMO"
	EntryCheckRelativeStrength = "RELATIVE_STRENGTH"
	EntryCheckRotationScore    = "ROTATION_SCORE"
	EntryCheckRotationRank     = "ROTATION_RANK"
	EntryCheckMMAccumulation   = "MM_ACCUMULATION"
	EntryCheckAssetFlowEntry   = "ASSET_FLOW_ENTRY"
	EntryCheckLiquidityQuality = "LIQUIDITY_QUALITY"
	EntryCheckDiscountZone     = "DISCOUNT_ZONE"
	EntryCheckRewardRisk       = "REWARD_RISK"
)

const (
	EntryCheckHard = "HARD"
	EntryCheckSoft = "SOFT"
)

type EntryChecklistItem struct {
	Name     string `json:"name"`
	Pass     bool   `json:"pass"`
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
}

type WatchCandidate struct {
	Symbol           string               `json:"symbol"`
	State            State                `json:"state"`
	ReadinessScore   float64              `json:"readiness_score"`
	Actionable       bool                 `json:"actionable"`
	Tier             string               `json:"tier"`
	NoiseFlags       []string             `json:"noise_flags,omitempty"`
	EntryChecklist   []EntryChecklistItem `json:"entry_checklist,omitempty"`
	Reasons          []DecisionReason     `json:"reasons,omitempty"`
	RotationRank     int                  `json:"rotation_rank,omitempty"`
	RotationScore    float64              `json:"rotation_score,omitempty"`
	AssetReturn      float64              `json:"asset_return"`
	BenchmarkReturn  float64              `json:"benchmark_return"`
	RelativeReturn   float64              `json:"relative_return"`
	FlowBias         flow.Bias            `json:"flow_bias,omitempty"`
	FlowBullScore    float64              `json:"flow_bull_score"`
	FlowBearScore    float64              `json:"flow_bear_score"`
	MMCase           MMCase               `json:"mm_case,omitempty"`
	MMScore          float64              `json:"mm_score,omitempty"`
	MMReasons        []string             `json:"mm_reasons,omitempty"`
	MMMissing        []string             `json:"mm_missing,omitempty"`
	LiquidityQuality liquidity.Quality    `json:"liquidity_quality,omitempty"`
	Price            float64              `json:"price"`
	Support          market.Zone          `json:"support"`
	Resistance       market.Zone          `json:"resistance"`
	DiscountGap      float64              `json:"discount_gap"`
	ZoneWidthPct     float64              `json:"zone_width_pct,omitempty"`
	ZoneQuality      string               `json:"zone_quality,omitempty"`
	RewardRisk       float64              `json:"reward_risk"`
	RewardRiskDetail RewardRiskResult     `json:"reward_risk_detail,omitempty"`
	BlockReason      string               `json:"block_reason"`
	Missing          []string             `json:"missing"`
	NextTrigger      string               `json:"next_trigger"`
}

type WatchlistReport struct {
	Candidates []WatchCandidate `json:"candidates"`
	Summary    string           `json:"summary"`
}

func BuildWatchlist(cfg config.Config, assets map[string][]market.Candle, benchmark []market.Candle, rotation []AssetRotationScore, plans []AssetPlan) WatchlistReport {
	assetSymbols := accumulationAssets(cfg)
	rotationBySymbol := map[string]AssetRotationScore{}
	for _, r := range rotation {
		rotationBySymbol[r.Symbol] = r
	}
	planBySymbol := map[string]AssetPlan{}
	for _, p := range plans {
		planBySymbol[p.Symbol] = p
	}

	out := WatchlistReport{}
	for _, sym := range assetSymbols {
		out.Candidates = append(out.Candidates, buildWatchCandidate(cfg, sym, assets[sym], benchmark, rotationBySymbol[sym], planBySymbol[sym]))
	}
	sortWatchCandidates(out.Candidates)
	out.Summary = summarizeWatchlist(out.Candidates)
	return out
}

func buildWatchCandidate(cfg config.Config, sym string, candles []market.Candle, benchmark []market.Candle, rotation AssetRotationScore, plan AssetPlan) WatchCandidate {
	c := WatchCandidate{Symbol: sym, State: StateWatch, FlowBias: flow.BiasNeutral, Tier: WatchTierEarly}
	if plan.Symbol != "" {
		c.State = plan.State
		c.BlockReason = plan.Reason
		c.RotationRank = plan.RotationRank
		c.RotationScore = plan.RotationScore
		c.FlowBias = plan.AssetFlowBias
		c.FlowBullScore = plan.AssetFlowScore
		c.MMCase = plan.MMCase
		c.MMScore = plan.MMScore
		c.MMReasons = plan.MMReasons
		c.MMMissing = plan.MMMissing
		c.LiquidityQuality = plan.LiquidityQuality
		c.RewardRisk = plan.RewardRisk
		c.RewardRiskDetail = plan.RewardRiskDetail
		c.Reasons = append(c.Reasons, plan.Reasons...)
		if plan.SetupScore > 0 {
			c.ReadinessScore = plan.SetupScore
		}
		c.ZoneWidthPct = plan.ZoneWidthPct
		c.ZoneQuality = plan.ZoneQuality
	} else {
		c.BlockReason = "chưa có plan chi tiết"
	}
	if len(candles) < 60 {
		c.Missing = append(c.Missing, "chưa đủ dữ liệu 1D")
		c.Tier = WatchTierDataWait
		c.Actionable = false
		c.NextTrigger = "Chờ đủ dữ liệu nến 1D trước khi đánh giá candidate."
		c.Reasons = AddReason(c.Reasons, NewDecisionReason(ReasonDataWait, ReasonHardBlock, ReasonScopeData, "chưa đủ dữ liệu 1D"))
		c.EntryChecklist = checklistFromReasons(c.Reasons)
		return c
	}

	c.Price = market.LastClose(candles)
	c.Support, c.Resistance = actionSupportResistanceZones(candles)
	mm := AnalyzeMMAccumulation(sym, candles)
	if c.MMCase == "" {
		c.MMCase = mm.Case
		c.MMScore = mm.Score
		c.MMReasons = mm.Reasons
		c.MMMissing = mm.Missing
	}
	if !c.LiquidityQuality.Enabled {
		c.LiquidityQuality = liquidity.EvaluateCandleProxy(cfg, sym, candles, desiredLiquidityNotional(cfg, sym))
	}
	c.DiscountGap = discountGapPct(c.Price, c.Support)
	c.ZoneWidthPct = zoneWidthPct(c.Support)
	if c.ZoneQuality == "" {
		c.ZoneQuality = zoneQuality(c.Support)
	}

	lookback, minRelative, minMomentum := rotationStrengthParams(cfg)
	relativeReady := 0.50
	if len(benchmark) > lookback {
		rs := RelativeStrength(candles, benchmark, lookback, minRelative, minMomentum)
		c.AssetReturn = rs.AssetReturn
		c.BenchmarkReturn = rs.BenchmarkReturn
		c.RelativeReturn = rs.RelativeReturn
		relativeReady = relativeComponent(rs.RelativeReturn)
		if !rs.Pass {
			c.Missing = append(c.Missing, "relative strength yếu hơn BTC")
		}
	} else {
		c.Missing = append(c.Missing, "thiếu BTC benchmark cho relative strength")
	}

	rotationReady := 0.50
	if rotation.Symbol != "" {
		c.RotationRank = rotation.Rank
		c.RotationScore = rotation.Score
		rotationReady = rotation.Score
		if enabled, minScore, maxRank := rotationParams(cfg); enabled {
			if rotation.Score < minScore {
				c.Missing = append(c.Missing, fmt.Sprintf("rotation score dưới %.2f", minScore))
			}
			if maxRank > 0 && rotation.Rank > maxRank {
				c.Missing = append(c.Missing, fmt.Sprintf("rotation rank ngoài top %d", maxRank))
				rotationReady *= 0.60
			}
		}
	} else {
		c.Missing = append(c.Missing, "chưa có rotation ranking")
	}

	flowReady := 0.50
	if enabled, minBull, allowNeutral := assetFlowEntryParams(cfg); enabled {
		entry := AssetFlowEntryFromMM(mm, minBull, allowNeutral)
		c.FlowBias = entry.Bias
		c.FlowBullScore = entry.BullScore
		c.FlowBearScore = entry.BearScore
		if entry.Pass {
			flowReady = 1.00
		} else if entry.HardBlock {
			flowReady = 0
			c.Missing = append(c.Missing, "asset flow đang distribution/bull-trap")
		} else {
			flowReady = 0.50
			c.Missing = append(c.Missing, "asset flow chưa reclaim/absorption")
		}
	}
	mmReady := clamp01(c.MMScore / 100)
	if c.MMCase == MMCaseFallingKnife || c.MMCase == MMCaseDistributionTrap {
		mmReady = 0
		c.Missing = append(c.Missing, fmt.Sprintf("MM case %s hard block", c.MMCase))
	} else if c.MMCase != MMCaseSpringReclaim && c.MMCase != MMCaseArmedProbeCandidate {
		c.Missing = append(c.Missing, fmt.Sprintf("MM case %s chưa đủ footprint", c.MMCase))
	}
	liquidityReady := clamp01(c.LiquidityQuality.Score / 100)
	if c.LiquidityQuality.Enabled && !c.LiquidityQuality.Pass {
		c.Missing = append(c.Missing, "liquidity gate chưa đạt")
	}

	discountReady := discountComponent(c.Price, c.Support)
	if c.ZoneQuality != "ZONE_OK" {
		c.Missing = append(c.Missing, zoneQualityReason(c.ZoneQuality, c.ZoneWidthPct))
	} else if c.Price > c.Support.High*(1+discountZonePremiumPct(cfg)) {
		c.Missing = append(c.Missing, fmt.Sprintf("giá chưa vào discount zone: cao hơn support %.2f%%", c.DiscountGap*100))
	} else if c.Price < c.Support.Low*0.97 {
		c.Missing = append(c.Missing, "giá dưới support quá sâu; tránh dao rơi")
	}

	rrReady := 0.50
	if c.RewardRisk <= 0 && c.Support.Valid() && c.Resistance.Valid() && c.Price > 0 {
		rr := RewardRiskFromZones(c.Price, c.Support, c.Resistance)
		c.RewardRiskDetail = rr
		if rr.Valid {
			c.RewardRisk = rr.Ratio
		}
	}
	if cfg.Risk.MinRewardRisk > 0 && c.RewardRisk > 0 {
		rrReady = clamp01(c.RewardRisk / cfg.Risk.MinRewardRisk)
		if c.RewardRisk < cfg.Risk.MinRewardRisk {
			c.Missing = append(c.Missing, fmt.Sprintf("reward/risk %.2f dưới %.2f", c.RewardRisk, cfg.Risk.MinRewardRisk))
		}
	} else {
		c.Missing = append(c.Missing, "chưa tính được reward/risk")
	}

	c.Missing = uniqueStrings(c.Missing)
	computedReadiness := clamp01(relativeReady*0.20 + rotationReady*0.20 + flowReady*0.15 + mmReady*0.15 + liquidityReady*0.10 + discountReady*0.10 + rrReady*0.10)
	if c.ReadinessScore > 0 {
		c.ReadinessScore = clamp01(c.ReadinessScore*0.60 + computedReadiness*0.40)
	} else {
		c.ReadinessScore = computedReadiness
	}
	if len(c.Reasons) > 0 {
		c.EntryChecklist = checklistFromReasons(c.Reasons)
	}
	if len(plan.SetupGates) > 0 {
		c.EntryChecklist = mergeChecklistItems(append(c.EntryChecklist, checklistFromSetupGates(plan.SetupGates)...))
	}
	c = tuneWatchCandidate(c, cfg)
	c.NextTrigger = nextTrigger(c)
	if len(c.EntryChecklist) == 0 {
		c.EntryChecklist = checklistFromCandidateFacts(c)
	}
	return c
}

func checklistFromReasons(reasons []DecisionReason) []EntryChecklistItem {
	items := []EntryChecklistItem{}
	for _, reason := range reasons {
		if reason.Severity == ReasonInfo {
			continue
		}
		items = append(items, EntryChecklistItem{Name: checklistNameFromReason(reason.Code), Pass: false, Severity: checklistSeverityFromReason(reason.Severity), Reason: reason.Message})
	}
	return mergeChecklistItems(items)
}

func checklistNameFromReason(code ReasonCode) string {
	switch code {
	case ReasonBTCPermission, ReasonBTCPanic, ReasonBTCDowntrend:
		return EntryCheckBTCPermission
	case ReasonFallingKnife:
		return EntryCheckFallingKnife
	case ReasonFOMO:
		return EntryCheckFOMO
	case ReasonRelativeStrength:
		return EntryCheckRelativeStrength
	case ReasonRotationScore:
		return EntryCheckRotationScore
	case ReasonRotationRank:
		return EntryCheckRotationRank
	case ReasonMMAccumulation:
		return EntryCheckMMAccumulation
	case ReasonAssetFlowEntry:
		return EntryCheckAssetFlowEntry
	case ReasonLiquidityQuality:
		return EntryCheckLiquidityQuality
	case ReasonDiscountZone:
		return EntryCheckDiscountZone
	case ReasonRewardRisk, ReasonExecutionLayer:
		return EntryCheckRewardRisk
	case ReasonDataWait:
		return EntryCheckData
	default:
		return string(code)
	}
}

func checklistSeverityFromReason(severity ReasonSeverity) string {
	if severity == ReasonHardBlock {
		return EntryCheckHard
	}
	return EntryCheckSoft
}

func checklistFromSetupGates(gates []SetupGateResult) []EntryChecklistItem {
	items := []EntryChecklistItem{{Name: EntryCheckBTCPermission, Pass: true, Severity: EntryCheckHard, Reason: "BTC permission/risk đã cho phép."}}
	for _, gate := range gates {
		name := gate.Name
		if name == "ZONE_QUALITY" {
			name = EntryCheckDiscountZone
		}
		severity := EntryCheckSoft
		if gate.Severity == SetupGateHard {
			severity = EntryCheckHard
		}
		reason := gate.Reason
		if gate.Pass {
			reason = setupGatePassReason(name)
		}
		items = append(items, EntryChecklistItem{Name: name, Pass: gate.Pass, Severity: severity, Reason: reason})
	}
	return mergeChecklistItems(items)
}

func setupGatePassReason(name string) string {
	switch name {
	case EntryCheckFallingKnife:
		return "Không có falling-knife risk."
	case EntryCheckFOMO:
		return "Không có FOMO risk."
	case EntryCheckRelativeStrength:
		return "Relative strength không yếu hơn BTC."
	case EntryCheckRotationScore:
		return "Rotation score đạt ngưỡng."
	case EntryCheckRotationRank:
		return "Rotation rank nằm trong top được phép."
	case EntryCheckMMAccumulation:
		return "MM accumulation footprint đã xác nhận."
	case EntryCheckAssetFlowEntry:
		return "Asset flow entry đã xác nhận."
	case EntryCheckLiquidityQuality:
		return "Liquidity đủ cho sizing hiện tại."
	case EntryCheckDiscountZone:
		return "Giá nằm trong vùng discount hợp lệ."
	case EntryCheckRewardRisk:
		return "Reward/risk đạt ngưỡng hoặc đã tính được."
	default:
		return "Setup gate pass."
	}
}

func mergeChecklistItems(items []EntryChecklistItem) []EntryChecklistItem {
	order := []string{EntryCheckBTCPermission, EntryCheckFallingKnife, EntryCheckFOMO, EntryCheckRelativeStrength, EntryCheckRotationScore, EntryCheckRotationRank, EntryCheckMMAccumulation, EntryCheckAssetFlowEntry, EntryCheckLiquidityQuality, EntryCheckDiscountZone, EntryCheckRewardRisk}
	byName := map[string]EntryChecklistItem{}
	for _, item := range items {
		if item.Name == "" {
			continue
		}
		prev, ok := byName[item.Name]
		if !ok || (!item.Pass && prev.Pass) || (!item.Pass && !prev.Pass && item.Severity == EntryCheckHard && prev.Severity != EntryCheckHard) {
			byName[item.Name] = item
		}
	}
	out := []EntryChecklistItem{}
	for _, name := range order {
		if item, ok := byName[name]; ok {
			out = append(out, item)
		}
	}
	return out
}

func AddWatchlistMissing(w *WatchlistReport, missing string, cfg config.Config) {
	for i := range w.Candidates {
		w.Candidates[i].Missing = uniqueStrings(append([]string{missing}, w.Candidates[i].Missing...))
		w.Candidates[i] = tuneWatchCandidate(w.Candidates[i], cfg)
		w.Candidates[i].NextTrigger = nextTrigger(w.Candidates[i])
		w.Candidates[i].EntryChecklist = checklistFromCandidateFacts(w.Candidates[i])
	}
	sortWatchCandidates(w.Candidates)
	w.Summary = summarizeWatchlist(w.Candidates)
}

func tuneWatchCandidate(c WatchCandidate, cfg config.Config) WatchCandidate {
	if len(c.EntryChecklist) == 0 {
		c.EntryChecklist = checklistFromCandidateFacts(c)
	}
	return tuneWatchCandidateFromChecklist(c, cfg)
}

func tuneWatchCandidateFromChecklist(c WatchCandidate, cfg config.Config) WatchCandidate {
	c.Actionable = false
	c.Tier = WatchTierEarly
	capScore := 1.0
	capReasons := []string{}
	hardFails := map[string]bool{}
	softFails := map[string]bool{}
	for _, item := range c.EntryChecklist {
		if item.Pass {
			continue
		}
		if item.Severity == EntryCheckHard {
			hardFails[item.Name] = true
		} else {
			softFails[item.Name] = true
		}
	}
	if hardFails[EntryCheckData] || softFails[EntryCheckData] {
		capScore = 0
		capReasons = append(capReasons, "DATA_WAIT")
		c.Tier = WatchTierDataWait
	} else if hardFails[EntryCheckBTCPermission] {
		capScore = 0.49
		capReasons = append(capReasons, "BTC_NOT_ALLOWED")
		c.Tier = WatchTierEarly
	} else if len(hardFails) > 0 {
		capScore = 0.35
		capReasons = append(capReasons, "HARD_CHECK_FAILED")
		c.Tier = WatchTierBlocked
	}
	applySoftCap := func(name, reason string, cap float64) {
		if !softFails[name] {
			return
		}
		if cap < capScore {
			capScore = cap
		}
		capReasons = append(capReasons, reason)
		if c.Tier == "" || c.Tier == WatchTierActionable {
			c.Tier = WatchTierEarly
		}
	}
	applySoftCap(EntryCheckRotationRank, "ROTATION_RANK", 0.55)
	applySoftCap(EntryCheckRotationScore, "ROTATION_SCORE", 0.50)
	applySoftCap(EntryCheckLiquidityQuality, "LIQUIDITY_NOT_READY", 0.60)
	applySoftCap(EntryCheckMMAccumulation, "MM_NOT_CONFIRMED", 0.65)
	applySoftCap(EntryCheckAssetFlowEntry, "FLOW_NOT_CONFIRMED", 0.65)
	applySoftCap(EntryCheckDiscountZone, "DISCOUNT_NOT_READY", 0.65)
	applySoftCap(EntryCheckRewardRisk, "RR_NOT_READY", 0.70)
	if c.ReadinessScore > capScore {
		c.ReadinessScore = capScore
	}
	if len(capReasons) > 0 {
		c.NoiseFlags = uniqueStrings(append(c.NoiseFlags, capReasons...))
	}
	minActionable := cfg.Risk.MinWatchReadinessForProbe
	if minActionable <= 0 {
		minActionable = 0.70
	}
	if len(hardFails) == 0 && len(softFails) == 0 && len(c.Missing) == 0 && c.ReadinessScore >= minActionable {
		c.Tier = WatchTierActionable
		c.Actionable = true
	}
	return c
}

func checklistFromCandidateFacts(c WatchCandidate) []EntryChecklistItem {
	items := []EntryChecklistItem{
		entryChecklistItem(EntryCheckBTCPermission, EntryCheckHard, true, "", "BTC permission/risk đã cho phép."),
		entryChecklistItem(EntryCheckFallingKnife, EntryCheckHard, true, "", "Không có falling-knife risk."),
		entryChecklistItem(EntryCheckFOMO, EntryCheckHard, true, "", "Không có FOMO risk."),
		entryChecklistItem(EntryCheckRelativeStrength, EntryCheckSoft, c.RelativeReturn >= 0, "relative strength yếu hơn BTC", "Relative strength không yếu hơn BTC."),
		entryChecklistItem(EntryCheckRotationScore, EntryCheckSoft, c.RotationScore > 0, "chưa có rotation score", "Rotation score đạt ngưỡng."),
		entryChecklistItem(EntryCheckRotationRank, EntryCheckSoft, c.RotationRank > 0, "chưa có rotation rank", "Rotation rank nằm trong top được phép."),
		entryChecklistItem(EntryCheckMMAccumulation, EntryCheckSoft, c.MMScore > 0, "MM accumulation chưa xác nhận", "MM accumulation footprint đã xác nhận."),
		entryChecklistItem(EntryCheckAssetFlowEntry, EntryCheckSoft, c.FlowBullScore > 0, "asset flow chưa xác nhận", "Asset flow entry đã xác nhận."),
		entryChecklistItem(EntryCheckLiquidityQuality, EntryCheckSoft, !c.LiquidityQuality.Enabled || c.LiquidityQuality.Pass, "liquidity gate chưa đạt", "Liquidity đủ cho sizing hiện tại."),
		entryChecklistItem(EntryCheckDiscountZone, EntryCheckSoft, c.Support.Valid() && c.Price <= c.Support.High*(1+c.DiscountGap), "giá chưa vào discount zone", "Giá nằm trong vùng discount hợp lệ."),
		entryChecklistItem(EntryCheckRewardRisk, EntryCheckSoft, c.RewardRisk > 0, "reward/risk chưa tính được", "Reward/risk đạt ngưỡng hoặc đã tính được."),
	}
	return mergeChecklistItems(items)
}

func entryChecklistItem(name, severity string, pass bool, reason, passReason string) EntryChecklistItem {
	if pass {
		reason = passReason
	} else if reason == "" {
		reason = "check chưa đạt theo deterministic engine."
	}
	return EntryChecklistItem{Name: name, Pass: pass, Severity: severity, Reason: reason}
}

func ChecklistSummary(items []EntryChecklistItem) string {
	if len(items) == 0 {
		return "checklist unavailable"
	}
	hardFails := []string{}
	softFails := []string{}
	for _, item := range items {
		if item.Pass {
			continue
		}
		if item.Severity == EntryCheckHard {
			hardFails = append(hardFails, item.Name)
		} else {
			softFails = append(softFails, item.Name)
		}
	}
	if len(hardFails) == 0 && len(softFails) == 0 {
		return "all checks pass"
	}
	parts := []string{}
	if len(hardFails) > 0 {
		parts = append(parts, "HARD fail: "+strings.Join(hardFails, ", "))
	}
	if len(softFails) > 0 {
		parts = append(parts, "SOFT wait: "+strings.Join(softFails, ", "))
	}
	return strings.Join(parts, "; ")
}

func nextTrigger(c WatchCandidate) string {
	for _, item := range c.EntryChecklist {
		if item.Pass {
			continue
		}
		switch item.Name {
		case EntryCheckBTCPermission:
			return "Chờ BTC chuyển ALLOWED; asset chỉ nằm watchlist, không tạo lệnh."
		case EntryCheckFallingKnife:
			return "Chờ cấu trúc ngừng lower-low và có reclaim support rõ."
		case EntryCheckFOMO:
			return "Không đuổi giá; chờ pullback về value/support."
		case EntryCheckRelativeStrength:
			return "Chờ asset ngừng underperform BTC trong lookback."
		case EntryCheckRotationScore, EntryCheckRotationRank:
			return "Chờ rotation score tăng hoặc rank vào top được phép."
		case EntryCheckMMAccumulation:
			return "Chờ sweep low + close reclaim support + retest giữ vùng."
		case EntryCheckAssetFlowEntry:
			return "Chờ sweep low + reclaim support hoặc absorption volume gần support."
		case EntryCheckLiquidityQuality:
			return "Chờ spread/depth/volume đủ dày trước khi tạo live layer."
		case EntryCheckDiscountZone:
			return "Chờ giá về support/discount zone mà không tạo falling knife."
		case EntryCheckRewardRisk:
			return "Chờ entry sâu hơn hoặc resistance mở rộng để RR đạt ngưỡng."
		case EntryCheckData:
			return "Chờ đủ dữ liệu nến 1D trước khi đánh giá candidate."
		}
	}
	if c.State == StateArmed {
		return "ARMED probe: BTC chưa full ALLOWED; nếu hard safety pass, live manager chỉ được sizing nhỏ."
	}
	if c.State == StateActiveLimit {
		return "Đã đủ điều kiện theo deterministic engine; chỉ paper limit plan."
	}
	return "Theo dõi thêm; chưa có trigger rõ để tạo layer."
}

func summarizeWatchlist(candidates []WatchCandidate) string {
	if len(candidates) == 0 {
		return "Watchlist trống hoặc thiếu dữ liệu asset."
	}
	actionable := 0
	for _, c := range candidates {
		if c.Actionable {
			actionable++
		}
	}
	best := candidates[0]
	if actionable == 0 {
		return fmt.Sprintf("No actionable watch candidates; closest=%s readiness=%.2f tier=%s next=%s", best.Symbol, best.ReadinessScore, best.Tier, best.NextTrigger)
	}
	return fmt.Sprintf("Watchlist candidates=%d actionable=%d closest=%s readiness=%.2f tier=%s next=%s", len(candidates), actionable, best.Symbol, best.ReadinessScore, best.Tier, best.NextTrigger)
}

func sortWatchCandidates(candidates []WatchCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Actionable != candidates[j].Actionable {
			return candidates[i].Actionable
		}
		if candidates[i].ReadinessScore != candidates[j].ReadinessScore {
			return candidates[i].ReadinessScore > candidates[j].ReadinessScore
		}
		ir, jr := candidates[i].RotationRank, candidates[j].RotationRank
		if ir == 0 {
			ir = 999
		}
		if jr == 0 {
			jr = 999
		}
		if ir != jr {
			return ir < jr
		}
		return candidates[i].Symbol < candidates[j].Symbol
	})
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
