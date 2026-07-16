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
func viTerm(v string) string {
	u := strings.ToUpper(strings.TrimSpace(v))
	labels := map[string]string{
		"NO_TRADE": "chưa nên mở vị thế", "SCOUT": "đang thăm dò cơ hội", "WATCH": "tiếp tục quan sát", "HOLD": "giữ nguyên, chưa hành động",
		"ACTIVE_LIMIT": "đủ điều kiện đặt lệnh giới hạn", "RANGE": "đi ngang", "MARKDOWN": "đang trong nhịp giảm",
		"ACCUMULATION_CONFIRMED": "đã xác nhận tích lũy", "POSSIBLE_ACCUMULATION": "có dấu hiệu tích lũy", "DISTRIBUTION": "có dấu hiệu phân phối",
		"NO_EDGE": "chưa có lợi thế rõ", "DOCTOR_OK": "hệ thống hoạt động tốt", "RECONCILE_CLEAN": "sổ lệnh khớp sạch",
		"LOW": "thấp", "MEDIUM": "trung bình", "HIGH": "cao", "BULLISH": "tích cực", "BEARISH": "tiêu cực", "NEUTRAL": "trung tính",
		"UP": "tăng", "DOWN": "giảm", "FLAT": "đi ngang", "EXTREME FEAR": "sợ hãi cực độ", "FEAR": "sợ hãi", "GREED": "tham lam", "EXTREME GREED": "tham lam cực độ",
	}
	if x, ok := labels[u]; ok {
		return x
	}
	if v == "" {
		return "chưa có dữ liệu"
	}
	return strings.ToLower(strings.ReplaceAll(v, "_", " "))
}

func yesNo(v bool) string {
	if v {
		return "có"
	}
	return "không"
}

func briefAction(b HermesOperationsBrief) string {
	if b.Bot.OperatorHalt {
		return "Bot đang dừng khẩn cấp; chỉ kiểm tra tài khoản và bảo vệ vốn."
	}
	if strings.Contains(strings.ToUpper(b.Bot.DataHealthStatus), "BLOCK") || strings.Contains(strings.ToUpper(b.Bot.ReconcileSafetyStatus), "BLOCK") {
		return "Hệ thống đang có vấn đề dữ liệu hoặc đối soát; Hermes không tăng vốn lúc này."
	}
	if strings.TrimSpace(b.HermesReport.ActionLine) != "" && !strings.Contains(strings.ToUpper(b.HermesReport.ActionLine), "READ_ONLY") {
		return b.HermesReport.ActionLine
	}
	return "Hermes chưa mở lệnh mới; tiếp tục chờ vùng giá và dòng tiền cho tỷ lệ lợi nhuận/rủi ro tốt hơn."
}

func renderHermesExecutive(b HermesOperationsBrief) string {
	var x strings.Builder
	fmt.Fprintf(&x, "BẢN PHÂN TÍCH VÀ KẾ HOẠCH CỦA HERMES\n%s | %s\n\n1. KẾT LUẬN NHANH\n%s\nKế hoạch: %s | Trạng thái BTC: %s\nHệ thống: %s | Đối soát tài khoản: %s\n\n", b.LocalTime, strings.ToUpper(b.Kind), briefAction(b), viTerm(string(b.Bot.PlanState)), viTerm(string(b.Bot.BTCPermission)), viTerm(b.Bot.DoctorStatus), viTerm(b.Bot.ReconcileSafetyStatus))
	fmt.Fprintf(&x, "2. BỨC TRANH CHUNG\nTổng vốn hóa thị trường: %.2f nghìn tỷ USD | Khối lượng 24 giờ: %.1f tỷ USD\nTỷ trọng BTC: %.2f%% | Tâm lý: %d/100 — %s | EUR/USD: %.4f\nBTC: $%.0f — thị trường %s, sức mạnh xu hướng %.1f/100\nXu hướng tuần/ngày/4 giờ: %s / %s / %s\nDòng tiền: %s (%.2f) | Giai đoạn: %s (%.0f/100)\nRủi ro chung: %s | Nguy cơ bắt dao rơi: %s | Nguy cơ mua đuổi: %s\n%s | %s | %s\n\n", b.Global.MarketCapUSD/1e12, b.Global.VolumeUSD/1e9, b.Global.BTCDominance, b.Global.FearGreed, viTerm(b.Global.FearGreedLabel), b.Global.EURUSD, b.Bot.BTC.Price, viTerm(b.Bot.BTC.Regime), b.Bot.BTC.TrendScore, viTerm(b.Bot.BTC.WeeklyBias), viTerm(b.Bot.BTC.DailyBias), viTerm(b.Bot.BTC.FourHourBias), viTerm(b.Bot.BTC.FlowBias), b.Bot.BTC.FlowScore, viTerm(b.Bot.BTC.AccumulationPhase), b.Bot.BTC.AccumulationScore, viTerm(string(b.Bot.BTC.RiskLevel)), viTerm(string(b.Bot.BTC.FallingKnifeRisk)), viTerm(string(b.Bot.BTC.FomoRisk)), briefZone("Vùng hỗ trợ", b.Bot.BTC.SupportZone.Low, b.Bot.BTC.SupportZone.High), briefZone("Vùng cản", b.Bot.BTC.ResistanceZone.Low, b.Bot.BTC.ResistanceZone.High), briefZone("Mốc sai kịch bản", b.Bot.BTC.InvalidationZone.Low, b.Bot.BTC.InvalidationZone.High))
	x.WriteString("3. DÒNG TIỀN LỚN VÀ THANH KHOẢN\n")
	for _, m := range b.MM {
		fmt.Fprintf(&x, "- %s: %s, điểm %.0f/100; chất lượng dữ liệu %.0f%%; có %d tín hiệu chính; áp lực bán hiện tại: %s.\n  %s\n", m.Symbol, viTerm(m.Verdict), m.Score*100, m.Quality*100, m.Core, yesNo(m.AskPressure), briefCut(strings.Join(m.Reasons, "; "), 170))
	}
	x.WriteString("\n4. VÙNG CÓ THỂ GOM VÀ VÙNG CẦN PHÂN PHỐI\n")
	for _, z := range b.Zones {
		if z.Score < 20 {
			continue
		}
		kind := "Vùng có thể gom"
		if strings.Contains(z.Kind, "DISTRIBUTION") {
			kind = "Vùng cần theo dõi phân phối/chốt giảm"
		}
		fmt.Fprintf(&x, "- %s — %s: $%.4f đến $%.4f | độ tin cậy %.0f%%\n  Điểm ủng hộ: %s. Còn thiếu: %s.\n  Chờ xác nhận: %s | Kịch bản sai khi: %s\n", z.Symbol, kind, z.Low, z.High, z.Confidence*100, firstBrief(strings.Join(z.Evidence, ", "), "chưa có"), firstBrief(strings.Join(z.Missing, ", "), "không"), briefCut(z.Trigger, 150), z.Invalidation)
	}
	fmt.Fprintf(&x, "\n5. KẾ HOẠCH PHÂN BỔ VỐN\nGiữ tiền dự phòng: %.1f%% | Tổng mức vốn tối đa được triển khai: %.1f%%\n", b.ReservePct, b.PortfolioCapPct)
	for _, a := range b.Allocations {
		fmt.Fprintf(&x, "- %s: tối đa %.1f%% vốn; hiện dùng %.1f%%.\n  Nếu tín hiệu cải thiện: thăm dò %.1f%% → mở vị thế %.1f%% → tăng thêm tối đa %.1f%%.\n  Hiện tại: %s. Điều kiện tiếp theo: %s\n", a.Symbol, a.CeilingPct, a.CurrentPct, a.ProbePct, a.OpenPct, a.ScalePct, viTerm(a.State), briefCut(a.Condition, 150))
	}
	x.WriteString("\n6. KẾ HOẠCH CHO TỪNG TÀI SẢN\n")
	for _, a := range b.Hermes.Assets {
		fmt.Fprintf(&x, "- %s: %s, mức sẵn sàng %.0f%%.\n  Vùng mua dự kiến $%.4f–$%.4f | Cắt kịch bản dưới $%.4f | Mục tiêu $%.4f | Lãi/rủi ro %.2f lần.\n  Dấu chân dòng tiền lớn: %s %.0f/100 | Dòng tiền %s %.2f | Thanh khoản hạng %s %.0f/100.\n  Cần chờ: %s\n", a.Symbol, viTerm(a.State), a.Readiness*100, a.EntryZoneLow, a.EntryZoneHigh, a.Invalidation, a.Target, a.RR, viTerm(a.MMCase), a.MMScore, viTerm(a.FlowBias), a.FlowScore, a.LiquidityGrade, a.LiquidityScore, briefCut(a.NextTrigger, 160))
	}
	fmt.Fprintf(&x, "\n7. BA KỊCH BẢN CẦN THEO DÕI\nKịch bản chính: %s\nKịch bản tốt lên: %s\nKịch bản xấu đi: %s\n\n8. VĨ MÔ, CHÍNH SÁCH VÀ TIN TỨC\n%s\nTổng hợp tin: %s\n\n9. TRẠNG THÁI BOT\nLệnh đang chờ: %d | Vị thế đang giữ: %d. Hermes vẫn tự vận hành và chỉ đặt lệnh giới hạn khi đủ điều kiện.\nAn toàn: chỉ mua giao ngay; không vay đòn bẩy, không bán khống, không dùng lệnh thị trường.\n\n10. ĐỘ TIN CẬY CỦA DỮ LIỆU\n", briefCut(b.Scenario.BTC.BaseCase, 240), briefCut(b.Scenario.BTC.BullUnlock, 240), briefCut(b.Scenario.BTC.BearInvalidation, 240), briefCut(firstBrief(b.MacroSummary, "Chưa có dữ liệu vĩ mô mới đủ tin cậy; Hermes không tự suy diễn."), 450), briefCut(firstBrief(b.ResearchSummary, "Chưa có bản tin mới."), 220), b.Bot.OpenLiveOrders, b.Bot.LivePositions)
	for _, q := range b.Sources {
		state := "còn mới"
		if !q.Fresh {
			state = "đã cũ"
		}
		fmt.Fprintf(&x, "- %s: %s, cập nhật cách đây %d phút (%s).\n", q.Name, state, q.AgeMinutes, q.Detail)
	}
	if len(b.Missing) > 0 {
		fmt.Fprintf(&x, "Chưa có: %s. Hermes không dùng phần thiếu để nâng mức tin cậy.\n", strings.Join(b.Missing, ", "))
	}
	return x.String()
}

func renderHermesPlan(b HermesOperationsBrief) string {
	var x strings.Builder
	x.WriteString("HERMES — KẾ HOẠCH PHÂN BỔ VỐN\n")
	fmt.Fprintf(&x, "Giữ dự phòng %.1f%% vốn; tổng mức triển khai không vượt %.1f%%.\n", b.ReservePct, b.PortfolioCapPct)
	for _, a := range b.Allocations {
		fmt.Fprintf(&x, "\n%s — %s\nTỷ trọng tối đa %.1f%%; hiện dùng %.1f%%.\nKhi đủ tín hiệu: thăm dò %.1f%% → mở vị thế %.1f%% → tăng thêm tối đa %.1f%%.\nCần chờ: %s\n", a.Symbol, viTerm(a.State), a.CeilingPct, a.CurrentPct, a.ProbePct, a.OpenPct, a.ScalePct, a.Condition)
	}
	x.WriteString("\nCÁC VÙNG GIÁ QUAN TRỌNG\n")
	for _, z := range b.Zones {
		kind := "có thể gom"
		if strings.Contains(z.Kind, "DISTRIBUTION") {
			kind = "có thể phân phối/chốt giảm"
		}
		fmt.Fprintf(&x, "- %s: vùng %s $%.4f–$%.4f, độ tin cậy %.0f%%. Chờ: %s\n", z.Symbol, kind, z.Low, z.High, z.Confidence*100, briefCut(z.Trigger, 150))
	}
	fmt.Fprintf(&x, "\nKịch bản chính: %s\nNếu tốt lên: %s\nNếu xấu đi: %s\n", b.Scenario.BTC.BaseCase, b.Scenario.BTC.BullUnlock, b.Scenario.BTC.BearInvalidation)
	return x.String()
}

func renderHermesWhy(b HermesOperationsBrief) string {
	return fmt.Sprintf("HERMES — VÌ SAO CHƯA HÀNH ĐỘNG?\n\nKết luận: %s\nBTC đang ở giai đoạn %s, sức mạnh xu hướng %.1f/100; dòng tiền %s (%.2f).\nDấu chân dòng tiền lớn trên BTC: %s.\nRủi ro đáng chú ý: %s; nguy cơ bắt dao rơi %s; mua đuổi %s.\n\nĐiều kiện để hành động: %s\nKịch bản không còn đúng khi: %s\n\nCác nhãn thị trường yếu chỉ làm Hermes giảm tỷ trọng. Bot chỉ bị khóa khi dữ liệu, tài khoản, sàn hoặc đối soát có lỗi.\n", briefAction(b), viTerm(b.Bot.BTC.AccumulationPhase), b.Bot.BTC.TrendScore, viTerm(b.Bot.BTC.FlowBias), b.Bot.BTC.FlowScore, briefMMLine(b.MM), b.Bot.RiskGovernorSummary, viTerm(string(b.Bot.BTC.FallingKnifeRisk)), viTerm(string(b.Bot.BTC.FomoRisk)), b.Scenario.BTC.BullUnlock, b.Scenario.BTC.BearInvalidation)
}

func briefMMLine(mm []HermesBriefMM) string {
	for _, m := range mm {
		if m.Symbol == "BTCUSDT" {
			return fmt.Sprintf("%s, điểm %.0f/100, %d tín hiệu chính, chất lượng dữ liệu %.0f%%", viTerm(m.Verdict), m.Score*100, m.Core, m.Quality*100)
		}
	}
	return "chưa có dữ liệu"
}

func renderHermesRisk(b HermesOperationsBrief) string {
	return fmt.Sprintf("HERMES — QUẢN TRỊ RỦI RO\nDừng khẩn cấp: %s\nSức khỏe hệ thống: %s\nDữ liệu: %s — %s\nĐối soát tài khoản: %s — %s\nGiới hạn rủi ro: %s — %s\nLệnh đang chờ: %d | Vị thế đang giữ: %d\n\nMọi mức vốn đều tính theo phần trăm tài sản hiện có. Bot chỉ bị chặn khi dữ liệu lỗi/cũ, tài khoản lệch, sàn không sẵn sàng hoặc vượt giới hạn vốn.\n", yesNo(b.Bot.OperatorHalt), viTerm(b.Bot.DoctorStatus), viTerm(b.Bot.DataHealthStatus), b.Bot.DataHealthSummary, viTerm(b.Bot.ReconcileSafetyStatus), b.Bot.ReconcileSafetySummary, viTerm(b.Bot.RiskGovernorStatus), b.Bot.RiskGovernorSummary, b.Bot.OpenLiveOrders, b.Bot.LivePositions)
}

func renderHermesFlow(b HermesOperationsBrief) string {
	var x strings.Builder
	x.WriteString("HERMES — DÒNG TIỀN LỚN VÀ THANH KHOẢN\n")
	for _, m := range b.MM {
		fmt.Fprintf(&x, "\n%s: %s, điểm %.0f/100; chất lượng dữ liệu %.0f%%.\nCó %d tín hiệu chính; áp lực bán hiện tại: %s.\nChi tiết: %s\n", m.Symbol, viTerm(m.Verdict), m.Score*100, m.Quality*100, m.Core, yesNo(m.AskPressure), strings.Join(m.Reasons, "; "))
	}
	x.WriteString("\nPhí hợp đồng và chênh lệch giá chỉ dùng để tham khảo. Hermes chỉ tăng độ tin cậy khi giao dịch thực tế và sổ lệnh cùng xác nhận.\n")
	return x.String()
}

func renderHermesSchedule(b HermesOperationsBrief) string {
	return fmt.Sprintf("HERMES — LỊCH LÀM VIỆC (%s)\n07:00: kế hoạch đầu ngày — bối cảnh chung, vùng giá và ngân sách rủi ro.\n13:00: đánh giá giữa ngày — dòng tiền, mức tin cậy và tỷ trọng vốn.\nMỗi 4 giờ: chỉ gửi những thay đổi quan trọng và điều kiện sắp đạt.\n23:00: tổng kết — lệnh, kết quả và kế hoạch ngày sau.\nMỗi 15 phút: kiểm tra thị trường, tài khoản, lệnh và điểm thoát.\nMỗi 60 phút: cập nhật quyết định tự động của Hermes.\nThông báo ngay khi có lệnh, khớp lệnh, hủy lệnh, giảm vị thế, thoát vị thế hoặc sự cố an toàn.\n", b.Timezone)
}
