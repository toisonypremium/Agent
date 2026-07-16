package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/config"
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
	return b
}
func readReport(name string, v any) bool {
	d, e := os.ReadFile(filepath.Join("reports", name))
	return e == nil && json.Unmarshal(d, v) == nil
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
	fmt.Fprintf(&x, "HERMES — TRUNG TÂM VẬN HÀNH\n%s | %s\n\n", b.LocalTime, strings.ToUpper(b.Kind))
	fmt.Fprintf(&x, "1. QUYẾT ĐỊNH HIỆN TẠI\n%s\nMode autonomous | Plan %s | BTC permission %s\nDoctor %s | Reconcile %s | Orders %d | Positions %d\n\n", briefAction(b), b.Bot.PlanState, b.Bot.BTCPermission, b.Bot.DoctorStatus, b.Bot.ReconcileSafetyStatus, b.Bot.OpenLiveOrders, b.Bot.LivePositions)
	fmt.Fprintf(&x, "2. THỊ TRƯỜNG & DÒNG TIỀN\nBTC $%.0f | %s | trend %.1f/100\nW/D/4H: %s/%s/%s | Flow %s %.2f | Accumulation %s %.0f\n%s | %s\n", b.Bot.BTC.Price, b.Bot.BTC.Regime, b.Bot.BTC.TrendScore, b.Bot.BTC.WeeklyBias, b.Bot.BTC.DailyBias, b.Bot.BTC.FourHourBias, b.Bot.BTC.FlowBias, b.Bot.BTC.FlowScore, b.Bot.BTC.AccumulationPhase, b.Bot.BTC.AccumulationScore, briefZone("Support", b.Bot.BTC.SupportZone.Low, b.Bot.BTC.SupportZone.High), briefZone("Invalid", b.Bot.BTC.InvalidationZone.Low, b.Bot.BTC.InvalidationZone.High))
	if len(b.MM) > 0 {
		x.WriteString("MM footprint: ")
		for i, m := range b.MM {
			if i >= 4 {
				break
			}
			if i > 0 {
				x.WriteString(" | ")
			}
			fmt.Fprintf(&x, "%s %s %.0f%% q%.0f%%", m.Symbol, m.Verdict, m.Score*100, m.Quality*100)
		}
		x.WriteString("\n")
	}
	fmt.Fprintf(&x, "\n3. VĨ MÔ / CHÍNH SÁCH / TÂM LÝ\n%s\nResearch: %s\n", briefCut(firstBrief(b.MacroSummary, "Chưa có expert macro/political fresh; Hermes không tự suy diễn."), 500), briefCut(firstBrief(b.ResearchSummary, "Chưa có research brief fresh."), 350))
	x.WriteString("\n4. KẾ HOẠCH TÀI SẢN\n")
	for i, c := range b.Scenario.Coins {
		if i >= 4 {
			break
		}
		fmt.Fprintf(&x, "- %s %s %.0f%% | MM %s %.0f | Liq %s %.0f | RR %.2f\n  Trigger: %s\n", c.Symbol, c.State, c.ReadinessScore*100, c.MMCase, c.MMScore, c.LiquidityGrade, c.LiquidityScore, c.RewardRisk, briefCut(c.NextTrigger, 180))
	}
	fmt.Fprintf(&x, "\n5. TRIGGER / VÔ HIỆU\nBase: %s\nMở khóa: %s\nVô hiệu: %s\n", briefCut(b.Scenario.BTC.BaseCase, 250), briefCut(b.Scenario.BTC.BullUnlock, 250), briefCut(b.Scenario.BTC.BearInvalidation, 250))
	fmt.Fprintf(&x, "\n6. LỊCH HERMES (%s)\n07:00 opening brief | 13:00 mid-day review | mỗi 4h digest | 23:00 closing review\nMỗi 15m: reconcile + supervisor/exit | mỗi 60m: audit + Hermes decision | safety/execution: báo ngay\n", b.Timezone)
	fmt.Fprintf(&x, "\n7. DATA QUALITY\n")
	for _, s := range b.Sources {
		state := "fresh"
		if !s.Fresh {
			state = "stale"
		}
		fmt.Fprintf(&x, "- %s: %s age=%dm (%s)\n", s.Name, state, s.AgeMinutes, s.Detail)
	}
	if len(b.Missing) > 0 {
		fmt.Fprintf(&x, "Thiếu: %s. Hermes không suy diễn nguồn thiếu.\n", strings.Join(b.Missing, ", "))
	}
	x.WriteString("\nSafety: spot-only; limit orders; không futures, leverage, short, withdrawal. Market uncertainty điều chỉnh sizing; system/account faults mới hard-block.\n")
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
