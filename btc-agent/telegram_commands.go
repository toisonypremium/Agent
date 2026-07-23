package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/freeapi"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/notify"
	"btc-agent/internal/reportio"
	"btc-agent/internal/storage"
	"btc-agent/internal/usertext"
)

type telegramCommandState struct {
	LastUpdateID int                    `json:"last_update_id"`
	UpdatedAt    time.Time              `json:"updated_at"`
	RateLimits   map[string][]time.Time `json:"rate_limits,omitempty"`
}

const telegramMaxRequestsPerMinute = 5

var telegramRateMu sync.Mutex
var telegramRateWindow = map[int64][]time.Time{}

func telegramRateAllow(chatID int64) bool {
	telegramRateMu.Lock()
	defer telegramRateMu.Unlock()
	now := time.Now()
	window := telegramRateWindow[chatID]
	valid := make([]time.Time, 0, len(window))
	for _, t := range window {
		if now.Sub(t) < time.Minute {
			valid = append(valid, t)
		}
	}
	if len(valid) >= telegramMaxRequestsPerMinute {
		telegramRateWindow[chatID] = valid
		return false
	}
	telegramRateWindow[chatID] = append(valid, now)
	return true
}

func loadTelegramRateLimits(state telegramCommandState) {
	telegramRateMu.Lock()
	defer telegramRateMu.Unlock()
	telegramRateWindow = map[int64][]time.Time{}
	for chatID, timestamps := range state.RateLimits {
		id, err := strconv.ParseInt(chatID, 10, 64)
		if err != nil {
			continue
		}
		for _, timestamp := range timestamps {
			if time.Since(timestamp) < time.Minute {
				telegramRateWindow[id] = append(telegramRateWindow[id], timestamp)
			}
		}
	}
}

func saveTelegramRateLimits(state *telegramCommandState) {
	telegramRateMu.Lock()
	defer telegramRateMu.Unlock()
	state.RateLimits = map[string][]time.Time{}
	now := time.Now()
	for chatID, timestamps := range telegramRateWindow {
		valid := make([]time.Time, 0, len(timestamps))
		for _, timestamp := range timestamps {
			if now.Sub(timestamp) < time.Minute {
				valid = append(valid, timestamp)
			}
		}
		if len(valid) > 0 {
			state.RateLimits[strconv.FormatInt(chatID, 10)] = valid
		}
	}
}

func runTelegramCommands(ctx context.Context, cfg config.Config, db *storage.DB) error {
	token := os.Getenv("TELEGRAM_TOKEN")
	chatID := firstNonEmpty(cfg.Notify.TelegramChatID, os.Getenv("TELEGRAM_CHAT_ID"))
	if strings.TrimSpace(token) == "" || strings.TrimSpace(chatID) == "" {
		return fmt.Errorf("telegram command config missing token/chat_id")
	}
	state := loadTelegramCommandState()
	loadTelegramRateLimits(state)
	updates, err := notify.TelegramGetUpdates(ctx, token, state.LastUpdateID+1)
	if err != nil {
		return err
	}
	for _, update := range updates {
		if update.UpdateID <= state.LastUpdateID {
			continue
		}
		advance := true
		if telegramChatAllowed(chatID, update.Message.Chat.ID) {
			if !telegramRateAllow(update.Message.Chat.ID) {
				log.Printf("[TelegramCommands] rate limit exceeded for chat %d", update.Message.Chat.ID)
				continue
			}
			cmd := normalizeTelegramCommand(update.Message.Text)
			// Free-text Hermes question routing (before command check)
			if cmd == "" {
				if trigger, ok := parseTelegramHermesRequest(update.Message.Text); ok {
					result := runHermesTelegramReply(context.Background(), cfg, db, trigger)
					telegramToken := os.Getenv("TELEGRAM_TOKEN")
					telegramChatID := firstNonEmpty(cfg.Notify.TelegramChatID, os.Getenv("TELEGRAM_CHAT_ID"))
					if err := func() error {
						_, e := notify.TelegramSendAndHideKeyboard(ctx, telegramToken, telegramChatID, usertext.TelegramVietnamese(result))
						return e
					}(); err != nil {
						log.Printf("[TelegramCommands] hermes free-text reply error: %v", err)
					} else {
						log.Printf("[TelegramCommands] hermes free-text reply sent ok")
					}
				}
			}
			if cmd != "" {
				if cmd == "/menu" || cmd == "/start" {
					menuText := hermesTelegramMenuText(cfg)
					if _, err := notify.TelegramSendMenu(ctx, token, chatID, menuText); err != nil {
						return err
					}
					log.Printf("[TelegramCommands] Hermes menu sent ok [%s]", cmd)
					state.LastUpdateID = update.UpdateID
					state.UpdatedAt = time.Now()
					saveTelegramRateLimits(&state)
					if err := saveTelegramCommandState(state); err != nil {
						return err
					}
					continue
				}
				text, ok := buildReadOnlyTelegramCommandReplyWithConfig(cfg, cmd)
				if ok {
					if err := func() error {
						_, e := notify.TelegramSendAndHideKeyboard(ctx, token, chatID, usertext.TelegramVietnamese(text))
						return e
					}(); err != nil {
						log.Printf("[TelegramCommands] reply error [%s]: %v", cmd, err)
						advance = false
						return err
					}
					log.Printf("[TelegramCommands] reply sent ok [%s]", cmd)
				} else {
					log.Printf("[TelegramCommands] command ignored [%s]", cmd)
				}
			}
		}
		if advance {
			state.LastUpdateID = update.UpdateID
			state.UpdatedAt = time.Now()
			saveTelegramRateLimits(&state)
			if err := saveTelegramCommandState(state); err != nil {
				return err
			}
		}
	}
	return nil
}

func telegramChatAllowed(configured string, actual int64) bool {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return false
	}
	return configured == strconv.FormatInt(actual, 10)
}

func normalizeTelegramCommand(text string) string {
	text = strings.TrimSpace(text)
	iconCommands := map[string]string{
		"📊 Trạng thái": "/status", "🧠 Phân tích": "/hermes", "❓ Lý do": "/why",
		"🗺 Kế hoạch": "/plan", "🕒 Lịch": "/schedule", "🌊 Dòng tiền": "/flow",
		"🌐 Vĩ mô": "/macro", "🛡 Rủi ro": "/risk", "🎯 Điểm thoát": "/exits",
		"💼 Vị thế": "/positions", "📋 Lệnh chờ": "/orders", "🧾 Nguồn dữ liệu": "/sources",
		"⚙️ Menu": "/menu", "⌨️ Ẩn bàn phím": "/hide",
	}
	if cmd, ok := iconCommands[text]; ok {
		return cmd
	}
	if !strings.HasPrefix(text, "/") {
		return ""
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	cmd := strings.ToLower(fields[0])
	if at := strings.Index(cmd, "@"); at >= 0 {
		cmd = cmd[:at]
	}
	aliases := map[string]string{"/trangthai": "/status", "/lydo": "/why", "/kehoach": "/plan", "/lich": "/schedule", "/dongtien": "/flow", "/tienvi": "/macro", "/ruiro": "/risk", "/thoat": "/exits", "/vithe": "/positions", "/lenh": "/orders", "/coin": "/coins", "/dieukien": "/next"}
	if translated, ok := aliases[cmd]; ok {
		cmd = translated
	}
	switch cmd {
	case "/menu", "/start", "/hide", "/anbanphim", "/trogiup", "/macro", "/sources", "/status", "/why", "/plan", "/schedule", "/flow", "/coins", "/filters", "/scorecard", "/allocation", "/capital", "/universe", "/trigger", "/orders", "/positions", "/doctor", "/supervisor", "/next", "/risk", "/hermes", "/h", "/ask", "/exits", "/audit", "/help":
		return cmd
	default:
		return ""
	}
}

func buildReadOnlyTelegramCommandReplyWithConfig(cfg config.Config, cmd string) (string, bool) {
	snapshot, snapshotOK := loadBotRuntimeSnapshotReport()
	scenario, scenarioOK := loadScenarioReportFile()
	supervisor, supervisorOK := loadLatestSupervisorReportFile()
	switch cmd {
	case "/help", "/trogiup":
		return telegramCommandsHelp(), true
	case "/hide", "/anbanphim":
		return "Đã ẩn bàn phím. Gõ /menu khi cần mở lại.", true
	case "/macro":
		return telegramCommandFreeAPI(false), true
	case "/sources":
		return telegramCommandFreeAPI(true), true
	case "/status", "/hermes", "/h":
		return renderHermesExecutive(buildHermesOperationsBrief(cfg, "interactive status")), true
	case "/why":
		return renderHermesWhy(buildHermesOperationsBrief(cfg, "decision explanation")), true
	case "/plan":
		return renderHermesPlan(buildHermesOperationsBrief(cfg, "capital plan")), true
	case "/schedule":
		return renderHermesSchedule(buildHermesOperationsBrief(cfg, "operating schedule")), true
	case "/flow":
		return renderHermesFlow(buildHermesOperationsBrief(cfg, "flow detail")), true
	case "/coins":
		if !scenarioOK {
			return "Chưa có bản đánh giá thị trường. Hãy chờ bot giám sát chạy thêm một lượt.", true
		}
		return telegramCommandCoins(scenario), true
	case "/filters":
		filterReport, ok := loadFilterAttributionReportFile()
		if !ok {
			return "Chưa có báo cáo các điều kiện đang chặn. Hãy chờ bot cập nhật.", true
		}
		return telegramCommandFilters(filterReport), true
	case "/scorecard":
		report, ok := loadTechnicalScorecardReportFile()
		if !ok {
			return "Chưa có bảng điểm kỹ thuật. Hãy chờ bot cập nhật.", true
		}
		return telegramCommandScorecard(report), true
	case "/allocation", "/capital":
		report, ok := loadCapitalPlanResearchReportFile()
		if !ok {
			return "Chưa có kế hoạch phân bổ vốn. Hãy chờ bot cập nhật.", true
		}
		return telegramCommandAllocation(report), true
	case "/universe":
		report, ok := loadUniverseResearchReportFile()
		if !ok {
			return "Chưa có universe research report. Chạy universe-research hoặc chờ report được tạo.", true
		}
		return telegramCommandUniverse(report), true
	case "/trigger":
		if !scenarioOK {
			return "Chưa có scenario report. Chờ chu kỳ giám sát tiếp theo.", true
		}
		return telegramCommandScenarioTrigger(scenario), true
	case "/orders":
		if !snapshotOK || !scenarioOK {
			return "Chưa có dữ liệu trạng thái mới. Hãy chờ bot cập nhật.", true
		}
		return telegramCommandOrders(snapshot, scenario), true
	case "/positions":
		positionReport, ok := loadLivePositionReportFile()
		if !ok {
			return "Chưa có báo cáo tài sản đang giữ. Hãy chờ bot đối soát tài khoản.", true
		}
		return telegramCommandPositions(positionReport), true
	case "/doctor":
		if !snapshotOK {
			return "Chưa có dữ liệu sức khỏe hệ thống. Hãy chờ bot cập nhật.", true
		}
		return telegramCommandDoctor(snapshot), true
	case "/supervisor":
		if !snapshotOK {
			return "Chưa có dữ liệu sức khỏe hệ thống. Hãy chờ bot cập nhật.", true
		}
		return telegramCommandSupervisor(snapshot, supervisor, supervisorOK), true
	case "/next":
		if !scenarioOK {
			return "Chưa có bản đánh giá thị trường. Hãy chờ bot giám sát chạy thêm một lượt.", true
		}
		return telegramCommandNext(scenario), true
	case "/risk":
		base := renderHermesRisk(buildHermesOperationsBrief(cfg, "risk detail"))
		return base + "\n\n" + telegramProtectionStatus(cfg), true
	case "/ask":
		return "Dùng: /ask <câu hỏi>, ví dụ: /ask tại sao bot chưa vào lệnh?", true
	case "/exits":
		brief := buildHermesOperationsBrief(cfg, "exit state")
		return telegramCommandExits(brief.Hermes), true
	case "/audit":
		return telegramCommandAudit(), true
	default:
		return "", false
	}
}

func telegramCommandFreeAPI(sources bool) string {
	r, e := freeapi.Load("reports")
	if e != nil {
		return "FREE API: chưa có snapshot; Hermes sẽ refresh ở chu kỳ kế tiếp."
	}
	if sources {
		var b strings.Builder
		b.WriteString("HERMES — FREE API SOURCES\n")
		for _, s := range r.Sources {
			fmt.Fprintf(&b, "- %s fresh=%v age=%dm err=%s\n", s.Name, s.Fresh, s.AgeMinutes, s.Error)
		}
		return b.String()
	}
	return fmt.Sprintf("HERMES — MACRO / SENTIMENT / FX\nGlobal cap: %.0f USD\nGlobal volume: %.0f USD\nBTC dominance: %.2f%%\nFear & Greed: %d (%s)\nEUR/USD: %.5f\n%s funding: %.6f%%\n%s open interest: %.0f USD\nDeFi TVL: %.0f USD\nNews items: %d\nFree API chỉ là context; không tự tạo hard block hay order authority.", r.GlobalMarketCapUSD, r.GlobalVolumeUSD, r.BTCDominancePct, r.FearGreedValue, r.FearGreedLabel, r.EURUSD, r.DerivativesSymbol, r.FundingRate*100, r.DerivativesSymbol, r.OpenInterestUSD, r.DeFiTVLUSD, len(r.News))
}

func hermesTelegramMenuText(cfg config.Config) string {
	tz := cfg.App.Timezone
	if tz == "" {
		tz = "Asia/Ho_Chi_Minh"
	}
	return fmt.Sprintf("HERMES — MENU NHANH\n\nChọn một mục bên dưới. Bàn phím sẽ tự ẩn sau khi chọn.\nGõ /anbanphim để ẩn ngay; /menu để mở lại.\n\nBot chỉ nhận lệnh xem thông tin, không nhận lệnh mua bán qua Telegram.\nMúi giờ: %s.", tz)
}
func telegramCommandsHelp() string {
	return strings.TrimSpace(`HERMES — TRỢ GIÚP
/menu — mở menu nhanh
/anbanphim — ẩn bàn phím
/trangthai — tình trạng bot và quyền đặt lệnh
/lydo — vì sao bot chưa hành động
/kehoach — kế hoạch dùng vốn
/lich — lịch hoạt động
/dongtien — dòng tiền lớn và thanh khoản
/tienvi — bối cảnh kinh tế và tâm lý
/ruiro — các lớp bảo vệ vốn
/thoat — tình trạng bảo vệ lợi nhuận và cảnh báo DCA
/vithe — tài sản đang giữ
/lenh — lệnh đang chờ
/coin — tình trạng từng đồng
/dieukien — điều kiện hành động tiếp theo

Telegram chỉ dùng để xem. Hermes không nhận lệnh mua hoặc bán tại đây.`) + "\n"
}

func telegramCommandCoins(r ScenarioReport) string {
	var b strings.Builder
	b.WriteString("BTC Agent — Coins\n")
	for _, coin := range r.Coins {
		b.WriteString(fmt.Sprintf("- %s %s %.0f%% | rank=%d score=%.2f | MM=%s %.0f | Liq=%s %.0f | RR %.2f | desired=%d\n", coin.Symbol, coin.State, coin.ReadinessScore*100, coin.RotationRank, coin.RotationScore, emptyStringDefault(coin.MMCase, "n/a"), coin.MMScore, emptyStringDefault(coin.LiquidityGrade, "n/a"), coin.LiquidityScore, coin.RewardRisk, coin.DesiredLayers))
		if coin.NextTrigger != "" {
			b.WriteString("  Next: " + coin.NextTrigger + "\n")
		}
	}
	return b.String()
}

func telegramCommandOrders(s BotRuntimeSnapshot, r ScenarioReport) string {
	var b strings.Builder
	b.WriteString("BTC Agent — Orders\n")
	b.WriteString(fmt.Sprintf("Open live=%d | desired=%d | placed=%d | canceled=%d | replaced=%d | blocked=%d\n", s.OpenLiveOrders, s.DesiredOrders, s.PlacedOrders, s.CanceledOrders, s.ReplacedOrders, s.BlockedOrders))
	if len(s.OpenOrders) == 0 {
		b.WriteString("Không có live order đang mở.\n")
	} else {
		for _, order := range s.OpenOrders {
			b.WriteString(fmt.Sprintf("- %s %s %s px=%.8f qty=%.8f notional=%.2f status=%s layer=%d\n", order.Symbol, order.Side, order.Type, order.Price, order.Quantity, order.Notional, order.Status, order.LayerIndex))
		}
	}
	b.WriteString("Can submit now: " + fmt.Sprint(r.CanSubmitOrder) + "\n")
	b.WriteString("Read-only: Telegram không đặt/hủy/sửa lệnh.\n")
	return b.String()
}

func telegramCommandFilters(r FilterAttributionReport) string {
	var b strings.Builder
	b.WriteString("BTC Agent — Filters\n")
	b.WriteString(r.Summary + "\n")
	if len(r.Aggregate) > 0 {
		b.WriteString("Top blockers:\n")
		for _, row := range firstFilterAggregateRows(r.Aggregate, 5) {
			b.WriteString(fmt.Sprintf("- %s: %d\n", row.Key, row.Count))
		}
	}
	for _, coin := range firstFilterCoinRows(r.Coins, 3) {
		b.WriteString(fmt.Sprintf("%s %s setup=%.2f top=%s hard=%d soft=%d desired=%d\n", coin.Symbol, coin.State, coin.SetupScore, emptyStringDefault(coin.TopBlockerKey, "none"), coin.FailedHard, coin.FailedSoft, coin.DesiredLayers))
		if coin.NextTrigger != "" {
			b.WriteString("Next: " + coin.NextTrigger + "\n")
		}
	}
	if len(r.NearActionable) > 0 {
		b.WriteString(fmt.Sprintf("Near-actionable research: %d\n", len(r.NearActionable)))
	}
	b.WriteString("Read-only: filter report không đổi threshold, không bypass ACTIVE_LIMIT.\n")
	return b.String()
}

func telegramCommandScorecard(r TechnicalScorecardReport) string {
	var b strings.Builder
	b.WriteString("BTC Agent — Scorecard\n")
	b.WriteString(r.Summary + "\n")
	for _, coin := range firstTechnicalScorecardCoins(r.Coins, 3) {
		b.WriteString(fmt.Sprintf("- %s %s score=%.0f%% verdict=%s RR=%.2f top=%s\n", coin.Symbol, coin.State, coin.TechnicalScore*100, coin.Verdict, coin.RewardRisk, emptyStringDefault(coin.TopBlockerKey, "none")))
		if coin.NextTrigger != "" {
			b.WriteString("  Next: " + coin.NextTrigger + "\n")
		}
	}
	b.WriteString("Read-only: scorecard không đặt lệnh, không bypass ACTIVE_LIMIT.\n")
	return b.String()
}

func telegramCommandAllocation(r CapitalPlanResearchReport) string {
	var b strings.Builder
	b.WriteString("BTC Agent — Capital Research\n")
	b.WriteString(r.Summary + "\n")
	for _, coin := range firstCapitalPlanCoins(r.Coins, 3) {
		b.WriteString(fmt.Sprintf("- %s %s current=%.1f%% suggested=%.1f%% max=%.2f score=%.1f verdict=%s layers=%d\n", coin.Symbol, coin.State, coin.CurrentConfigAllocation*100, coin.SuggestedResearchAllocation*100, coin.MaxResearchNotional, coin.OpportunityScore, coin.OpportunityVerdict, coin.SuggestedLayers))
	}
	b.WriteString("Research-only: Telegram không sửa config allocation, không bypass ACTIVE_LIMIT.\n")
	return b.String()
}

func telegramCommandUniverse(r agent2.UniverseResearchReport) string {
	var b strings.Builder
	b.WriteString("BTC Agent — Universe Research\n")
	b.WriteString(r.Summary + "\n")
	for _, row := range firstUniverseRows(r.TopCandidates, 5) {
		b.WriteString(fmt.Sprintf("- %s score=%.1f verdict=%s state=%s production=%v data=%s\n", row.Symbol, row.OpportunityScore, row.OpportunityVerdict, row.State, row.InProduction, row.DataStatus))
	}
	b.WriteString("Research-only: universe không tự thay production assets, không đặt lệnh.\n")
	return b.String()
}

func telegramCommandScenarioTrigger(r ScenarioReport) string {
	var b strings.Builder
	b.WriteString("BTC Agent — Trigger tiếp theo\n")
	if len(r.NearTriggers) > 0 {
		b.WriteString("Next: " + r.NearTriggers[0] + "\n")
	}
	for _, blocker := range firstStrings(r.Blockers, 5) {
		b.WriteString("- " + blocker + "\n")
	}
	b.WriteString("Chỉ dùng để xem: điều kiện này không tự đặt lệnh và không bỏ qua bước kiểm tra cuối.\n")
	return b.String()
}

func telegramCommandPositions(r liveguard.LiveLedgerReport) string {
	var b strings.Builder
	b.WriteString("BTC Agent — Positions\n")
	b.WriteString(liveguard.LiveLedgerSummary(r) + "\n")
	if len(r.Positions) == 0 {
		b.WriteString("Không có vị thế live trong ledger.\n")
	} else {
		for _, p := range r.Positions {
			b.WriteString(fmt.Sprintf("- %s qty=%.8f entry=%.8f cost=%.2f\n", p.Symbol, p.Quantity, p.AvgEntryPrice, p.CostBasis))
		}
	}
	if len(r.ManualCheckRequired) > 0 {
		b.WriteString("Manual check:\n")
		for _, item := range firstStrings(r.ManualCheckRequired, 5) {
			b.WriteString("- " + item + "\n")
		}
	}
	b.WriteString("Telegram chỉ dùng để xem, không đóng hoặc mở vị thế.\n")
	return b.String()
}

func telegramCommandDoctor(s BotRuntimeSnapshot) string {
	return fmt.Sprintf("HERMES — SỨC KHỎE HỆ THỐNG\nTình trạng: %s\nTóm tắt: %s\nDữ liệu: %s\nĐối soát tài khoản: %s\nBảo vệ vốn: %s\n", emptyStringDefault(s.DoctorStatus, "chưa rõ"), emptyStringDefault(s.DoctorSummary, "không có"), emptyStringDefault(s.DataHealthSummary, "chưa rõ"), emptyStringDefault(s.ReconcileSafetySummary, "chưa rõ"), emptyStringDefault(s.RiskGovernorSummary, "chưa rõ"))
}

func telegramCommandSupervisor(s BotRuntimeSnapshot, supervisor liveguard.SupervisorResult, ok bool) string {
	var b strings.Builder
	b.WriteString("HERMES — GIÁM SÁT HOẠT ĐỘNG\n")
	b.WriteString(fmt.Sprintf("Tình trạng: %s | Việc vừa làm: %s | Đang hoạt động: %s\n", emptyStringDefault(s.SupervisorStatus, "chưa rõ"), emptyStringDefault(s.SupervisorAction, "chưa rõ"), vietnameseBool(s.SupervisorAlive)))
	b.WriteString("Tóm tắt: " + emptyStringDefault(s.SupervisorSummary, "không có") + "\n")
	if ok && supervisor.Managed != nil {
		m := supervisor.Managed
		b.WriteString(fmt.Sprintf("Quản lý lệnh: %s | Dự kiến %d | Đã đặt %d | Đã hủy %d | Đã thay %d | Bị chặn %d\n", m.Status, len(m.Desired), len(m.Placed), len(m.Canceled), len(m.Replaced), len(m.Blocked)))
	}
	b.WriteString("Lần kiểm tra tiếp theo: " + emptyStringDefault(s.NextLiveSupervisorCycle, "chưa rõ") + "\n")
	return b.String()
}

func telegramCommandNext(r ScenarioReport) string {
	var b strings.Builder
	b.WriteString("BTC Agent — Điều kiện tiếp theo\n")
	b.WriteString("BTC:\n")
	for _, item := range firstStrings(r.BTC.UnlockConditions, 4) {
		b.WriteString("- " + item + "\n")
	}
	for _, coin := range firstCoinScenarios(r.Coins, 3) {
		b.WriteString(fmt.Sprintf("\n%s:\n", coin.Symbol))
		for _, item := range firstStrings(coin.UnlockConditions, 4) {
			b.WriteString("- " + item + "\n")
		}
	}
	return b.String()
}

func loadBotRuntimeSnapshotReport() (BotRuntimeSnapshot, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "bot_state_latest.json"))
	if err != nil {
		return BotRuntimeSnapshot{}, false
	}
	var out BotRuntimeSnapshot
	if err := json.Unmarshal(b, &out); err != nil {
		return BotRuntimeSnapshot{}, false
	}
	return out, true
}

func loadScenarioReportFile() (ScenarioReport, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "scenario_latest.json"))
	if err != nil {
		return ScenarioReport{}, false
	}
	var out ScenarioReport
	if err := json.Unmarshal(b, &out); err != nil {
		return ScenarioReport{}, false
	}
	return out, true
}

func loadLatestSupervisorReportFile() (liveguard.SupervisorResult, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "live_supervisor_latest.json"))
	if err != nil {
		return liveguard.SupervisorResult{}, false
	}
	var out liveguard.SupervisorResult
	if err := json.Unmarshal(b, &out); err != nil {
		return liveguard.SupervisorResult{}, false
	}
	return out, true
}

func loadFilterAttributionReportFile() (FilterAttributionReport, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "filter_attribution_latest.json"))
	if err != nil {
		return FilterAttributionReport{}, false
	}
	var out FilterAttributionReport
	if err := json.Unmarshal(b, &out); err != nil {
		return FilterAttributionReport{}, false
	}
	return out, true
}

func loadTechnicalScorecardReportFile() (TechnicalScorecardReport, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "technical_scorecard_latest.json"))
	if err != nil {
		return TechnicalScorecardReport{}, false
	}
	var out TechnicalScorecardReport
	if err := json.Unmarshal(b, &out); err != nil {
		return TechnicalScorecardReport{}, false
	}
	return out, true
}

func loadCapitalPlanResearchReportFile() (CapitalPlanResearchReport, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "capital_plan_research_latest.json"))
	if err != nil {
		return CapitalPlanResearchReport{}, false
	}
	var out CapitalPlanResearchReport
	if err := json.Unmarshal(b, &out); err != nil {
		return CapitalPlanResearchReport{}, false
	}
	return out, true
}

func loadUniverseResearchReportFile() (agent2.UniverseResearchReport, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "coin_universe_research_latest.json"))
	if err != nil {
		return agent2.UniverseResearchReport{}, false
	}
	var out agent2.UniverseResearchReport
	if err := json.Unmarshal(b, &out); err != nil {
		return agent2.UniverseResearchReport{}, false
	}
	return out, true
}
func loadLivePositionReportFile() (liveguard.LiveLedgerReport, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "live_position_latest.json"))
	if err != nil {
		return liveguard.LiveLedgerReport{}, false
	}
	var out liveguard.LiveLedgerReport
	if err := json.Unmarshal(b, &out); err != nil {
		return liveguard.LiveLedgerReport{}, false
	}
	return out, true
}

func firstTechnicalScorecardCoins(items []TechnicalScorecardCoin, limit int) []TechnicalScorecardCoin {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func firstCapitalPlanCoins(items []CapitalPlanResearchCoin, limit int) []CapitalPlanResearchCoin {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func firstFilterAggregateRows(items []FilterAttributionAggregateRow, limit int) []FilterAttributionAggregateRow {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func firstFilterCoinRows(items []FilterAttributionCoinRow, limit int) []FilterAttributionCoinRow {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func loadTelegramCommandState() telegramCommandState {
	b, err := os.ReadFile(filepath.Join("reports", "telegram_command_state.json"))
	if err != nil {
		return telegramCommandState{}
	}
	var state telegramCommandState
	if err := json.Unmarshal(b, &state); err != nil {
		return telegramCommandState{}
	}
	return state
}

func saveTelegramCommandState(state telegramCommandState) error {
	return reportio.WriteJSON("reports", "telegram_command_state.json", state)
}

func telegramProtectionStatus(cfg config.Config) string {
	db, err := storage.Open(cfg.Storage.Path)
	if err != nil {
		return "Bảo vệ Hermes: chưa đọc được trạng thái."
	}
	defer db.Close()
	items, err := db.ProtectionStatuses()
	if err != nil {
		items = nil
	}
	var b strings.Builder
	b.WriteString("Bảo vệ Hermes đang hoạt động:\n")
	active := 0
	for _, p := range items {
		if !p.Active {
			continue
		}
		active++
		fmt.Fprintf(&b, "- %s", p.Name)
		if p.Symbol != "" {
			fmt.Fprintf(&b, " (%s)", p.Symbol)
		}
		if !p.UnlockAt.IsZero() {
			fmt.Fprintf(&b, " — mở lại lúc %s", p.UnlockAt.Local().Format("02/01 15:04"))
		}
		if p.Detail != "" {
			fmt.Fprintf(&b, " — %s", p.Detail)
		}
		b.WriteString("\n")
	}
	if active == 0 {
		b.WriteString("- Không có khóa tạm thời.\n")
	}
	if eq, e := db.EquityRiskState(); e == nil {
		fmt.Fprintf(&b, "- Tài sản hiện tại %.2f USDT | đỉnh %.2f | giảm từ đỉnh %.2f%%\n", eq.CurrentEquity, eq.HighWaterMark, eq.DrawdownPct*100)
	}
	b.WriteString("- " + db.ExecutionMarkoutSummary())
	return b.String()
}

func vietnameseBool(v bool) string {
	if v {
		return "có"
	}
	return "không"
}
