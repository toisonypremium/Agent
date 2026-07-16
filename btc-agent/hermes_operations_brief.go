package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/freeapi"
	"btc-agent/internal/hermesagent"
	"btc-agent/internal/microstructure"
)

type HermesBriefSource struct {
	Name       string `json:"name"`
	Fresh      bool   `json:"fresh"`
	AgeMinutes int    `json:"age_minutes"`
	Detail     string `json:"detail,omitempty"`
}
type HermesBriefMM struct {
	Symbol, Verdict string
	Score, Quality  float64
	Core            int
	AskPressure     bool
	Reasons         []string
}
type HermesBriefGlobal struct {
	MarketCapUSD, VolumeUSD, BTCDominance, EURUSD float64
	FearGreed                                     int
	FearGreedLabel                                string
}
type HermesBriefZone struct {
	Symbol, Kind                 string
	Low, High, Score, Confidence float64
	Evidence, Missing            []string
	Invalidation, Trigger        string
}
type HermesBriefAllocation struct {
	Symbol                                              string
	CeilingPct, CurrentPct, ProbePct, OpenPct, ScalePct float64
	State, Condition                                    string
}
type HermesOperationsBrief struct {
	GeneratedAt     time.Time
	LocalTime       string
	Timezone        string
	Kind            string
	Bot             BotRuntimeSnapshot
	Scenario        ScenarioReport
	Hermes          hermesagent.HermesSnapshot
	HermesReport    hermesagent.HermesReport
	MM              []HermesBriefMM
	ResearchSummary string
	MacroSummary    string
	Sources         []HermesBriefSource
	Missing         []string
	Global          HermesBriefGlobal
	Zones           []HermesBriefZone
	Allocations     []HermesBriefAllocation
	ReservePct      float64
	PortfolioCapPct float64
}

func buildHermesOperationsBrief(cfg config.Config, kind string) HermesOperationsBrief {
	tz := cfg.App.Timezone
	if tz == "" {
		tz = "Asia/Ho_Chi_Minh"
	}
	loc, _ := time.LoadLocation(tz)
	if loc == nil {
		loc = time.UTC
	}
	b := HermesOperationsBrief{GeneratedAt: time.Now().UTC(), Timezone: tz, Kind: kind}
	b.LocalTime = b.GeneratedAt.In(loc).Format("02/01/2006 15:04 MST")
	if x, ok := loadBotRuntimeSnapshotReport(); ok {
		b.Bot = x
		b.Sources = append(b.Sources, sourceStatus("bot_state", x.GeneratedAt, "execution/market state"))
	} else {
		b.Missing = append(b.Missing, "bot_state")
	}
	if x, ok := loadScenarioReportFile(); ok {
		b.Scenario = x
		b.Sources = append(b.Sources, sourceStatus("scenario", x.GeneratedAt, "asset plan/readiness"))
	} else {
		b.Missing = append(b.Missing, "scenario")
	}
	b.Hermes = buildHermesSnapshot(cfg)
	if x, ok := loadHermesReportFile(); ok {
		b.HermesReport = x
		b.Sources = append(b.Sources, sourceStatus("hermes", x.GeneratedAt, "latest autonomous analysis"))
	} else {
		b.Missing = append(b.Missing, "hermes decision")
	}
	var ms microstructure.Report
	if readReport("microstructure_latest.json", &ms) {
		for sym, x := range ms.Summary.MMFootprint {
			b.MM = append(b.MM, HermesBriefMM{sym, x.Verdict, x.FootprintScore, x.DataQuality, x.CoreSignalCount, x.CurrentAskPressure, x.Reasons})
		}
		sort.Slice(b.MM, func(i, j int) bool { return b.MM[i].Symbol < b.MM[j].Symbol })
		b.Sources = append(b.Sources, sourceStatus("microstructure", ms.GeneratedAt, "CVD/taker/orderbook/funding/basis"))
	} else {
		b.Missing = append(b.Missing, "microstructure")
	}
	var research struct {
		GeneratedAt time.Time `json:"generated_at"`
		Summary     string    `json:"summary"`
	}
	if readReport("research_brief_latest.json", &research) {
		b.ResearchSummary = research.Summary
		b.Sources = append(b.Sources, sourceStatus("research", research.GeneratedAt, "news/current context"))
	} else {
		b.Missing = append(b.Missing, "research brief")
	}
	var expert struct {
		GeneratedAt      time.Time `json:"generated_at"`
		Summary          string    `json:"summary"`
		ExecutiveSummary string    `json:"executive_summary"`
	}
	if readReport("expert_report_latest.json", &expert) {
		b.MacroSummary = firstBrief(expert.ExecutiveSummary, expert.Summary)
		b.Sources = append(b.Sources, sourceStatus("expert macro", expert.GeneratedAt, "macro/policy/geopolitics"))
	} else {
		b.Missing = append(b.Missing, "expert macro/political")
	}
	var api freeapi.Report
	if readReport("freeapi_latest.json", &api) {
		b.Global = HermesBriefGlobal{api.GlobalMarketCapUSD, api.GlobalVolumeUSD, api.BTCDominancePct, api.EURUSD, api.FearGreedValue, api.FearGreedLabel}
		b.Sources = append(b.Sources, sourceStatusMax("free APIs", api.GeneratedAt, "global cap/volume, dominance, sentiment, FX", 90))
	} else {
		b.Missing = append(b.Missing, "free API global context")
	}
	b.ReservePct = cfg.Portfolio.ReserveCashRatio * 100
	b.PortfolioCapPct = config.EffectiveHermesPortfolioExposure(cfg) / cfg.Portfolio.TotalCapital * 100
	for _, asset := range b.Hermes.Assets {
		capPct := cfg.Portfolio.Allocation[strings.ToUpper(asset.Symbol)] * 100
		if max := config.EffectiveLiveNotionalPerAsset(cfg) / cfg.Portfolio.TotalCapital * 100; capPct <= 0 || capPct > max {
			capPct = max
		}
		confidence := asset.Readiness
		probe := math.Min(capPct, cfg.HermesOperator.MaxProbeNotionalPct*100*confidence)
		open := math.Min(capPct, probe*2)
		scale := math.Min(capPct, open*1.5)
		if strings.Contains(strings.ToUpper(asset.State), "NO_TRADE") {
			probe, open, scale = 0, 0, 0
		}
		b.Allocations = append(b.Allocations, HermesBriefAllocation{asset.Symbol, capPct, 0, probe, open, scale, asset.State, asset.NextTrigger})
		acc := HermesBriefZone{Symbol: asset.Symbol, Kind: "ACCUMULATION_CANDIDATE", Low: asset.EntryZoneLow, High: asset.EntryZoneHigh, Invalidation: fmt.Sprintf("$%.4f", asset.Invalidation), Trigger: asset.NextTrigger}
		if asset.EntryZoneLow > 0 && asset.EntryZoneHigh > 0 {
			acc.Score += 25
			acc.Evidence = append(acc.Evidence, "discount/support zone")
		} else {
			acc.Missing = append(acc.Missing, "entry zone")
		}
		if asset.MMScore >= 50 {
			acc.Score += 25
			acc.Evidence = append(acc.Evidence, "MM footprint")
		} else {
			acc.Missing = append(acc.Missing, "MM footprint")
		}
		if asset.FlowScore >= 0.25 {
			acc.Score += 20
			acc.Evidence = append(acc.Evidence, "bullish executed flow")
		} else {
			acc.Missing = append(acc.Missing, "flow reclaim/absorption")
		}
		if asset.LiquidityPass {
			acc.Score += 15
			acc.Evidence = append(acc.Evidence, "liquidity pass")
		} else {
			acc.Missing = append(acc.Missing, "liquidity")
		}
		if asset.RR >= cfg.Risk.MinRewardRisk {
			acc.Score += 15
			acc.Evidence = append(acc.Evidence, "RR envelope")
		} else {
			acc.Missing = append(acc.Missing, "RR target")
		}
		acc.Confidence = acc.Score / 100
		b.Zones = append(b.Zones, acc)
		dist := HermesBriefZone{Symbol: asset.Symbol, Kind: "DISTRIBUTION_CANDIDATE", Low: asset.Target, High: asset.Target, Invalidation: fmt.Sprintf("entry invalid $%.4f", asset.Invalidation), Trigger: "ask pressure/CVD divergence hoặc distribution trap tại target/resistance"}
		if asset.Target > 0 {
			dist.Score += 35
			dist.Evidence = append(dist.Evidence, "target/resistance")
		} else {
			dist.Missing = append(dist.Missing, "target")
		}
		if strings.Contains(strings.ToUpper(asset.MMCase), "DISTRIBUTION") {
			dist.Score += 40
			dist.Evidence = append(dist.Evidence, "distribution trap")
		} else {
			dist.Missing = append(dist.Missing, "distribution confirmation")
		}
		for _, m := range b.MM {
			if m.Symbol == asset.Symbol && m.AskPressure {
				dist.Score += 25
				dist.Evidence = append(dist.Evidence, "current ask pressure")
			}
		}
		dist.Confidence = dist.Score / 100
		b.Zones = append(b.Zones, dist)
	}
	return b
}
func readReport(name string, v any) bool {
	d, e := os.ReadFile(filepath.Join("reports", name))
	return e == nil && json.Unmarshal(d, v) == nil
}
func sourceStatusMax(name string, at time.Time, detail string, maxAge int) HermesBriefSource {
	age := 0
	fresh := false
	if !at.IsZero() {
		age = int(time.Since(at).Minutes())
		fresh = age >= 0 && age <= maxAge
	}
	return HermesBriefSource{name, fresh, age, detail}
}
func sourceStatus(name string, at time.Time, detail string) HermesBriefSource {
	age := 0
	fresh := false
	if !at.IsZero() {
		age = int(time.Since(at).Minutes())
		fresh = age >= 0 && age <= 240
	}
	return HermesBriefSource{name, fresh, age, detail}
}
func firstBrief(v ...string) string {
	for _, x := range v {
		if strings.TrimSpace(x) != "" {
			return strings.TrimSpace(x)
		}
	}
	return ""
}
func briefCut(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
func briefZone(name string, lo, hi float64) string {
	if lo <= 0 || hi <= 0 {
		return name + " n/a"
	}
	return fmt.Sprintf("%s $%.0f–%.0f", name, lo, hi)
}
func briefAction(b HermesOperationsBrief) string {
	if b.Bot.OperatorHalt {
		return "OPERATOR HALT — chỉ reconcile/quản trị rủi ro"
	}
	if strings.Contains(strings.ToUpper(b.Bot.DataHealthStatus), "BLOCK") || strings.Contains(strings.ToUpper(b.Bot.ReconcileSafetyStatus), "BLOCK") {
		return "SYSTEM BLOCK — không tăng exposure"
	}
	if b.HermesReport.ActionLine != "" {
		return b.HermesReport.ActionLine
	}
	return "Hermes tự đánh giá HOLD/PROBE/OPEN/SCALE theo confidence và safety"
}

func renderHermesExecutive(b HermesOperationsBrief) string {
	var x strings.Builder
	fmt.Fprintf(&x, "HERMES INTELLIGENCE BRIEF\n%s | %s\n\nI. EXECUTIVE DECISION\n%s\nAutonomous | Plan %s | BTC %s | Doctor %s | Reconcile %s\n\n", b.LocalTime, strings.ToUpper(b.Kind), briefAction(b), b.Bot.PlanState, b.Bot.BTCPermission, b.Bot.DoctorStatus, b.Bot.ReconcileSafetyStatus)
	fmt.Fprintf(&x, "II. GLOBAL & BTC REGIME\nGlobal cap $%.2fT | volume $%.1fB | BTC dom %.2f%%\nFear & Greed %d %s | EUR/USD %.4f\nBTC $%.0f %s trend %.1f/100 | W/D/4H %s/%s/%s\nFlow %s %.2f | phase %s %.0f | risk %s knife %s FOMO %s\n%s | %s | %s\n\n", b.Global.MarketCapUSD/1e12, b.Global.VolumeUSD/1e9, b.Global.BTCDominance, b.Global.FearGreed, b.Global.FearGreedLabel, b.Global.EURUSD, b.Bot.BTC.Price, b.Bot.BTC.Regime, b.Bot.BTC.TrendScore, b.Bot.BTC.WeeklyBias, b.Bot.BTC.DailyBias, b.Bot.BTC.FourHourBias, b.Bot.BTC.FlowBias, b.Bot.BTC.FlowScore, b.Bot.BTC.AccumulationPhase, b.Bot.BTC.AccumulationScore, b.Bot.BTC.RiskLevel, b.Bot.BTC.FallingKnifeRisk, b.Bot.BTC.FomoRisk, briefZone("Support", b.Bot.BTC.SupportZone.Low, b.Bot.BTC.SupportZone.High), briefZone("Resistance", b.Bot.BTC.ResistanceZone.Low, b.Bot.BTC.ResistanceZone.High), briefZone("Invalid", b.Bot.BTC.InvalidationZone.Low, b.Bot.BTC.InvalidationZone.High))
	x.WriteString("III. MM / FLOW / LIQUIDITY\n")
	for _, m := range b.MM {
		fmt.Fprintf(&x, "- %s %s %.0f/100 quality %.0f%% core=%d ask=%v | %s\n", m.Symbol, m.Verdict, m.Score*100, m.Quality*100, m.Core, m.AskPressure, briefCut(strings.Join(m.Reasons, "; "), 150))
	}
	x.WriteString("\nIV. ACCUMULATION / DISTRIBUTION MAP\n")
	for _, z := range b.Zones {
		if z.Score < 20 {
			continue
		}
		fmt.Fprintf(&x, "- %s %s $%.4f–%.4f score %.0f confidence %.0f%%\n  Evidence: %s | Missing: %s\n  Confirm: %s | Invalid: %s\n", z.Symbol, z.Kind, z.Low, z.High, z.Score, z.Confidence*100, firstBrief(strings.Join(z.Evidence, ", "), "none"), firstBrief(strings.Join(z.Missing, ", "), "none"), briefCut(z.Trigger, 130), z.Invalidation)
	}
	fmt.Fprintf(&x, "\nV. CAPITAL ALLOCATION (%% VỐN)\nReserve %.1f%% | portfolio cap %.1f%%\n", b.ReservePct, b.PortfolioCapPct)
	for _, a := range b.Allocations {
		fmt.Fprintf(&x, "- %s ceiling %.1f%% | now %.1f%% | probe/open/scale %.1f/%.1f/%.1f%% | %s\n  Điều kiện: %s\n", a.Symbol, a.CeilingPct, a.CurrentPct, a.ProbePct, a.OpenPct, a.ScalePct, a.State, briefCut(a.Condition, 130))
	}
	x.WriteString("\nVI. ASSET PLAYBOOK\n")
	for _, a := range b.Hermes.Assets {
		fmt.Fprintf(&x, "- %s %s ready %.0f%% | entry $%.4f–%.4f | invalid $%.4f | target $%.4f | RR %.2f\n  MM %s %.0f | flow %s %.2f | Liq %s %.0f | trigger %s\n", a.Symbol, a.State, a.Readiness*100, a.EntryZoneLow, a.EntryZoneHigh, a.Invalidation, a.Target, a.RR, a.MMCase, a.MMScore, a.FlowBias, a.FlowScore, a.LiquidityGrade, a.LiquidityScore, briefCut(a.NextTrigger, 140))
	}
	fmt.Fprintf(&x, "\nVII. SCENARIO MATRIX\nBASE: %s\nBULL unlock: %s\nBEAR invalidation: %s\n\nVIII. MACRO / POLICY / NEWS\n%s\nResearch: %s\n\nIX. BOT & SAFETY\nOrders %d | positions %d | autonomous execution retained. Spot limit post-only; không futures, leverage, short, market order.\n\nX. DATA QUALITY\n", briefCut(b.Scenario.BTC.BaseCase, 220), briefCut(b.Scenario.BTC.BullUnlock, 220), briefCut(b.Scenario.BTC.BearInvalidation, 220), briefCut(firstBrief(b.MacroSummary, "Không có macro evidence fresh; không suy diễn."), 420), briefCut(firstBrief(b.ResearchSummary, "Không có research fresh."), 200), b.Bot.OpenLiveOrders, b.Bot.LivePositions)
	for _, q := range b.Sources {
		state := "fresh"
		if !q.Fresh {
			state = "STALE"
		}
		fmt.Fprintf(&x, "- %s %s age=%dm: %s\n", q.Name, state, q.AgeMinutes, q.Detail)
	}
	if len(b.Missing) > 0 {
		fmt.Fprintf(&x, "Missing: %s. Không dùng nguồn thiếu để tăng confidence.\n", strings.Join(b.Missing, ", "))
	}
	return x.String()
}

func renderHermesPlan(b HermesOperationsBrief) string {
	var x strings.Builder
	x.WriteString("HERMES — KẾ HOẠCH & PHÂN BỔ CHUYÊN NGHIỆP\n")
	fmt.Fprintf(&x, "Reserve %.1f%% | portfolio cap %.1f%%\n", b.ReservePct, b.PortfolioCapPct)
	for _, a := range b.Allocations {
		fmt.Fprintf(&x, "\n%s %s — ceiling %.1f%% vốn\nProbe %.1f%% → Open %.1f%% → Scale %.1f%%\nĐiều kiện: %s\n", a.Symbol, a.State, a.CeilingPct, a.ProbePct, a.OpenPct, a.ScalePct, a.Condition)
	}
	x.WriteString("\nVÙNG GOM / PHÂN PHỐI\n")
	for _, z := range b.Zones {
		fmt.Fprintf(&x, "- %s %s $%.4f–%.4f score %.0f | %s\n", z.Symbol, z.Kind, z.Low, z.High, z.Score, briefCut(z.Trigger, 130))
	}
	fmt.Fprintf(&x, "\nBase: %s\nBull: %s\nBear/invalid: %s\n", b.Scenario.BTC.BaseCase, b.Scenario.BTC.BullUnlock, b.Scenario.BTC.BearInvalidation)
	return x.String()
}

func renderHermesWhy(b HermesOperationsBrief) string {
	return fmt.Sprintf("HERMES — GIẢI THÍCH QUYẾT ĐỊNH\n\nHành động: %s\nGate: %s\nAssets: %s\nExit: %s\n\nBằng chứng thuận: BTC phase=%s trend=%.1f flow=%s %.2f; MM=%s\nBằng chứng nghịch/rủi ro: %s | falling knife=%s | FOMO=%s\nTrigger tiếp theo: %s\nVô hiệu: %s\n\nNO_TRADE/MARKDOWN là input sizing trong autonomous; operator halt/data/reconcile/exchange/caps mới là hard safety.\n", briefAction(b), b.HermesReport.GateSummary, b.HermesReport.AssetSummary, b.HermesReport.ExitSummary, b.Bot.BTC.AccumulationPhase, b.Bot.BTC.TrendScore, b.Bot.BTC.FlowBias, b.Bot.BTC.FlowScore, briefMMLine(b.MM), b.Bot.RiskGovernorSummary, b.Bot.BTC.FallingKnifeRisk, b.Bot.BTC.FomoRisk, b.Scenario.BTC.BullUnlock, b.Scenario.BTC.BearInvalidation)
}
func briefMMLine(mm []HermesBriefMM) string {
	for _, m := range mm {
		if m.Symbol == "BTCUSDT" {
			return fmt.Sprintf("%s %.0f%% core=%d quality=%.0f%%", m.Verdict, m.Score*100, m.Core, m.Quality*100)
		}
	}
	return "missing"
}
func renderHermesRisk(b HermesOperationsBrief) string {
	return fmt.Sprintf("HERMES — RISK & SAFETY\nOperator halt: %v\nDoctor: %s\nData: %s — %s\nReconcile: %s — %s\nRisk governor: %s — %s\nOpen orders=%d positions=%d\nCaps động theo %% vốn; probe/action/portfolio được final assertion kiểm tra.\nHard locks: halt, stale/bad data, mismatch/unknown position, exchange/filter failure, ownership/no-short, caps.\n", b.Bot.OperatorHalt, b.Bot.DoctorStatus, b.Bot.DataHealthStatus, b.Bot.DataHealthSummary, b.Bot.ReconcileSafetyStatus, b.Bot.ReconcileSafetySummary, b.Bot.RiskGovernorStatus, b.Bot.RiskGovernorSummary, b.Bot.OpenLiveOrders, b.Bot.LivePositions)
}
func renderHermesFlow(b HermesOperationsBrief) string {
	var x strings.Builder
	x.WriteString("HERMES — FLOW / MM / LIQUIDITY\n")
	for _, m := range b.MM {
		fmt.Fprintf(&x, "\n%s: %s score %.0f%% quality %.0f%% core=%d ask_pressure=%v\n- %s\n", m.Symbol, m.Verdict, m.Score*100, m.Quality*100, m.Core, m.AskPressure, strings.Join(m.Reasons, "; "))
	}
	x.WriteString("\nFunding/basis chỉ là context; verdict dương cần executed-flow/orderbook core signal. Threshold taker anomaly tự calibrate theo outcome 24h.\n")
	return x.String()
}
func renderHermesSchedule(b HermesOperationsBrief) string {
	return fmt.Sprintf("HERMES — LỊCH VẬN HÀNH (%s)\n07:00: opening — macro/news, BTC regime, flow/MM, asset plan, risk budget\n13:00: mid-day — thay đổi confidence, liquidity, decision/exposure\nMỗi 4 giờ: digest — delta từ bản trước, trigger sắp đạt\n23:00: closing — action/fill/PnL, calibration outcome, kế hoạch ngày sau\nMỗi 15 phút: market/reconcile/supervisor/exit\nMỗi 60 phút: live audit + Hermes autonomous decision\nNgay lập tức: execution, fill, cancel, reduce/exit, halt, reconcile/data/exchange incident\n", b.Timezone)
}
