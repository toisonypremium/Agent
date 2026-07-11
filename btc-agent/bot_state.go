package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/market"
	"btc-agent/internal/reportio"
	"btc-agent/internal/storage"
)

const safetyLine = "spot limit BUY post-only only; no futures, no leverage, no market order"

type BotRuntimeSnapshot struct {
	GeneratedAt             time.Time                      `json:"generated_at"`
	Mode                    string                         `json:"mode"`
	DryRun                  bool                           `json:"dry_run"`
	SchedulerAlive          bool                           `json:"scheduler_alive"`
	SchedulerPID            int                            `json:"scheduler_pid,omitempty"`
	SchedulerStatus         string                         `json:"scheduler_status,omitempty"`
	SchedulerLastEvent      string                         `json:"scheduler_last_event,omitempty"`
	NextLiveSupervisorCycle string                         `json:"next_live_supervisor_cycle,omitempty"`
	SupervisorAlive         bool                           `json:"supervisor_alive"`
	SupervisorStatus        string                         `json:"supervisor_status,omitempty"`
	SupervisorAction        string                         `json:"supervisor_action,omitempty"`
	SupervisorSummary       string                         `json:"supervisor_summary,omitempty"`
	DoctorStatus            string                         `json:"doctor_status,omitempty"`
	DoctorSummary           string                         `json:"doctor_summary,omitempty"`
	PlanState               agent2.State                   `json:"plan_state,omitempty"`
	BTCPermission           agent1.Permission              `json:"btc_permission,omitempty"`
	AutoLiveAllowed         bool                           `json:"auto_live_allowed"`
	CanSubmitLiveOrder      bool                           `json:"can_submit_live_order"`
	OperatorHalt            bool                           `json:"operator_halt"`
	LiveEnabled             bool                           `json:"live_enabled"`
	AutoExecute             bool                           `json:"auto_execute"`
	RequireManualConfirm    bool                           `json:"require_manual_confirm"`
	ProofOnly               bool                           `json:"proof_only"`
	RealTradingEnabled      bool                           `json:"real_trading_enabled"`
	OrderManagementEnabled  bool                           `json:"order_management_enabled"`
	OpenLiveOrders          int                            `json:"open_live_orders"`
	LivePositions           int                            `json:"live_positions"`
	ManagedStatus           string                         `json:"managed_status,omitempty"`
	DesiredOrders           int                            `json:"desired_orders"`
	KeptOrders              int                            `json:"kept_orders"`
	PlacedOrders            int                            `json:"placed_orders"`
	CanceledOrders          int                            `json:"canceled_orders"`
	ReplacedOrders          int                            `json:"replaced_orders"`
	BlockedOrders           int                            `json:"blocked_orders"`
	DataHealthStatus        string                         `json:"data_health_status,omitempty"`
	DataHealthSummary       string                         `json:"data_health_summary,omitempty"`
	ReconcileSafetyStatus   string                         `json:"reconcile_safety_status,omitempty"`
	ReconcileSafetySummary  string                         `json:"reconcile_safety_summary,omitempty"`
	RiskGovernorStatus      string                         `json:"risk_governor_status,omitempty"`
	RiskGovernorSummary     string                         `json:"risk_governor_summary,omitempty"`
	RiskGovernorWarnings    []string                       `json:"risk_governor_warnings,omitempty"`
	PerCoin                 []liveguard.ManagedCoinSummary `json:"per_coin,omitempty"`
	Plan                    agent2.Plan                    `json:"-"`
	BTC                     BotBTCSnapshot                 `json:"btc,omitempty"`
	OpenOrders              []BotOpenOrderSnapshot         `json:"open_orders,omitempty"`
	Safety                  string                         `json:"safety"`
	Errors                  []string                       `json:"errors,omitempty"`
}

type BotBTCSnapshot struct {
	Price            float64     `json:"price,omitempty"`
	Regime           string      `json:"regime,omitempty"`
	TrendScore       float64     `json:"trend_score,omitempty"`
	WeeklyBias       string      `json:"weekly_bias,omitempty"`
	DailyBias        string      `json:"daily_bias,omitempty"`
	FourHourBias     string      `json:"four_hour_bias,omitempty"`
	FlowBias         string      `json:"flow_bias,omitempty"`
	FlowScore        float64     `json:"flow_score,omitempty"`
	PermissionReason string      `json:"permission_reason,omitempty"`
	RiskLevel        agent1.Risk `json:"risk_level,omitempty"`
	FallingKnifeRisk agent1.Risk `json:"falling_knife_risk,omitempty"`
	FomoRisk         agent1.Risk `json:"fomo_risk,omitempty"`
	SupportZone      market.Zone `json:"support_zone,omitempty"`
	ResistanceZone   market.Zone `json:"resistance_zone,omitempty"`
	AccumulationZone market.Zone `json:"accumulation_zone,omitempty"`
	InvalidationZone market.Zone `json:"invalidation_zone,omitempty"`
}

type BotOpenOrderSnapshot struct {
	Symbol        string  `json:"symbol"`
	InstID        string  `json:"inst_id"`
	Side          string  `json:"side,omitempty"`
	Type          string  `json:"type,omitempty"`
	Price         float64 `json:"price,omitempty"`
	Quantity      float64 `json:"quantity,omitempty"`
	Notional      float64 `json:"notional,omitempty"`
	Status        string  `json:"status,omitempty"`
	LayerIndex    int     `json:"layer_index,omitempty"`
	ClientOrderID string  `json:"client_order_id,omitempty"`
}

type ScenarioReport struct {
	GeneratedAt    time.Time         `json:"generated_at"`
	Conclusion     string            `json:"conclusion"`
	BotAction      string            `json:"bot_action"`
	PlanState      agent2.State      `json:"plan_state,omitempty"`
	BTCPermission  agent1.Permission `json:"btc_permission,omitempty"`
	CanSubmitOrder bool              `json:"can_submit_order"`
	BTC            BTCScenario       `json:"btc"`
	Coins          []CoinScenario    `json:"coins"`
	NearTriggers   []string          `json:"near_triggers,omitempty"`
	Blockers       []string          `json:"blockers,omitempty"`
	Safety         string            `json:"safety"`
}

type BTCScenario struct {
	BaseCase               string            `json:"base_case"`
	BullUnlock             string            `json:"bull_unlock"`
	BearInvalidation       string            `json:"bear_invalidation"`
	CurrentPermission      agent1.Permission `json:"current_permission,omitempty"`
	PermissionReason       string            `json:"permission_reason,omitempty"`
	UnlockConditions       []string          `json:"unlock_conditions,omitempty"`
	InvalidationConditions []string          `json:"invalidation_conditions,omitempty"`
	KeyZones               ScenarioZones     `json:"key_zones,omitempty"`
	RiskLevel              agent1.Risk       `json:"risk_level,omitempty"`
}

type ScenarioZones struct {
	Support      market.Zone `json:"support,omitempty"`
	Resistance   market.Zone `json:"resistance,omitempty"`
	Accumulation market.Zone `json:"accumulation,omitempty"`
	Invalidation market.Zone `json:"invalidation,omitempty"`
}

type CoinScenario struct {
	Symbol                 string       `json:"symbol"`
	State                  agent2.State `json:"state"`
	ReadinessScore         float64      `json:"readiness_score,omitempty"`
	RotationRank           int          `json:"rotation_rank,omitempty"`
	RotationScore          float64      `json:"rotation_score,omitempty"`
	MMCase                 string       `json:"mm_case,omitempty"`
	MMScore                float64      `json:"mm_score,omitempty"`
	LiquidityGrade         string       `json:"liquidity_grade,omitempty"`
	LiquidityScore         float64      `json:"liquidity_score,omitempty"`
	DiscountGapPct         float64      `json:"discount_gap_pct,omitempty"`
	RewardRisk             float64      `json:"reward_risk,omitempty"`
	DesiredLayers          int          `json:"desired_layers"`
	OpenOrders             int          `json:"open_orders"`
	WhyNoOrder             []string     `json:"why_no_order,omitempty"`
	UnlockConditions       []string     `json:"unlock_conditions,omitempty"`
	InvalidationConditions []string     `json:"invalidation_conditions,omitempty"`
	NextTrigger            string       `json:"next_trigger,omitempty"`
	IfActiveLimitAction    string       `json:"if_active_limit_action"`
	NearTrigger            bool         `json:"near_trigger,omitempty"`
}

func writeBotStateAndScenario(cfg config.Config, db *storage.DB, supervisor liveguard.SupervisorResult) (BotRuntimeSnapshot, ScenarioReport, error) {
	snapshot, err := buildBotRuntimeSnapshot(cfg, db, supervisor)
	if err != nil {
		return snapshot, ScenarioReport{}, err
	}
	scenario := buildScenarioReport(cfg, snapshot)
	if err := reportio.WriteJSON("reports", "bot_state_latest.json", snapshot); err != nil {
		return snapshot, scenario, err
	}
	if err := reportio.WriteJSON("reports", "scenario_latest.json", scenario); err != nil {
		return snapshot, scenario, err
	}
	if err := writeFilterAttributionReport(snapshot); err != nil {
		return snapshot, scenario, err
	}
	return snapshot, scenario, nil
}

func buildBotRuntimeSnapshot(cfg config.Config, db *storage.DB, supervisor liveguard.SupervisorResult) (BotRuntimeSnapshot, error) {
	now := time.Now()
	heartbeat, heartbeatOK := loadSchedulerHeartbeatReport()
	s := BotRuntimeSnapshot{GeneratedAt: now, Mode: os.Getenv("BTC_AGENT_MODE"), LiveEnabled: cfg.Live.Enabled, AutoExecute: cfg.Live.AutoExecute, RequireManualConfirm: cfg.Live.RequireManualConfirm, ProofOnly: cfg.Live.ProofOnly, RealTradingEnabled: cfg.Execution.RealTradingEnabled, OrderManagementEnabled: cfg.Live.OrderManagementEnabled, Safety: safetyLine}
	if heartbeatOK {
		s.SchedulerAlive = heartbeat.Status == "running" || heartbeat.Status == "starting"
		s.SchedulerPID = heartbeat.PID
		s.SchedulerStatus = heartbeat.Status
		s.SchedulerLastEvent = heartbeat.LastEvent
		s.NextLiveSupervisorCycle = heartbeat.NextLiveSupervisorCycle
		s.DryRun = heartbeat.DryRun
		if heartbeat.Mode != "" {
			s.Mode = heartbeat.Mode
		}
	}
	s.SupervisorAlive = !supervisor.GeneratedAt.IsZero()
	s.SupervisorStatus = supervisor.Status
	s.SupervisorAction = supervisor.Action
	s.SupervisorSummary = supervisor.Summary
	if supervisor.Doctor != nil {
		s.DoctorStatus = string(supervisor.Doctor.Status)
		s.DoctorSummary = supervisor.Doctor.Summary
	}
	if supervisor.Managed != nil {
		applyManagedToSnapshot(&s, *supervisor.Managed)
	} else if managed, ok := loadLatestManagedCycleReport(); ok {
		applyManagedToSnapshot(&s, managed)
	}
	halted, err := db.IsHalted()
	if err != nil {
		s.Errors = append(s.Errors, "operator halt: "+err.Error())
	} else {
		s.OperatorHalt = halted
	}
	open, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		s.Errors = append(s.Errors, "open live orders: "+err.Error())
	} else {
		s.OpenLiveOrders = len(open)
		s.OpenOrders = summarizeOpenOrders(open)
	}
	positions, err := db.LivePositions()
	if err != nil {
		s.Errors = append(s.Errors, "live positions: "+err.Error())
	} else {
		s.LivePositions = len(positions)
	}
	analysis, err := db.LatestAnalysis()
	if err != nil {
		s.Errors = append(s.Errors, "latest analysis: "+err.Error())
	} else {
		s.BTCPermission = analysis.ActionPermission
		s.BTC = BotBTCSnapshot{Price: analysis.BTCPrice, Regime: analysis.MarketRegime, TrendScore: analysis.TrendScore, WeeklyBias: analysis.WeeklyBias, DailyBias: analysis.DailyBias, FourHourBias: analysis.FourHourBias, FlowBias: string(analysis.Flow.Bias), FlowScore: analysis.Flow.Score, PermissionReason: analysis.PermissionReason, RiskLevel: analysis.RiskLevel, FallingKnifeRisk: analysis.FallingKnifeRisk, FomoRisk: analysis.FomoRisk, SupportZone: analysis.PrimarySupportZone, ResistanceZone: analysis.ResistanceZone, AccumulationZone: analysis.AccumulationZone, InvalidationZone: analysis.InvalidationZone}
	}
	plan, err := db.LatestPlan()
	if err != nil {
		s.Errors = append(s.Errors, "latest plan: "+err.Error())
	} else {
		s.Plan = plan
		s.PlanState = plan.State
		if s.BTCPermission == "" {
			s.BTCPermission = plan.ActionPermission
		}
		if len(s.PerCoin) == 0 {
			s.PerCoin = liveguard.BuildManagedCoinSummaries(cfg, plan, open, liveguard.ManagedCycleResult{PlanState: plan.State, Desired: []liveguard.ManagedDesiredOrder{}})
		}
	}
	s.AutoLiveAllowed = os.Getenv("BTC_AGENT_ALLOW_AUTO_LIVE") == "true"
	s.CanSubmitLiveOrder = canSubmitLiveOrderFromSnapshot(s)
	return s, nil
}

func canSubmitLiveOrderFromSnapshot(s BotRuntimeSnapshot) bool {
	runtimeCanSubmit := s.Mode == "live-auto" && !s.DryRun && s.AutoLiveAllowed && s.LiveEnabled && s.AutoExecute && !s.RequireManualConfirm && !s.ProofOnly && s.RealTradingEnabled && !s.OperatorHalt && s.DoctorStatus != string(liveguard.DoctorBlock)
	return runtimeCanSubmit && s.PlanState == agent2.StateActiveLimit && s.DesiredOrders > 0
}

func applyManagedToSnapshot(s *BotRuntimeSnapshot, managed liveguard.ManagedCycleResult) {
	s.ManagedStatus = managed.Status
	s.PlanState = managed.PlanState
	s.DesiredOrders = len(managed.Desired)
	s.KeptOrders = len(managed.Kept)
	s.PlacedOrders = len(managed.Placed)
	s.CanceledOrders = len(managed.Canceled)
	s.ReplacedOrders = len(managed.Replaced)
	s.BlockedOrders = len(managed.Blocked)
	s.DataHealthStatus = managed.DataHealth.Status
	s.DataHealthSummary = managed.DataHealth.Summary
	s.ReconcileSafetyStatus = managed.ReconcileSafety.Status
	s.ReconcileSafetySummary = managed.ReconcileSafety.Summary
	s.RiskGovernorStatus = managed.RiskGovernor.Status
	s.RiskGovernorSummary = managed.RiskGovernor.Summary
	s.RiskGovernorWarnings = managed.RiskGovernor.Warnings
	s.PerCoin = managed.PerCoin
	s.DryRun = managed.DryRun
}

func loadSchedulerHeartbeatReport() (SchedulerHeartbeat, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "scheduler_heartbeat_latest.json"))
	if err != nil {
		return SchedulerHeartbeat{}, false
	}
	var h SchedulerHeartbeat
	if err := json.Unmarshal(b, &h); err != nil {
		return SchedulerHeartbeat{}, false
	}
	return h, true
}

func summarizeOpenOrders(open []live.OrderStatus) []BotOpenOrderSnapshot {
	out := []BotOpenOrderSnapshot{}
	for _, o := range open {
		symbol := strings.ToUpper(o.Symbol)
		if symbol == "" {
			symbol = live.InternalSymbol(o.InstID)
		}
		notional := o.Notional
		if notional <= 0 && o.Price > 0 && o.Quantity > 0 {
			notional = o.Price * o.Quantity
		}
		out = append(out, BotOpenOrderSnapshot{Symbol: symbol, InstID: o.InstID, Side: o.Side, Type: o.OrderType, Price: o.Price, Quantity: o.Quantity, Notional: notional, Status: o.Status, LayerIndex: o.LayerIndex, ClientOrderID: o.ClientOrderID})
	}
	return out
}

func buildScenarioReport(cfg config.Config, s BotRuntimeSnapshot) ScenarioReport {
	report := ScenarioReport{GeneratedAt: s.GeneratedAt, PlanState: s.PlanState, BTCPermission: s.BTCPermission, CanSubmitOrder: s.CanSubmitLiveOrder, Safety: safetyLine}
	report.BTC = buildBTCScenario(s)
	report.Coins = buildCoinScenarios(cfg, s)
	report.Blockers = scenarioBlockers(s)
	report.NearTriggers = nearTriggerMessages(report.Coins)
	report.Conclusion, report.BotAction = scenarioConclusion(s, report)
	return report
}

func buildBTCScenario(s BotRuntimeSnapshot) BTCScenario {
	btc := BTCScenario{CurrentPermission: s.BTCPermission, PermissionReason: s.BTC.PermissionReason, RiskLevel: s.BTC.RiskLevel, KeyZones: ScenarioZones{Support: s.BTC.SupportZone, Resistance: s.BTC.ResistanceZone, Accumulation: s.BTC.AccumulationZone, Invalidation: s.BTC.InvalidationZone}}
	btc.BaseCase = fmt.Sprintf("BTC permission %s, plan %s. Bot chỉ đặt lệnh khi plan ACTIVE_LIMIT và safety gates pass.", emptyStringDefault(string(s.BTCPermission), "UNKNOWN"), emptyStringDefault(string(s.PlanState), "UNKNOWN"))
	btc.BullUnlock = "BTC chuyển ALLOWED, flow/reclaim xác nhận, asset có ACTIVE_LIMIT layer hợp lệ. Bot tự đặt spot limit BUY post-only theo cap."
	btc.BearInvalidation = "BTC mất support/invalidation hoặc data/reconcile/risk chuyển BLOCK. Bot đứng ngoài hoặc reconcile-only fail-closed."
	if s.BTCPermission != agent1.Allowed {
		btc.UnlockConditions = append(btc.UnlockConditions, "BTC permission chuyển ALLOWED")
	}
	if s.BTC.FlowBias == "" || strings.Contains(strings.ToLower(s.BTC.FlowBias), "neutral") {
		btc.UnlockConditions = append(btc.UnlockConditions, "BTC flow/reclaim rõ hơn")
	}
	if s.BTC.PermissionReason != "" {
		btc.UnlockConditions = append(btc.UnlockConditions, s.BTC.PermissionReason)
	}
	if s.BTC.InvalidationZone.Valid() {
		btc.InvalidationConditions = append(btc.InvalidationConditions, fmt.Sprintf("BTC mất vùng invalidation %.2f-%.2f", s.BTC.InvalidationZone.Low, s.BTC.InvalidationZone.High))
	}
	if s.BTC.RiskLevel == agent1.High || s.BTC.FallingKnifeRisk == agent1.High || s.BTC.FomoRisk == agent1.High {
		btc.InvalidationConditions = append(btc.InvalidationConditions, "BTC risk/falling knife/FOMO ở mức HIGH")
	}
	return btc
}

func buildCoinScenarios(cfg config.Config, s BotRuntimeSnapshot) []CoinScenario {
	out := []CoinScenario{}
	for _, coin := range s.PerCoin {
		cs := CoinScenario{Symbol: coin.Symbol, State: coin.State, ReadinessScore: coin.ReadinessScore, DesiredLayers: coin.DesiredLayers, OpenOrders: coin.OpenOrders, WhyNoOrder: firstStrings(coin.WhyNoOrder, 6), NextTrigger: coin.NextTrigger, IfActiveLimitAction: "Nếu chuyển ACTIVE_LIMIT và preflight/caps pass, bot tự đặt spot limit BUY post-only."}
		if asset, ok := findPlanAsset(s.Plan, coin.Symbol); ok {
			cs.RotationRank = asset.RotationRank
			cs.RotationScore = asset.RotationScore
			cs.MMCase = string(asset.MMCase)
			cs.MMScore = asset.MMScore
			cs.LiquidityGrade = asset.LiquidityQuality.Grade
			cs.LiquidityScore = asset.LiquidityQuality.Score
			cs.DiscountGapPct = asset.DiscountGapPct
			cs.RewardRisk = asset.RewardRisk
		}
		if cs.State != agent2.StateActiveLimit {
			cs.UnlockConditions = append(cs.UnlockConditions, "coin chuyển ACTIVE_LIMIT")
		}
		if s.BTCPermission != agent1.Allowed {
			cs.UnlockConditions = append(cs.UnlockConditions, "BTC permission ALLOWED")
		}
		if cs.DesiredLayers == 0 {
			cs.UnlockConditions = append(cs.UnlockConditions, "có layer hợp lệ đạt reward/risk và discount zone")
		}
		if coin.NextTrigger != "" {
			cs.UnlockConditions = append(cs.UnlockConditions, coin.NextTrigger)
		}
		cs.UnlockConditions = uniqueStringsMain(cs.UnlockConditions)
		cs.InvalidationConditions = []string{"BTC/data/reconcile/risk governor chuyển BLOCK", "coin mất support/invalidation theo plan"}
		cs.NearTrigger = coin.ReadinessScore >= nearTriggerReadinessThreshold() || (coin.DesiredLayers > 0 && cs.State == agent2.StateArmed)
		out = append(out, cs)
	}
	return out
}

func scenarioBlockers(s BotRuntimeSnapshot) []string {
	blockers := []string{}
	if !s.SchedulerAlive {
		blockers = append(blockers, "scheduler không active")
	}
	if s.DryRun {
		blockers = append(blockers, "scheduler đang dry-run")
	}
	if s.Mode != "live-auto" {
		blockers = append(blockers, "mode chưa phải live-auto")
	}
	if !s.AutoLiveAllowed {
		blockers = append(blockers, "BTC_AGENT_ALLOW_AUTO_LIVE chưa true")
	}
	if s.OperatorHalt {
		blockers = append(blockers, "operator halt active")
	}
	if s.DoctorStatus == string(liveguard.DoctorBlock) {
		blockers = append(blockers, "live doctor block")
	}
	if s.PlanState != agent2.StateActiveLimit {
		blockers = append(blockers, "plan chưa ACTIVE_LIMIT")
	}
	if s.DesiredOrders == 0 {
		blockers = append(blockers, "desired orders = 0")
	}
	return uniqueStringsMain(blockers)
}

func scenarioConclusion(s BotRuntimeSnapshot, report ScenarioReport) (string, string) {
	if s.PlacedOrders > 0 || s.CanceledOrders > 0 || s.ReplacedOrders > 0 {
		return fmt.Sprintf("Bot live-auto đã hành động: placed=%d canceled=%d replaced=%d.", s.PlacedOrders, s.CanceledOrders, s.ReplacedOrders), "Quản lý lệnh thật theo managed cycle."
	}
	if s.PlanState == agent2.StateActiveLimit && s.DesiredOrders > 0 && s.CanSubmitLiveOrder {
		return "Plan ACTIVE_LIMIT và có desired order; bot được phép tự đặt lệnh thật theo cap.", "Đặt/giữ/cancel/replace spot limit BUY post-only theo managed cycle."
	}
	if len(report.NearTriggers) > 0 {
		return "Bot live-auto đang chạy, có coin gần điều kiện nhưng chưa đủ ACTIVE_LIMIT để đặt lệnh.", "Đứng ngoài, theo dõi near-trigger, không cần user xác nhận."
	}
	return fmt.Sprintf("Bot live-auto đang chạy, chưa đặt lệnh vì plan %s và desired=%d.", emptyStringDefault(string(s.PlanState), "UNKNOWN"), s.DesiredOrders), "Đứng ngoài fail-closed; tự hành động khi ACTIVE_LIMIT và gates pass."
}

func nearTriggerMessages(coins []CoinScenario) []string {
	out := []string{}
	for _, coin := range coins {
		if coin.NearTrigger {
			out = append(out, fmt.Sprintf("%s gần điều kiện: state=%s readiness=%.0f%% next=%s", coin.Symbol, coin.State, coin.ReadinessScore*100, emptyStringDefault(coin.NextTrigger, "chờ trigger rõ hơn")))
		}
	}
	return out
}

func findPlanAsset(plan agent2.Plan, symbol string) (agent2.AssetPlan, bool) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	for _, asset := range plan.Assets {
		if strings.ToUpper(strings.TrimSpace(asset.Symbol)) == symbol {
			return asset, true
		}
	}
	return agent2.AssetPlan{}, false
}

func scenarioMarkdown(report ScenarioReport) string {
	var b strings.Builder
	b.WriteString("SCENARIO\n\n")
	b.WriteString("Conclusion: " + report.Conclusion + "\n")
	b.WriteString("Bot action: " + report.BotAction + "\n")
	b.WriteString(fmt.Sprintf("Can submit order now: %v\n", report.CanSubmitOrder))
	b.WriteString("BTC base: " + report.BTC.BaseCase + "\n")
	if len(report.BTC.UnlockConditions) > 0 {
		b.WriteString("BTC unlock:\n")
		for _, item := range firstStrings(report.BTC.UnlockConditions, 4) {
			b.WriteString("- " + item + "\n")
		}
	}
	if len(report.Coins) > 0 {
		b.WriteString("\nCoins:\n")
		for _, coin := range report.Coins {
			b.WriteString(fmt.Sprintf("- %s: state=%s readiness=%.0f%% desired=%d open=%d RR=%.2f MM=%s %.0f Liq=%s %.0f\n", coin.Symbol, coin.State, coin.ReadinessScore*100, coin.DesiredLayers, coin.OpenOrders, coin.RewardRisk, emptyStringDefault(coin.MMCase, "n/a"), coin.MMScore, emptyStringDefault(coin.LiquidityGrade, "n/a"), coin.LiquidityScore))
			if len(coin.WhyNoOrder) > 0 {
				b.WriteString("  why=" + strings.Join(firstStrings(coin.WhyNoOrder, 3), "; ") + "\n")
			}
			if coin.NextTrigger != "" {
				b.WriteString("  next=" + coin.NextTrigger + "\n")
			}
		}
	}
	if len(report.NearTriggers) > 0 {
		b.WriteString("\nNear triggers:\n")
		for _, item := range firstStrings(report.NearTriggers, 5) {
			b.WriteString("- " + item + "\n")
		}
	}
	if len(report.Blockers) > 0 {
		b.WriteString("\nBlockers:\n")
		for _, item := range firstStrings(report.Blockers, 6) {
			b.WriteString("- " + item + "\n")
		}
	}
	b.WriteString("Safety: " + report.Safety + "\n")
	return b.String()
}

func liveSupervisorScenarioTelegram(report ScenarioReport, result liveguard.SupervisorResult) string {
	var b strings.Builder
	b.WriteString("📊 BTC Agent — Quản lý bot live-auto\n")
	b.WriteString("I. KẾT LUẬN\n")
	b.WriteString(report.Conclusion + "\n")
	b.WriteString(fmt.Sprintf("Giám sát: %s | chu kỳ quản lý lệnh thật\n", result.Status))
	if result.Managed != nil {
		m := result.Managed
		b.WriteString(fmt.Sprintf("Lệnh: mong muốn=%d đặt mới=%d hủy=%d thay=%d bị chặn=%d\n", len(m.Desired), len(m.Placed), len(m.Canceled), len(m.Replaced), len(m.Blocked)))
	}
	b.WriteString("\nII. BTC & KỊCH BẢN\n")
	b.WriteString(report.BTC.BaseCase + "\n")
	b.WriteString("Mở khóa: " + strings.Join(firstStrings(report.BTC.UnlockConditions, 3), "; ") + "\n")
	if len(report.BTC.InvalidationConditions) > 0 {
		b.WriteString("Vô hiệu: " + strings.Join(firstStrings(report.BTC.InvalidationConditions, 2), "; ") + "\n")
	}
	b.WriteString("\nIII. COIN & ĐIỀU KIỆN TIẾP THEO\n")
	for _, coin := range firstCoinScenarios(report.Coins, 3) {
		why := "đang chờ ACTIVE_LIMIT/layer hợp lệ"
		if len(coin.WhyNoOrder) > 0 {
			why = strings.Join(firstStrings(coin.WhyNoOrder, 2), "; ")
		}
		b.WriteString(fmt.Sprintf("- %s %s %.0f%% | MM=%s %.0f | Liq=%s %.0f | RR %.2f | thiếu: %s | điều kiện kích hoạt=%s\n", coin.Symbol, coin.State, coin.ReadinessScore*100, emptyStringDefault(coin.MMCase, "n/a"), coin.MMScore, emptyStringDefault(coin.LiquidityGrade, "n/a"), coin.LiquidityScore, coin.RewardRisk, why, emptyStringDefault(coin.NextTrigger, "chờ tín hiệu rõ hơn")))
	}
	b.WriteString("\nIV. BOT & AN TOÀN\n")
	b.WriteString(report.BotAction + "\n")
	if len(report.Blockers) > 0 {
		b.WriteString("Lý do chặn: " + strings.Join(firstStrings(report.Blockers, 4), "; ") + "\n")
	}
	b.WriteString("An toàn: chỉ mua spot bằng limit post-only; không futures; không leverage; không market order.\n")
	return b.String()
}

func firstCoinScenarios(items []CoinScenario, limit int) []CoinScenario {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

type TelegramScenarioState struct {
	GeneratedAt    time.Time         `json:"generated_at"`
	PlanState      agent2.State      `json:"plan_state,omitempty"`
	BTCPermission  agent1.Permission `json:"btc_permission,omitempty"`
	NearTriggerKey string            `json:"near_trigger_key,omitempty"`
	CanSubmitOrder bool              `json:"can_submit_order"`
}

func shouldSendNearTriggerAlert(report ScenarioReport) bool {
	if len(report.NearTriggers) == 0 {
		return false
	}
	prev, ok := loadTelegramScenarioState()
	key := strings.Join(report.NearTriggers, "|")
	if !ok {
		return true
	}
	return prev.NearTriggerKey != key || prev.PlanState != report.PlanState || prev.BTCPermission != report.BTCPermission || prev.CanSubmitOrder != report.CanSubmitOrder
}

func saveTelegramScenarioState(report ScenarioReport) error {
	state := TelegramScenarioState{GeneratedAt: report.GeneratedAt, PlanState: report.PlanState, BTCPermission: report.BTCPermission, NearTriggerKey: strings.Join(report.NearTriggers, "|"), CanSubmitOrder: report.CanSubmitOrder}
	return reportio.WriteJSON("reports", "telegram_scenario_state_latest.json", state)
}

func loadTelegramScenarioState() (TelegramScenarioState, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "telegram_scenario_state_latest.json"))
	if err != nil {
		return TelegramScenarioState{}, false
	}
	var state TelegramScenarioState
	if err := json.Unmarshal(b, &state); err != nil {
		return TelegramScenarioState{}, false
	}
	return state, true
}

func nearTriggerTelegram(report ScenarioReport) string {
	var b strings.Builder
	b.WriteString("🟡 BTC Agent — Gần điều kiện đặt lệnh\n")
	b.WriteString(fmt.Sprintf("Plan: %s | BTC: %s | can_submit_now=%v\n", report.PlanState, report.BTCPermission, report.CanSubmitOrder))
	for _, item := range firstStrings(report.NearTriggers, 5) {
		b.WriteString("- " + item + "\n")
	}
	b.WriteString("Bot chưa bypass gate. Nếu ACTIVE_LIMIT và preflight/caps pass, bot tự đặt limit BUY post-only.\n")
	b.WriteString("An toàn: chỉ mua spot bằng limit post-only; không futures; không leverage; không market order.\n")
	return b.String()
}

func nearTriggerReadinessThreshold() float64 { return 0.75 }

func emptyStringDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
