package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/notify"
	"btc-agent/internal/reportio"
	"btc-agent/internal/storage"
	"btc-agent/internal/usertext"
)

type telegramCommandState struct {
	LastUpdateID int       `json:"last_update_id"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func runTelegramCommands(ctx context.Context, cfg config.Config, db *storage.DB) error {
	token := firstNonEmpty(cfg.Notify.TelegramToken, os.Getenv("TELEGRAM_TOKEN"))
	chatID := firstNonEmpty(cfg.Notify.TelegramChatID, os.Getenv("TELEGRAM_CHAT_ID"))
	if strings.TrimSpace(token) == "" || strings.TrimSpace(chatID) == "" {
		return fmt.Errorf("telegram command config missing token/chat_id")
	}
	state := loadTelegramCommandState()
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
			cmd := normalizeTelegramCommand(update.Message.Text)
			if cmd != "" {
				text, ok := buildReadOnlyTelegramCommandReply(cmd)
				if ok {
					if err := notify.Telegram(ctx, token, chatID, usertext.TelegramVietnamese(text)); err != nil {
						advance = false
						return err
					}
				}
			}
		}
		if advance {
			state.LastUpdateID = update.UpdateID
			state.UpdatedAt = time.Now()
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
	switch cmd {
	case "/status", "/why", "/coins", "/filters", "/scorecard", "/allocation", "/capital", "/universe", "/dashboard", "/trigger", "/orders", "/positions", "/doctor", "/supervisor", "/next", "/risk", "/hermes", "/exits", "/audit", "/help":
		return cmd
	default:
		return ""
	}
}

func buildReadOnlyTelegramCommandReply(cmd string) (string, bool) {
	snapshot, snapshotOK := loadBotRuntimeSnapshotReport()
	scenario, scenarioOK := loadScenarioReportFile()
	supervisor, supervisorOK := loadLatestSupervisorReportFile()
	switch cmd {
	case "/help":
		return telegramCommandsHelp(), true
	case "/status":
		if !snapshotOK || !scenarioOK {
			return "Chưa có bot_state/scenario report. Chờ live supervisor chạy một chu kỳ.", true
		}
		return telegramCommandStatus(snapshot, scenario), true
	case "/why":
		if !scenarioOK {
			return "Chưa có scenario report. Chờ live supervisor chạy một chu kỳ.", true
		}
		return telegramCommandWhy(scenario), true
	case "/coins":
		if !scenarioOK {
			return "Chưa có scenario report. Chờ live supervisor chạy một chu kỳ.", true
		}
		return telegramCommandCoins(scenario), true
	case "/filters":
		filterReport, ok := loadFilterAttributionReportFile()
		if !ok {
			return "Chưa có filter attribution report. Chờ live supervisor chạy một chu kỳ.", true
		}
		return telegramCommandFilters(filterReport), true
	case "/scorecard":
		report, ok := loadTechnicalScorecardReportFile()
		if !ok {
			return "Chưa có technical scorecard report. Chờ live supervisor chạy một chu kỳ.", true
		}
		return telegramCommandScorecard(report), true
	case "/allocation", "/capital":
		report, ok := loadCapitalPlanResearchReportFile()
		if !ok {
			return "Chưa có capital plan research report. Chờ live supervisor chạy một chu kỳ.", true
		}
		return telegramCommandAllocation(report), true
	case "/universe":
		report, ok := loadUniverseResearchReportFile()
		if !ok {
			return "Chưa có universe research report. Chạy universe-research hoặc chờ report được tạo.", true
		}
		return telegramCommandUniverse(report), true
	case "/dashboard":
		report, ok := loadDecisionDashboardReportFile()
		if !ok {
			return "Chưa có decision dashboard report. Chờ live supervisor chạy một chu kỳ.", true
		}
		return telegramCommandDashboard(report), true
	case "/trigger":
		report, ok := loadDecisionDashboardReportFile()
		if !ok {
			return "Chưa có decision dashboard report. Chờ live supervisor chạy một chu kỳ.", true
		}
		return telegramCommandTrigger(report), true
	case "/orders":
		if !snapshotOK || !scenarioOK {
			return "Chưa có bot_state/scenario report. Chờ live supervisor chạy một chu kỳ.", true
		}
		return telegramCommandOrders(snapshot, scenario), true
	case "/positions":
		positionReport, ok := loadLivePositionReportFile()
		if !ok {
			return "Chưa có live position report. Chạy reconcile/positions hoặc chờ supervisor cập nhật.", true
		}
		return telegramCommandPositions(positionReport), true
	case "/doctor":
		if !snapshotOK {
			return "Chưa có bot_state report. Chờ live supervisor chạy một chu kỳ.", true
		}
		return telegramCommandDoctor(snapshot), true
	case "/supervisor":
		if !snapshotOK {
			return "Chưa có bot_state report. Chờ live supervisor chạy một chu kỳ.", true
		}
		return telegramCommandSupervisor(snapshot, supervisor, supervisorOK), true
	case "/next":
		if !scenarioOK {
			return "Chưa có scenario report. Chờ live supervisor chạy một chu kỳ.", true
		}
		return telegramCommandNext(scenario), true
	case "/risk":
		if !snapshotOK || !scenarioOK {
			return "Chưa có bot_state/scenario report. Chờ live supervisor chạy một chu kỳ.", true
		}
		return telegramCommandRisk(snapshot, scenario), true
	case "/hermes":
		report, ok := loadHermesReportFile()
		if !ok {
			return "Chua co Hermes report. Cho Hermes cycle chay hoac dung: ./bin/btc-agent hermes-cycle --config config.yaml", true
		}
		return telegramCommandHermes(report), true
	case "/exits":
		snap := buildHermesSnapshotFromReports()
		return telegramCommandExits(snap), true
	case "/audit":
		return telegramCommandAudit(), true
	default:
		return "", false
	}
}

func telegramCommandsHelp() string {
	return strings.TrimSpace(`BTC Agent — lệnh Telegram read-only
/status — trạng thái bot
/why — vì sao chưa đặt lệnh
/coins — từng coin
/filters — bộ lọc đang chặn gì
/scorecard — bảng điểm kỹ thuật
/allocation — phân bổ vốn nghiên cứu
/capital — tóm tắt vốn nghiên cứu
/universe — universe research coin
/dashboard — bảng điều khiển quyết định
/trigger — trigger tiếp theo
/orders — lệnh đang mở và desired
/positions — vị thế live đang ghi nhận
/doctor — live doctor
/supervisor — live supervisor
/next — điều kiện kích hoạt tiếp theo
/risk — risk governor và caps
/hermes — Hermes AI analysis tổng hợp
/exits — exit signals hiện tại
/audit — live-auto-audit verdict

Không có lệnh đặt mua/bán qua Telegram. Không bypass ACTIVE_LIMIT.`) + "\n"
}

func telegramCommandStatus(s BotRuntimeSnapshot, r ScenarioReport) string {
	return fmt.Sprintf("BTC Agent — Status\nMode: %s | dry_run=%v | scheduler=%v | supervisor=%s\nPlan: %s | BTC: %s | can_submit_now=%v\nDoctor: %s\nKết luận: %s\nAn toàn: chỉ mua spot bằng limit post-only; không futures; không leverage; không market order.\n", s.Mode, s.DryRun, s.SchedulerAlive, emptyStringDefault(s.SupervisorStatus, "unknown"), s.PlanState, s.BTCPermission, r.CanSubmitOrder, emptyStringDefault(s.DoctorStatus, "unknown"), r.Conclusion)
}

func telegramCommandWhy(r ScenarioReport) string {
	var b strings.Builder
	b.WriteString("BTC Agent — Vì sao chưa đặt lệnh\n")
	if len(r.Blockers) > 0 {
		b.WriteString("Blockers chính:\n")
		for _, item := range firstStrings(r.Blockers, 6) {
			b.WriteString("- " + item + "\n")
		}
	}
	for _, coin := range firstCoinScenarios(r.Coins, 3) {
		b.WriteString(fmt.Sprintf("\n%s %s:\n", coin.Symbol, coin.State))
		for _, reason := range firstStrings(coin.WhyNoOrder, 4) {
			b.WriteString("- " + reason + "\n")
		}
		if coin.NextTrigger != "" {
			b.WriteString("Next: " + coin.NextTrigger + "\n")
		}
	}
	return b.String()
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

func telegramCommandDashboard(r DecisionDashboardReport) string {
	var b strings.Builder
	b.WriteString("BTC Agent — Dashboard\n")
	b.WriteString(fmt.Sprintf("Bot ready=%v | Market ready=%v | can_submit=%v\n", r.BotReady, r.MarketReady, r.CanSubmitNow))
	b.WriteString(fmt.Sprintf("Plan=%s | BTC=%s\n", r.PlanState, r.BTCPermission))
	b.WriteString(fmt.Sprintf("Best production=%s | universe=%s\n", emptyStringDefault(r.BestProductionCoin, "n/a"), emptyStringDefault(r.BestUniverseCoin, "n/a")))
	b.WriteString("Next: " + emptyStringDefault(r.NextTrigger, "n/a") + "\n")
	for _, blocker := range firstStrings(r.Blockers, 4) {
		b.WriteString("- " + blocker + "\n")
	}
	b.WriteString("Read-only: dashboard không đặt lệnh, không bypass ACTIVE_LIMIT.\n")
	return b.String()
}

func telegramCommandTrigger(r DecisionDashboardReport) string {
	var b strings.Builder
	b.WriteString("BTC Agent — Trigger tiếp theo\n")
	b.WriteString("Next: " + emptyStringDefault(r.NextTrigger, "n/a") + "\n")
	if len(r.Blockers) > 0 {
		b.WriteString("Blockers:\n")
		for _, blocker := range firstStrings(r.Blockers, 5) {
			b.WriteString("- " + blocker + "\n")
		}
	}
	if len(r.Actions) > 0 {
		b.WriteString("Actions:\n")
		for _, action := range firstStrings(r.Actions, 4) {
			b.WriteString("- " + action + "\n")
		}
	}
	b.WriteString("Read-only: trigger không đặt lệnh, không bypass ACTIVE_LIMIT.\n")
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
	b.WriteString("Read-only: Telegram không đóng/mở vị thế.\n")
	return b.String()
}

func telegramCommandDoctor(s BotRuntimeSnapshot) string {
	return fmt.Sprintf("BTC Agent — Doctor\nStatus: %s\nSummary: %s\nData: %s\nReconcile: %s\nRisk: %s\n", emptyStringDefault(s.DoctorStatus, "unknown"), emptyStringDefault(s.DoctorSummary, "none"), emptyStringDefault(s.DataHealthSummary, "unknown"), emptyStringDefault(s.ReconcileSafetySummary, "unknown"), emptyStringDefault(s.RiskGovernorSummary, "unknown"))
}

func telegramCommandSupervisor(s BotRuntimeSnapshot, supervisor liveguard.SupervisorResult, ok bool) string {
	var b strings.Builder
	b.WriteString("BTC Agent — Supervisor\n")
	b.WriteString(fmt.Sprintf("Status: %s | action=%s | alive=%v\n", emptyStringDefault(s.SupervisorStatus, "unknown"), emptyStringDefault(s.SupervisorAction, "unknown"), s.SupervisorAlive))
	b.WriteString("Summary: " + emptyStringDefault(s.SupervisorSummary, "none") + "\n")
	if ok && supervisor.Managed != nil {
		m := supervisor.Managed
		b.WriteString(fmt.Sprintf("Managed: %s desired=%d placed=%d canceled=%d replaced=%d blocked=%d\n", m.Status, len(m.Desired), len(m.Placed), len(m.Canceled), len(m.Replaced), len(m.Blocked)))
	}
	b.WriteString("Next supervisor: " + emptyStringDefault(s.NextLiveSupervisorCycle, "unknown") + "\n")
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

func telegramCommandRisk(s BotRuntimeSnapshot, r ScenarioReport) string {
	var b strings.Builder
	b.WriteString("BTC Agent — Risk\n")
	b.WriteString(fmt.Sprintf("BTC risk=%s | falling=%s | fomo=%s\n", s.BTC.RiskLevel, s.BTC.FallingKnifeRisk, s.BTC.FomoRisk))
	b.WriteString("Risk governor: " + emptyStringDefault(s.RiskGovernorSummary, "unknown") + "\n")
	if len(s.RiskGovernorWarnings) > 0 {
		b.WriteString("Warnings:\n")
		for _, item := range firstStrings(s.RiskGovernorWarnings, 4) {
			b.WriteString("- " + item + "\n")
		}
	}
	if len(r.Blockers) > 0 {
		b.WriteString("Blockers:\n")
		for _, item := range firstStrings(r.Blockers, 6) {
			b.WriteString("- " + item + "\n")
		}
	}
	b.WriteString("An toàn: chỉ mua spot bằng limit post-only; không futures; không leverage; không market order.\n")
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

func loadDecisionDashboardReportFile() (DecisionDashboardReport, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "decision_dashboard_latest.json"))
	if err != nil {
		return DecisionDashboardReport{}, false
	}
	var out DecisionDashboardReport
	if err := json.Unmarshal(b, &out); err != nil {
		return DecisionDashboardReport{}, false
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
