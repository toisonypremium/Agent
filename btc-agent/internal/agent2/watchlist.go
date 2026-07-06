package agent2

import (
	"fmt"
	"sort"
	"strings"

	"btc-agent/internal/config"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

const (
	WatchTierActionable = "ACTIONABLE_WATCH"
	WatchTierEarly      = "EARLY_WATCH"
	WatchTierBlocked    = "BLOCKED"
	WatchTierDataWait   = "DATA_WAIT"
)

const (
	EntryCheckBTCPermission    = "BTC_PERMISSION"
	EntryCheckFallingKnife     = "FALLING_KNIFE"
	EntryCheckFOMO             = "FOMO"
	EntryCheckRelativeStrength = "RELATIVE_STRENGTH"
	EntryCheckRotationScore    = "ROTATION_SCORE"
	EntryCheckRotationRank     = "ROTATION_RANK"
	EntryCheckAssetFlowEntry   = "ASSET_FLOW_ENTRY"
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
	Symbol          string               `json:"symbol"`
	State           State                `json:"state"`
	ReadinessScore  float64              `json:"readiness_score"`
	Actionable      bool                 `json:"actionable"`
	Tier            string               `json:"tier"`
	NoiseFlags      []string             `json:"noise_flags,omitempty"`
	EntryChecklist  []EntryChecklistItem `json:"entry_checklist,omitempty"`
	RotationRank    int                  `json:"rotation_rank,omitempty"`
	RotationScore   float64              `json:"rotation_score,omitempty"`
	AssetReturn     float64              `json:"asset_return"`
	BenchmarkReturn float64              `json:"benchmark_return"`
	RelativeReturn  float64              `json:"relative_return"`
	FlowBias        flow.Bias            `json:"flow_bias,omitempty"`
	FlowBullScore   float64              `json:"flow_bull_score"`
	FlowBearScore   float64              `json:"flow_bear_score"`
	Price           float64              `json:"price"`
	Support         market.Zone          `json:"support"`
	Resistance      market.Zone          `json:"resistance"`
	DiscountGap     float64              `json:"discount_gap"`
	RewardRisk      float64              `json:"reward_risk"`
	BlockReason     string               `json:"block_reason"`
	Missing         []string             `json:"missing"`
	NextTrigger     string               `json:"next_trigger"`
}

type WatchlistReport struct {
	Candidates []WatchCandidate `json:"candidates"`
	Summary    string           `json:"summary"`
}

func BuildWatchlist(cfg config.Config, assets map[string][]market.Candle, benchmark []market.Candle, rotation []AssetRotationScore, plans []AssetPlan) WatchlistReport {
	rotationBySymbol := map[string]AssetRotationScore{}
	for _, r := range rotation {
		rotationBySymbol[r.Symbol] = r
	}
	planBySymbol := map[string]AssetPlan{}
	for _, p := range plans {
		planBySymbol[p.Symbol] = p
	}

	out := WatchlistReport{}
	for _, sym := range cfg.Data.Symbols.Assets {
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
		c.RewardRisk = plan.RewardRisk
	} else {
		c.BlockReason = "chưa có plan chi tiết"
	}
	if len(candles) < 60 {
		c.Missing = append(c.Missing, "chưa đủ dữ liệu 1D")
		c.Tier = WatchTierDataWait
		c.Actionable = false
		c.NextTrigger = "Chờ đủ dữ liệu nến 1D trước khi đánh giá candidate."
		c.EntryChecklist = buildEntryChecklist(c, cfg)
		return c
	}

	c.Price = market.LastClose(candles)
	c.Support, c.Resistance = market.RangeZone(candles, 60)
	if c.Support.Valid() && c.Price > 0 {
		if c.Price > c.Support.High {
			c.DiscountGap = c.Price/c.Support.High - 1
		} else if c.Price < c.Support.Low {
			c.DiscountGap = c.Price/c.Support.Low - 1
		}
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
		entry := AssetFlowEntry(sym, candles, minBull, allowNeutral)
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

	discountReady := discountComponent(c.Price, c.Support)
	if !c.Support.Valid() {
		c.Missing = append(c.Missing, "support zone không hợp lệ")
	} else if c.Price > c.Support.High*(1+discountZonePremiumPct(cfg)) {
		c.Missing = append(c.Missing, "giá chưa vào discount zone")
	} else if c.Price < c.Support.Low*0.97 {
		c.Missing = append(c.Missing, "giá dưới support quá sâu; tránh dao rơi")
	}

	rrReady := 0.50
	if c.RewardRisk <= 0 && c.Support.Valid() && c.Resistance.Valid() && c.Price > 0 {
		rr := RewardRiskFromZones(c.Price, c.Support, c.Resistance)
		if rr.Valid {
			c.RewardRisk = rr.Ratio
		}
	}
	if cfg.Risk.MinRewardRisk > 0 && c.RewardRisk > 0 {
		rrReady = clamp01(c.RewardRisk / cfg.Risk.MinRewardRisk)
		if c.RewardRisk < cfg.Risk.MinRewardRisk {
			c.Missing = append(c.Missing, fmt.Sprintf("reward/risk dưới %.2f", cfg.Risk.MinRewardRisk))
		}
	} else {
		c.Missing = append(c.Missing, "chưa tính được reward/risk")
	}

	if c.BlockReason != "" && c.State != StateActiveLimit {
		appendReasonMissing(&c)
	}
	c.Missing = uniqueStrings(c.Missing)
	c.ReadinessScore = clamp01(relativeReady*0.25 + rotationReady*0.25 + flowReady*0.20 + discountReady*0.15 + rrReady*0.15)
	c = tuneWatchCandidate(c, cfg)
	c.NextTrigger = nextTrigger(c)
	c.EntryChecklist = buildEntryChecklist(c, cfg)
	return c
}

func appendReasonMissing(c *WatchCandidate) {
	r := c.BlockReason
	switch {
	case strings.Contains(r, "falling knife"):
		c.Missing = append(c.Missing, "falling knife risk")
	case strings.Contains(r, "FOMO"):
		c.Missing = append(c.Missing, "FOMO risk")
	case strings.Contains(r, "relative strength"):
		c.Missing = append(c.Missing, "relative strength yếu hơn BTC")
	case strings.Contains(r, "rotation score"):
		c.Missing = append(c.Missing, "rotation chưa đạt")
	case strings.Contains(r, "asset flow entry"):
		if strings.Contains(r, "chặn") {
			c.Missing = append(c.Missing, "asset flow đang distribution/bull-trap")
		} else {
			c.Missing = append(c.Missing, "asset flow chưa reclaim/absorption")
		}
	case strings.Contains(r, "discount"):
		c.Missing = append(c.Missing, "giá chưa vào discount zone")
	case strings.Contains(r, "reward/risk"):
		c.Missing = append(c.Missing, "reward/risk chưa đủ")
	}
}

func AddWatchlistMissing(w *WatchlistReport, missing string, cfg config.Config) {
	for i := range w.Candidates {
		w.Candidates[i].Missing = uniqueStrings(append([]string{missing}, w.Candidates[i].Missing...))
		w.Candidates[i] = tuneWatchCandidate(w.Candidates[i], cfg)
		w.Candidates[i].NextTrigger = nextTrigger(w.Candidates[i])
		w.Candidates[i].EntryChecklist = buildEntryChecklist(w.Candidates[i], cfg)
	}
	sortWatchCandidates(w.Candidates)
	w.Summary = summarizeWatchlist(w.Candidates)
}

func tuneWatchCandidate(c WatchCandidate, cfg config.Config) WatchCandidate {
	c.Actionable = false
	c.Tier = WatchTierEarly
	joined := strings.ToLower(strings.Join(c.Missing, " ") + " " + c.BlockReason)
	capScore := 1.0
	capReason := ""

	if strings.Contains(joined, "chưa đủ dữ liệu") {
		capScore = 0.0
		capReason = "DATA_WAIT"
		c.Tier = WatchTierDataWait
	} else if strings.Contains(joined, "falling knife") || strings.Contains(joined, "fomo") || strings.Contains(joined, "relative strength") || strings.Contains(joined, "distribution") || strings.Contains(joined, "bull-trap") {
		capScore = 0.35
		capReason = "DANGER_OR_RELATIVE_WEAK"
		c.Tier = WatchTierBlocked
	} else if strings.Contains(joined, "btc permission") || strings.Contains(joined, "btc risk") {
		capScore = 0.49
		capReason = "BTC_NOT_ALLOWED"
		c.Tier = WatchTierEarly
	} else if strings.Contains(joined, "rotation rank") {
		capScore = 0.55
		capReason = "ROTATION_RANK"
		c.Tier = WatchTierEarly
	} else if strings.Contains(joined, "rotation score") || strings.Contains(joined, "rotation chưa đạt") {
		capScore = 0.50
		capReason = "ROTATION_SCORE"
		c.Tier = WatchTierEarly
	} else if strings.Contains(joined, "flow") || strings.Contains(joined, "reclaim") || strings.Contains(joined, "absorption") {
		capScore = 0.65
		capReason = "FLOW_NOT_CONFIRMED"
		c.Tier = WatchTierEarly
	} else if strings.Contains(joined, "discount") {
		capScore = 0.65
		capReason = "DISCOUNT_NOT_READY"
		c.Tier = WatchTierEarly
	} else if strings.Contains(joined, "reward/risk") {
		capScore = 0.70
		capReason = "RR_NOT_READY"
		c.Tier = WatchTierEarly
	}

	if c.ReadinessScore > capScore {
		c.ReadinessScore = capScore
	}
	if capReason != "" {
		c.NoiseFlags = uniqueStrings(append(c.NoiseFlags, capReason))
	}
	if len(c.Missing) == 0 && c.ReadinessScore >= 0.70 {
		c.Tier = WatchTierActionable
		c.Actionable = true
	}
	return c
}

func buildEntryChecklist(c WatchCandidate, cfg config.Config) []EntryChecklistItem {
	_ = cfg
	joined := strings.ToLower(strings.Join(c.Missing, " ") + " " + c.BlockReason)
	items := []EntryChecklistItem{
		entryChecklistItem(EntryCheckBTCPermission, EntryCheckHard, !containsAny(joined, "btc permission", "btc risk"), firstMatchingReason(c, "BTC permission", "BTC risk"), "BTC permission/risk đã cho phép."),
		entryChecklistItem(EntryCheckFallingKnife, EntryCheckHard, !containsAny(joined, "falling knife", "dao rơi"), firstMatchingReason(c, "falling knife", "dao rơi"), "Không có falling-knife risk."),
		entryChecklistItem(EntryCheckFOMO, EntryCheckHard, !containsAny(joined, "fomo"), firstMatchingReason(c, "FOMO"), "Không có FOMO risk."),
	}

	relativePass := !containsAny(joined, "relative strength")
	relativeSeverity := EntryCheckHard
	relativeReason := firstMatchingReason(c, "relative strength")
	if containsAny(joined, "thiếu btc benchmark") {
		relativePass = false
		relativeSeverity = EntryCheckSoft
		relativeReason = firstMatchingReason(c, "thiếu BTC benchmark")
	}
	items = append(items, entryChecklistItem(EntryCheckRelativeStrength, relativeSeverity, relativePass, relativeReason, "Relative strength không yếu hơn BTC."))

	items = append(items,
		entryChecklistItem(EntryCheckRotationScore, EntryCheckSoft, !containsAny(joined, "rotation score", "rotation chưa đạt"), firstMatchingReason(c, "rotation score", "rotation chưa đạt"), "Rotation score đạt ngưỡng."),
		entryChecklistItem(EntryCheckRotationRank, EntryCheckSoft, !containsAny(joined, "rotation rank"), firstMatchingReason(c, "rotation rank"), "Rotation rank nằm trong top được phép."),
	)

	flowFail := containsAny(joined, "flow", "reclaim", "absorption", "distribution", "bull-trap")
	flowSeverity := EntryCheckSoft
	if containsAny(joined, "distribution", "bull-trap", "failed breakout") {
		flowSeverity = EntryCheckHard
	}
	items = append(items, entryChecklistItem(EntryCheckAssetFlowEntry, flowSeverity, !flowFail, firstMatchingReason(c, "asset flow", "flow", "reclaim", "absorption", "distribution", "bull-trap"), "Asset flow entry đã xác nhận."))

	discountFail := containsAny(joined, "discount", "support zone", "dưới support")
	discountSeverity := EntryCheckSoft
	if containsAny(joined, "dưới support", "dao rơi") {
		discountSeverity = EntryCheckHard
	}
	items = append(items,
		entryChecklistItem(EntryCheckDiscountZone, discountSeverity, !discountFail, firstMatchingReason(c, "discount", "support zone", "dưới support"), "Giá nằm trong vùng discount hợp lệ."),
		entryChecklistItem(EntryCheckRewardRisk, EntryCheckSoft, !containsAny(joined, "reward/risk"), firstMatchingReason(c, "reward/risk"), "Reward/risk đạt ngưỡng hoặc đã tính được."),
	)
	return items
}

func entryChecklistItem(name, severity string, pass bool, reason, passReason string) EntryChecklistItem {
	if pass {
		reason = passReason
	} else if reason == "" {
		reason = "check chưa đạt theo deterministic engine."
	}
	return EntryChecklistItem{Name: name, Pass: pass, Severity: severity, Reason: reason}
}

func firstMatchingReason(c WatchCandidate, needles ...string) string {
	for _, m := range c.Missing {
		lower := strings.ToLower(m)
		for _, needle := range needles {
			if strings.Contains(lower, strings.ToLower(needle)) {
				return m
			}
		}
	}
	lowerReason := strings.ToLower(c.BlockReason)
	for _, needle := range needles {
		if strings.Contains(lowerReason, strings.ToLower(needle)) {
			return c.BlockReason
		}
	}
	return ""
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, strings.ToLower(needle)) {
			return true
		}
	}
	return false
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
	for _, m := range c.Missing {
		switch {
		case strings.Contains(m, "BTC permission"):
			return "Chờ BTC chuyển ALLOWED; asset chỉ nằm watchlist, không tạo lệnh."
		case strings.Contains(m, "distribution") || strings.Contains(m, "bull-trap"):
			return "Chờ hết distribution/bull-trap; cần reclaim lại support với bull flow."
		case strings.Contains(m, "flow"):
			return "Chờ sweep low + reclaim support hoặc absorption volume gần support."
		case strings.Contains(m, "discount"):
			return "Chờ giá về support/discount zone mà không tạo falling knife."
		case strings.Contains(m, "reward/risk"):
			return "Chờ entry sâu hơn hoặc resistance mở rộng để RR đạt ngưỡng."
		case strings.Contains(m, "relative"):
			return "Chờ asset ngừng underperform BTC trong lookback."
		case strings.Contains(m, "rotation"):
			return "Chờ rotation score tăng hoặc rank vào top được phép."
		case strings.Contains(m, "falling knife"):
			return "Chờ cấu trúc ngừng lower-low và có reclaim support rõ."
		case strings.Contains(m, "FOMO"):
			return "Không đuổi giá; chờ pullback về value/support."
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
