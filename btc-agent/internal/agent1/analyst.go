package agent1

import (
	"encoding/json"
	"fmt"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/exchange"
	"btc-agent/internal/flow"
	"btc-agent/internal/market"
)

type Risk string

const (
	Low     Risk = "LOW"
	Medium  Risk = "MEDIUM"
	High    Risk = "HIGH"
	Unknown Risk = "UNKNOWN"
)

type Permission string

const (
	NoTrade Permission = "NO_TRADE"
	Watch   Permission = "WATCH"
	Armed   Permission = "ARMED"
	Allowed Permission = "ALLOWED"
)

type MarketAnalysis struct {
	Timestamp          time.Time                     `json:"timestamp"`
	BTCPrice           float64                       `json:"btc_price"`
	WeeklyBias         string                        `json:"weekly_bias"`
	DailyBias          string                        `json:"daily_bias"`
	FourHourBias       string                        `json:"four_hour_bias"`
	MarketRegime       string                        `json:"market_regime"`
	TrendScore         float64                       `json:"trend_score"`
	RiskLevel          Risk                          `json:"risk_level"`
	FallingKnifeRisk   Risk                          `json:"falling_knife_risk"`
	FomoRisk           Risk                          `json:"fomo_risk"`
	PrimarySupportZone market.Zone                   `json:"primary_support_zone"`
	DeepSupportZone    market.Zone                   `json:"deep_support_zone"`
	ResistanceZone     market.Zone                   `json:"resistance_zone"`
	AccumulationZone   market.Zone                   `json:"accumulation_zone"`
	InvalidationZone   market.Zone                   `json:"invalidation_zone"`
	ScenarioMain       string                        `json:"scenario_main"`
	ScenarioBullish    string                        `json:"scenario_bullish"`
	ScenarioBearish    string                        `json:"scenario_bearish"`
	ActionPermission   Permission                    `json:"action_permission"`
	Summary            string                        `json:"summary"`
	FearGreed          exchange.FearGreed            `json:"fear_greed"`
	Frames             map[string]market.FrameSignal `json:"frames"`
	Flow               flow.MultiFrame               `json:"flow"`
}

func Analyze(cfg config.Config, btc map[string][]market.Candle, fg exchange.FearGreed) (MarketAnalysis, error) {
	w, d, h4 := market.Frame(btc["1w"]), market.Frame(btc["1d"]), market.Frame(btc["4h"])
	price := market.LastClose(btc["1d"])
	if price == 0 {
		price = market.LastClose(btc["4h"])
	}
	if price == 0 {
		return MarketAnalysis{}, fmt.Errorf("missing BTC candles")
	}
	ps, rs := market.RangeZone(btc["1d"], 60)
	deep := market.DeepSupport(btc["1w"], 104)
	acc := market.AccumulationZone(ps, cfg.BTCCycle.StressPriceReference)
	trend := (w.TrendScore*0.45 + d.TrendScore*0.40 + h4.TrendScore*0.15)
	fl := flow.AnalyzeMultiFrame(btc)
	regime := classifyRegime(w, d, h4, price, fg)
	falling := fallingKnife(w, d, h4, btc["1d"])
	fomo := fomoRisk(w, d, h4, price, rs, fg)
	if fl.Bias == flow.BiasBullTrap || fl.Bias == flow.BiasDistribution || fl.Daily.FailedBreakout || fl.Daily.Distribution {
		fomo = High
	}
	risk := Medium
	if falling == High || fomo == High || regime == "PANIC_SELLING" {
		risk = High
	} else if trend >= 65 && falling == Low && fomo != High {
		risk = Low
	}
	perm := permission(regime, risk, falling, fomo, ps, rs, trend)
	if perm == Watch && risk != High && falling != High && fomo != High && (fl.Bias == flow.BiasAccumulation || fl.Bias == flow.BiasBearTrap) && fl.Score >= 0.25 {
		perm = Armed
	}
	if fl.Bias == flow.BiasBullTrap || fl.Bias == flow.BiasDistribution {
		if perm == Allowed {
			perm = Watch
		}
	}
	inv := market.Zone{}
	if ps.Valid() {
		inv = market.Zone{Low: ps.Low * 0.985, High: ps.Low, Name: "invalidation"}
	}
	if !inv.Valid() {
		perm = NoTrade
	}
	a := MarketAnalysis{Timestamp: time.Now(), BTCPrice: price, WeeklyBias: w.Bias, DailyBias: d.Bias, FourHourBias: h4.Bias, MarketRegime: regime, TrendScore: trend, RiskLevel: risk, FallingKnifeRisk: falling, FomoRisk: fomo, PrimarySupportZone: ps, DeepSupportZone: deep, ResistanceZone: rs, AccumulationZone: acc, InvalidationZone: inv, ActionPermission: perm, FearGreed: fg, Frames: map[string]market.FrameSignal{"1w": w, "1d": d, "4h": h4}, Flow: fl}
	a.ScenarioMain = "Ưu tiên bảo toàn vốn; chỉ gom khi BTC giữ vùng hỗ trợ/value và có reclaim rõ."
	a.ScenarioBullish = "BTC reclaim EMA/kháng cự gần, 1D giữ cấu trúc, volume bán giảm; Agent 2 có thể chuyển sang ARMED/ALLOWED."
	a.ScenarioBearish = "BTC phá hỗ trợ chính với volume bán tăng hoặc 4H/1D tiếp tục lower-low; giữ NO_TRADE."
	a.Summary = fmt.Sprintf("BTC %.2f, regime %s, trend %.1f, permission %s", price, regime, trend, perm)
	return a, nil
}

func classifyRegime(w, d, h4 market.FrameSignal, price float64, fg exchange.FearGreed) string {
	if d.Structure.BreakDown && d.Structure.LowerLowCount >= 2 && d.RSI14 < 35 {
		return "PANIC_SELLING"
	}
	if w.Bias == "BEARISH" && d.Bias == "BEARISH" {
		return "DOWNTREND"
	}
	if w.Bias == "BULLISH" && d.Bias == "BULLISH" && d.TrendScore >= 75 {
		return "STRONG_UPTREND"
	}
	if d.Bias == "BULLISH" {
		return "WEAK_UPTREND"
	}
	if d.Structure.LiquidityReclaim && d.RSI14 < 55 {
		return "ACCUMULATION"
	}
	if fg.Value >= 75 && price >= d.EMA20 && h4.RSI14 > 70 {
		return "DISTRIBUTION"
	}
	if d.Bias == "RANGE" {
		return "RANGE"
	}
	return "TRANSITION"
}
func fallingKnife(w, d, h4 market.FrameSignal, daily []market.Candle) Risk {
	if d.Structure.BreakDown || h4.Structure.LowerLowCount >= 3 {
		return High
	}
	if len(daily) > 2 {
		last := daily[len(daily)-1]
		prev := daily[len(daily)-2]
		if last.Close < prev.Low && last.Volume > prev.Volume*1.5 {
			return High
		}
	}
	if d.Bias == "BEARISH" || h4.Structure.LowerLowCount >= 2 {
		return Medium
	}
	return Low
}
func fomoRisk(w, d, h4 market.FrameSignal, price float64, r market.Zone, fg exchange.FearGreed) Risk {
	if fg.Value >= 80 {
		return High
	}
	if r.Valid() && price > r.Low*0.98 {
		return High
	}
	if h4.RSI14 > 75 || (d.EMA20 > 0 && price > d.EMA20*1.12) {
		return High
	}
	if h4.RSI14 > 65 {
		return Medium
	}
	return Low
}
func permission(regime string, risk Risk, falling Risk, fomo Risk, support, resistance market.Zone, trend float64) Permission {
	if regime == "PANIC_SELLING" || falling == High || fomo == High || risk == High {
		return NoTrade
	}
	if !support.Valid() || !resistance.Valid() {
		return NoTrade
	}
	rr := (resistance.High - support.High) / (support.High - support.Low)
	if rr < 2 {
		return Watch
	}
	if trend >= 60 && (regime == "ACCUMULATION" || regime == "WEAK_UPTREND" || regime == "RANGE") {
		return Allowed
	}
	if trend >= 45 {
		return Armed
	}
	return Watch
}
func (a MarketAnalysis) JSON() string { b, _ := json.MarshalIndent(a, "", "  "); return string(b) }
