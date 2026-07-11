package opsplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
)

const (
	UrgencyRiskAlert = "RISK_ALERT"
	UrgencyAction    = "ACTION"
	UrgencyHigh      = "HIGH"
	UrgencyElevated  = "ELEVATED"
	UrgencyNormal    = "NORMAL"
)

type Report struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Mode        string         `json:"mode"`
	Market      MarketPlan     `json:"market"`
	Capital     CapitalPlan    `json:"capital"`
	Monitoring  MonitoringPlan `json:"monitoring"`
	Runtime     RuntimePlan    `json:"runtime"`
	Telegram    TelegramPlan   `json:"telegram"`
	Fingerprint string         `json:"fingerprint"`
	Summary     string         `json:"summary"`
}

type MarketPlan struct {
	BTCPrice            float64           `json:"btc_price"`
	Regime              string            `json:"regime"`
	Permission          agent1.Permission `json:"permission"`
	PlanState           agent2.State      `json:"plan_state"`
	Risk                agent1.Risk       `json:"risk"`
	FallingKnifeRisk    agent1.Risk       `json:"falling_knife_risk"`
	FOMORisk            agent1.Risk       `json:"fomo_risk"`
	TrendScore          float64           `json:"trend_score"`
	FlowBias            string            `json:"flow_bias"`
	FlowScore           float64           `json:"flow_score"`
	AccumulationPhase   string            `json:"accumulation_phase,omitempty"`
	AccumulationScore   float64           `json:"accumulation_score,omitempty"`
	AccumulationTrigger string            `json:"accumulation_trigger,omitempty"`
	PrimarySupportLow   float64           `json:"primary_support_low,omitempty"`
	PrimarySupportHigh  float64           `json:"primary_support_high,omitempty"`
	InvalidationLow     float64           `json:"invalidation_low,omitempty"`
	ResistanceLow       float64           `json:"resistance_low,omitempty"`
	Urgency             string            `json:"urgency"`
	CriticalReasons     []string          `json:"critical_reasons,omitempty"`
	MainScenario        string            `json:"main_scenario"`
	UnlockScenario      string            `json:"unlock_scenario"`
	InvalidScenario     string            `json:"invalid_scenario"`
}

type ExposureSnapshot struct {
	PositionCostUSDT      float64                  `json:"position_cost_usdt"`
	OpenOrderNotionalUSDT float64                  `json:"open_order_notional_usdt"`
	Assets                map[string]AssetExposure `json:"assets,omitempty"`
	Source                string                   `json:"source,omitempty"`
}

type AssetExposure struct {
	PositionCostUSDT      float64 `json:"position_cost_usdt"`
	OpenOrderNotionalUSDT float64 `json:"open_order_notional_usdt"`
}

type CapitalPlan struct {
	TotalCapitalUSDT           float64            `json:"total_capital_usdt"`
	ReserveCashUSDT            float64            `json:"reserve_cash_usdt"`
	InvestableCapitalUSDT      float64            `json:"investable_capital_usdt"`
	CycleDeploymentCapUSDT     float64            `json:"cycle_deployment_cap_usdt"`
	ExistingPositionUSDT       float64            `json:"existing_position_usdt"`
	OpenOrderNotionalUSDT      float64            `json:"open_order_notional_usdt"`
	AlreadyCommittedUSDT       float64            `json:"already_committed_usdt"`
	AvailableCycleCapacityUSDT float64            `json:"available_cycle_capacity_usdt"`
	ExecutableNowUSDT          float64            `json:"executable_now_usdt"`
	OpportunityReservedUSDT    float64            `json:"opportunity_reserved_usdt"`
	UnusedCycleCapacityUSDT    float64            `json:"unused_cycle_capacity_usdt"`
	ExposureSource             string             `json:"exposure_source,omitempty"`
	Assets                     []AssetCapitalPlan `json:"assets"`
	Policy                     string             `json:"policy"`
}

type AssetCapitalPlan struct {
	Symbol                 string       `json:"symbol"`
	State                  agent2.State `json:"state"`
	Tier                   string       `json:"tier"`
	Readiness              float64      `json:"readiness"`
	TargetAllocationPct    float64      `json:"target_allocation_pct"`
	StrategicCapUSDT       float64      `json:"strategic_cap_usdt"`
	ExistingExposureUSDT   float64      `json:"existing_exposure_usdt"`
	RemainingStrategicUSDT float64      `json:"remaining_strategic_usdt"`
	ExecutableBudgetUSDT   float64      `json:"executable_budget_usdt"`
	OpportunityBudgetUSDT  float64      `json:"opportunity_budget_usdt"`
	LayerBudgetsUSDT       []float64    `json:"layer_budgets_usdt,omitempty"`
	Invalidation           float64      `json:"invalidation,omitempty"`
	NextTrigger            string       `json:"next_trigger,omitempty"`
	Reason                 string       `json:"reason"`
}

type MonitoringPlan struct {
	Enabled                  bool     `json:"enabled"`
	ConfiguredScanMinutes    int      `json:"configured_scan_minutes"`
	RecommendedScanMinutes   int      `json:"recommended_scan_minutes"`
	TelegramDigestMinutes    int      `json:"telegram_digest_minutes"`
	NotifyOnStateChange      bool     `json:"notify_on_state_change"`
	NotifyOnCritical         bool     `json:"notify_on_critical"`
	CriticalRepeatMinutes    int      `json:"critical_repeat_minutes"`
	MaxConsecutiveScanErrors int      `json:"max_consecutive_scan_errors"`
	Focus                    []string `json:"focus"`
}

type RuntimePlan struct {
	AppMode                   string `json:"app_mode"`
	LiveEnabled               bool   `json:"live_enabled"`
	RealTradingEnabled        bool   `json:"real_trading_enabled"`
	AutoExecute               bool   `json:"auto_execute"`
	SupervisorEnabled         bool   `json:"supervisor_enabled"`
	OrderManagementEnabled    bool   `json:"order_management_enabled"`
	ProofOnly                 bool   `json:"proof_only"`
	DailyRunTime              string `json:"daily_run_time"`
	ReconcileIntervalMinutes  int    `json:"reconcile_interval_minutes"`
	ManagementIntervalMinutes int    `json:"management_interval_minutes"`
	ExecutionAuthority        string `json:"execution_authority"`
	SafetyPolicy              string `json:"safety_policy"`
}

type TelegramPlan struct {
	ImmediateEvents []string `json:"immediate_events"`
	DigestContents  []string `json:"digest_contents"`
	NoisePolicy     string   `json:"noise_policy"`
}

func Build(cfg config.Config, analysis agent1.MarketAnalysis, plan agent2.Plan, snapshots ...ExposureSnapshot) Report {
	exposure := ExposureSnapshot{}
	if len(snapshots) > 0 {
		exposure = normalizeExposure(snapshots[0])
	}
	report := Report{
		GeneratedAt: time.Now(),
		Mode:        cfg.App.Mode,
		Market:      buildMarketPlan(analysis, plan),
		Capital:     buildCapitalPlan(cfg, analysis, plan, exposure),
		Monitoring:  buildMonitoringPlan(cfg, analysis, plan),
		Runtime:     buildRuntimePlan(cfg),
		Telegram:    buildTelegramPlan(),
	}
	report.Fingerprint = fingerprint(report)
	report.Summary = fmt.Sprintf("BTC %s | plan %s | urgency %s | committed %.2f | available %.2f | executable %.2f USDT | reserve %.2f USDT", report.Market.Permission, report.Market.PlanState, report.Market.Urgency, report.Capital.AlreadyCommittedUSDT, report.Capital.AvailableCycleCapacityUSDT, report.Capital.ExecutableNowUSDT, report.Capital.ReserveCashUSDT)
	return report
}

func buildMarketPlan(a agent1.MarketAnalysis, p agent2.Plan) MarketPlan {
	m := MarketPlan{
		BTCPrice:            finite(a.BTCPrice),
		Regime:              a.MarketRegime,
		Permission:          a.ActionPermission,
		PlanState:           p.State,
		Risk:                a.RiskLevel,
		FallingKnifeRisk:    a.FallingKnifeRisk,
		FOMORisk:            a.FomoRisk,
		TrendScore:          finite(a.TrendScore),
		FlowBias:            string(a.Flow.Bias),
		FlowScore:           finite(a.Flow.Score),
		AccumulationPhase:   string(a.BTCAccumulation.Phase),
		AccumulationScore:   finite(a.BTCAccumulation.Score),
		AccumulationTrigger: a.BTCAccumulation.NextTrigger,
		PrimarySupportLow:   finite(a.PrimarySupportZone.Low),
		PrimarySupportHigh:  finite(a.PrimarySupportZone.High),
		InvalidationLow:     finite(a.InvalidationZone.Low),
		ResistanceLow:       finite(a.ResistanceZone.Low),
		MainScenario:        a.ScenarioMain,
		UnlockScenario:      a.ScenarioBullish,
		InvalidScenario:     a.ScenarioBearish,
	}
	m.Urgency, m.CriticalReasons = marketUrgency(a, p)
	return m
}

func marketUrgency(a agent1.MarketAnalysis, p agent2.Plan) (string, []string) {
	reasons := []string{}
	if a.MarketRegime == "PANIC_SELLING" {
		reasons = append(reasons, "BTC PANIC_SELLING")
	}
	if a.FallingKnifeRisk == agent1.High {
		reasons = append(reasons, "falling-knife risk HIGH")
	}
	if a.FomoRisk == agent1.High && p.State == agent2.StateActiveLimit {
		reasons = append(reasons, "FOMO risk HIGH while plan ACTIVE_LIMIT")
	}
	if len(reasons) > 0 {
		return UrgencyRiskAlert, reasons
	}
	switch p.State {
	case agent2.StateActiveLimit:
		return UrgencyAction, nil
	case agent2.StateArmed:
		return UrgencyHigh, nil
	case agent2.StateScout:
		return UrgencyElevated, nil
	default:
		return UrgencyNormal, nil
	}
}

func buildCapitalPlan(cfg config.Config, analysis agent1.MarketAnalysis, plan agent2.Plan, exposure ExposureSnapshot) CapitalPlan {
	total := math.Max(0, cfg.Portfolio.TotalCapital)
	reserve := total * clamp01(cfg.Portfolio.ReserveCashRatio)
	investable := math.Max(0, total-reserve)
	cycleCap := investable * clamp01(cfg.Risk.MaxTotalDeploymentPerCycle)
	if cfg.Risk.MaxTotalDeploymentPerCycle <= 0 {
		cycleCap = 0
	}
	committed := math.Max(0, exposure.PositionCostUSDT) + math.Max(0, exposure.OpenOrderNotionalUSDT)
	availableCapacity := math.Max(0, cycleCap-committed)

	watchBySymbol := map[string]agent2.WatchCandidate{}
	for _, c := range plan.Watchlist.Candidates {
		watchBySymbol[strings.ToUpper(c.Symbol)] = c
	}

	assets := make([]AssetCapitalPlan, 0, len(plan.Assets))
	rawOpportunity := make([]float64, 0, len(plan.Assets))
	hardMarketRisk := marketRiskHardBlock(analysis)
	for _, asset := range plan.Assets {
		symbol := strings.ToUpper(asset.Symbol)
		allocation := math.Max(0, cfg.Portfolio.Allocation[symbol])
		strategicCap := investable * allocation
		if cfg.Risk.MaxSingleAssetDeployment > 0 {
			strategicCap = math.Min(strategicCap, investable*clamp01(cfg.Risk.MaxSingleAssetDeployment))
		}
		assetExposure := exposure.Assets[symbol]
		existing := math.Max(0, assetExposure.PositionCostUSDT) + math.Max(0, assetExposure.OpenOrderNotionalUSDT)
		remainingStrategic := math.Max(0, strategicCap-existing)
		candidate := watchBySymbol[symbol]
		readiness := candidate.ReadinessScore
		if readiness <= 0 {
			readiness = asset.SetupScore
		}
		readiness = clamp01(readiness)
		opportunityMult := readinessMultiplier(readiness) * statePotentialMultiplier(asset.State) * permissionPotentialMultiplier(plan.ActionPermission)
		raw := remainingStrategic * opportunityMult
		if hardMarketRisk {
			raw = 0
		}
		trigger := firstNonEmpty(asset.NextTrigger, candidate.NextTrigger)
		assets = append(assets, AssetCapitalPlan{
			Symbol:                 symbol,
			State:                  asset.State,
			Readiness:              readiness,
			TargetAllocationPct:    allocation,
			StrategicCapUSDT:       round2(strategicCap),
			ExistingExposureUSDT:   round2(existing),
			RemainingStrategicUSDT: round2(remainingStrategic),
			Invalidation:           finite(asset.Invalidation),
			NextTrigger:            trigger,
		})
		rawOpportunity = append(rawOpportunity, raw)
	}

	remainingCapacity := availableCapacity
	// Reserve cycle capacity by action priority. Within each state bucket, budget is
	// proportional to opportunity demand so no single candidate wins due to input order.
	for _, state := range []agent2.State{agent2.StateActiveLimit, agent2.StateArmed, agent2.StateScout, agent2.StateWatch} {
		indices := make([]int, 0)
		groupDemand := 0.0
		for i := range assets {
			if assets[i].State == state && rawOpportunity[i] > 0 {
				indices = append(indices, i)
				groupDemand += rawOpportunity[i]
			}
		}
		if groupDemand <= 0 || remainingCapacity <= 0 {
			continue
		}
		target := math.Min(remainingCapacity, groupDemand)
		scale := target / groupDemand
		assigned := 0.0
		for pos, i := range indices {
			budget := round2(rawOpportunity[i] * scale)
			if pos == len(indices)-1 {
				budget = round2(math.Max(0, target-assigned))
			}
			budget = math.Min(budget, math.Max(0, target-assigned))
			assets[i].OpportunityBudgetUSDT = budget
			assigned += budget
		}
		remainingCapacity = math.Max(0, remainingCapacity-assigned)
	}

	executableTotal := 0.0
	opportunityTotal := 0.0
	for i := range assets {
		executable := 0.0
		btcAccumulationConfirmed := analysis.BTCAccumulation.Phase == "ACCUMULATION_CONFIRMED"
		if plan.ActionPermission == agent1.Allowed && btcAccumulationConfirmed && plan.State == agent2.StateActiveLimit && assets[i].State == agent2.StateActiveLimit && !hardMarketRisk {
			executable = assets[i].OpportunityBudgetUSDT
		}
		assets[i].ExecutableBudgetUSDT = round2(executable)
		assets[i].LayerBudgetsUSDT = layerBudgets(assets[i].ExecutableBudgetUSDT, cfg.Execution.LayerDistribution)
		assets[i].Tier = capitalTier(assets[i].State, plan.ActionPermission, assets[i].Readiness, executable)
		assets[i].Reason = capitalReason(agent2.AssetPlan{State: assets[i].State}, plan.ActionPermission, assets[i].Readiness, executable)
		if assets[i].RemainingStrategicUSDT <= 0 {
			assets[i].Tier = "BLOCK"
			assets[i].Reason = "trần chiến lược của tài sản đã được dùng hết bởi vị thế/lệnh mở"
		}
		if hardMarketRisk {
			assets[i].Tier = "BLOCK"
			assets[i].Reason = "hard market-risk block (panic/falling-knife/FOMO); giữ tiền mặt, không cấp ngân sách thực thi"
		}
		executableTotal += assets[i].ExecutableBudgetUSDT
		opportunityTotal += assets[i].OpportunityBudgetUSDT
	}
	sort.SliceStable(assets, func(i, j int) bool {
		if assets[i].ExecutableBudgetUSDT != assets[j].ExecutableBudgetUSDT {
			return assets[i].ExecutableBudgetUSDT > assets[j].ExecutableBudgetUSDT
		}
		if assets[i].OpportunityBudgetUSDT != assets[j].OpportunityBudgetUSDT {
			return assets[i].OpportunityBudgetUSDT > assets[j].OpportunityBudgetUSDT
		}
		return assets[i].Symbol < assets[j].Symbol
	})
	return CapitalPlan{
		TotalCapitalUSDT:           round2(total),
		ReserveCashUSDT:            round2(reserve),
		InvestableCapitalUSDT:      round2(investable),
		CycleDeploymentCapUSDT:     round2(cycleCap),
		ExistingPositionUSDT:       round2(exposure.PositionCostUSDT),
		OpenOrderNotionalUSDT:      round2(exposure.OpenOrderNotionalUSDT),
		AlreadyCommittedUSDT:       round2(committed),
		AvailableCycleCapacityUSDT: round2(availableCapacity),
		ExecutableNowUSDT:          round2(executableTotal),
		OpportunityReservedUSDT:    round2(opportunityTotal),
		UnusedCycleCapacityUSDT:    round2(math.Max(0, availableCapacity-opportunityTotal)),
		ExposureSource:             exposure.Source,
		Assets:                     assets,
		Policy:                     "Vị thế và lệnh mở được trừ trước; chỉ ACTIVE_LIMIT + ALLOWED + ACCUMULATION_CONFIRMED mới có ngân sách thực thi. WATCH/SCOUT/ARMED chỉ là ngân sách cơ hội, không tạo lệnh.",
	}
}

func normalizeExposure(in ExposureSnapshot) ExposureSnapshot {
	out := ExposureSnapshot{
		PositionCostUSDT:      math.Max(0, finite(in.PositionCostUSDT)),
		OpenOrderNotionalUSDT: math.Max(0, finite(in.OpenOrderNotionalUSDT)),
		Assets:                map[string]AssetExposure{},
		Source:                strings.TrimSpace(in.Source),
	}
	for symbol, value := range in.Assets {
		out.Assets[strings.ToUpper(symbol)] = AssetExposure{
			PositionCostUSDT:      math.Max(0, finite(value.PositionCostUSDT)),
			OpenOrderNotionalUSDT: math.Max(0, finite(value.OpenOrderNotionalUSDT)),
		}
	}
	return out
}

func buildMonitoringPlan(cfg config.Config, a agent1.MarketAnalysis, p agent2.Plan) MonitoringPlan {
	recommended := recommendedScanMinutes(a, p)
	configured := cfg.Monitoring.MarketScanIntervalMinutes
	if configured <= 0 {
		configured = 60
	}
	digest := cfg.Monitoring.TelegramDigestIntervalMinutes
	if digest <= 0 {
		digest = 360
	}
	repeat := cfg.Monitoring.CriticalRepeatMinutes
	if repeat <= 0 {
		repeat = 60
	}
	maxErrors := cfg.Monitoring.MaxConsecutiveScanErrors
	if maxErrors <= 0 {
		maxErrors = 3
	}
	focus := []string{"BTC permission/regime/flow", "watchlist readiness + trigger", "discount zone + reward/risk", "order/reconcile/data-health state"}
	return MonitoringPlan{
		Enabled:                  cfg.Monitoring.Enabled,
		ConfiguredScanMinutes:    configured,
		RecommendedScanMinutes:   recommended,
		TelegramDigestMinutes:    digest,
		NotifyOnStateChange:      cfg.Monitoring.NotifyOnStateChange,
		NotifyOnCritical:         cfg.Monitoring.NotifyOnCritical,
		CriticalRepeatMinutes:    repeat,
		MaxConsecutiveScanErrors: maxErrors,
		Focus:                    focus,
	}
}

func recommendedScanMinutes(a agent1.MarketAnalysis, p agent2.Plan) int {
	if a.MarketRegime == "PANIC_SELLING" || a.FallingKnifeRisk == agent1.High || p.State == agent2.StateActiveLimit {
		return 15
	}
	if p.State == agent2.StateArmed {
		return 15
	}
	if p.State == agent2.StateScout || a.ActionPermission == agent1.Allowed {
		return 30
	}
	return 60
}

func buildRuntimePlan(cfg config.Config) RuntimePlan {
	authority := "Không có quyền đặt lệnh"
	if cfg.Execution.RealTradingEnabled && cfg.Live.Enabled && !cfg.Live.ProofOnly {
		if cfg.Live.SupervisorEnabled && cfg.Live.OrderManagementEnabled && cfg.Live.AutoExecute {
			authority = "Supervisor/managed engine chỉ có quyền khi ACTIVE_LIMIT + ALLOWED + ACCUMULATION_CONFIRMED và live guards sạch"
		} else {
			authority = "Chỉ đường đặt lệnh live thủ công/gated; supervisor chưa có toàn quyền"
		}
	} else if cfg.Execution.PaperTrading {
		authority = "Chỉ paper trading; không gửi lệnh thật"
	}
	return RuntimePlan{
		AppMode:                   cfg.App.Mode,
		LiveEnabled:               cfg.Live.Enabled,
		RealTradingEnabled:        cfg.Execution.RealTradingEnabled,
		AutoExecute:               cfg.Live.AutoExecute,
		SupervisorEnabled:         cfg.Live.SupervisorEnabled,
		OrderManagementEnabled:    cfg.Live.OrderManagementEnabled,
		ProofOnly:                 cfg.Live.ProofOnly,
		DailyRunTime:              firstNonEmpty(cfg.App.DailyRunTime, "08:00"),
		ReconcileIntervalMinutes:  positiveOr(cfg.App.ReconcileIntervalMinutes, 15),
		ManagementIntervalMinutes: positiveOr(cfg.Live.ManagementIntervalMinutes, 15),
		ExecutionAuthority:        authority,
		SafetyPolicy:              "Spot-limit BUY post-only; không futures, không leverage, không market order; fail-closed khi dữ liệu/reconcile/risk không sạch.",
	}
}

func buildTelegramPlan() TelegramPlan {
	return TelegramPlan{
		ImmediateEvents: []string{"regime/permission/plan-state thay đổi", "ACTIVE_LIMIT xuất hiện hoặc mất hiệu lực", "data-health/reconcile/risk block", "order submitted/partial/filled/cancelled/rejected", "operator halt/resume", "scheduler lỗi liên tiếp"},
		DigestContents:  []string{"kết luận hành động", "BTC regime/permission/flow/risk", "phân bổ vốn theo coin và từng layer", "watchlist readiness + trigger + invalidation", "trạng thái bot/reconcile/supervisor"},
		NoisePolicy:     "Không gửi lại cùng fingerprint trong cửa sổ chống spam; cảnh báo critical được nhắc lại theo critical_repeat_minutes.",
	}
}

func marketRiskHardBlock(a agent1.MarketAnalysis) bool {
	return a.MarketRegime == "PANIC_SELLING" || a.FallingKnifeRisk == agent1.High || a.FomoRisk == agent1.High
}

func capitalTier(state agent2.State, permission agent1.Permission, readiness, executable float64) string {
	if executable > 0 {
		return "EXECUTE"
	}
	if permission == agent1.NoTrade || state == agent2.StateNoTrade {
		return "BLOCK"
	}
	if state == agent2.StateArmed && readiness >= 0.55 {
		return "WATCH_STRONG"
	}
	if state == agent2.StateScout || state == agent2.StateWatch || state == agent2.StateArmed {
		return "WATCH"
	}
	return "BLOCK"
}

func capitalReason(asset agent2.AssetPlan, permission agent1.Permission, readiness, executable float64) string {
	if executable > 0 {
		return fmt.Sprintf("ACTIVE_LIMIT + ALLOWED + ACCUMULATION_CONFIRMED, readiness %.0f%%; ngân sách được chia theo layer_distribution", readiness*100)
	}
	if permission != agent1.Allowed {
		return fmt.Sprintf("BTC permission %s chưa cấp quyền thực thi", permission)
	}
	if asset.State != agent2.StateActiveLimit {
		return fmt.Sprintf("asset state %s; chờ trigger trước khi cấp ngân sách thực thi", asset.State)
	}
	return "chưa đủ chất lượng để cấp ngân sách thực thi"
}

func readinessMultiplier(v float64) float64 {
	switch {
	case v >= 0.85:
		return 1
	case v >= 0.70:
		return 0.75
	case v >= 0.55:
		return 0.40
	case v >= 0.40:
		return 0.15
	default:
		return 0
	}
}

func statePotentialMultiplier(state agent2.State) float64 {
	switch state {
	case agent2.StateActiveLimit:
		return 1
	case agent2.StateArmed:
		return 0.35
	case agent2.StateScout:
		return 0.15
	case agent2.StateWatch:
		return 0.05
	default:
		return 0
	}
}

func permissionPotentialMultiplier(permission agent1.Permission) float64 {
	switch permission {
	case agent1.Allowed:
		return 1
	case agent1.Armed:
		return 0.35
	case agent1.Watch:
		return 0.10
	default:
		return 0
	}
}

func layerBudgets(total float64, distribution []float64) []float64 {
	if total <= 0 || len(distribution) == 0 {
		return nil
	}
	out := make([]float64, 0, len(distribution))
	remaining := round2(total)
	for i, fraction := range distribution {
		budget := round2(total * fraction)
		if i == len(distribution)-1 {
			budget = remaining
		}
		if budget < 0 {
			budget = 0
		}
		out = append(out, budget)
		remaining = round2(remaining - budget)
	}
	return out
}

func fingerprint(report Report) string {
	type assetState struct {
		Symbol          string `json:"symbol"`
		State           string `json:"state"`
		Tier            string `json:"tier"`
		Readiness       int    `json:"readiness_bucket"`
		Exposure        int    `json:"exposure_bucket"`
		Executable      int    `json:"executable_bucket"`
		TriggerSemantic string `json:"trigger_semantic"`
	}
	assets := make([]assetState, 0, len(report.Capital.Assets))
	for _, a := range report.Capital.Assets {
		assets = append(assets, assetState{
			Symbol:          a.Symbol,
			State:           string(a.State),
			Tier:            a.Tier,
			Readiness:       int(math.Round(a.Readiness*20)) * 5,
			Exposure:        ratioBucket(a.ExistingExposureUSDT, a.StrategicCapUSDT, 10),
			Executable:      ratioBucket(a.ExecutableBudgetUSDT, a.StrategicCapUSDT, 10),
			TriggerSemantic: semanticTrigger(a.NextTrigger),
		})
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Symbol < assets[j].Symbol })
	stable := struct {
		Regime            string       `json:"regime"`
		Permission        string       `json:"permission"`
		PlanState         string       `json:"plan_state"`
		Risk              string       `json:"risk"`
		Falling           string       `json:"falling"`
		FOMO              string       `json:"fomo"`
		FlowBias          string       `json:"flow_bias"`
		Urgency           string       `json:"urgency"`
		AvailableCapacity int          `json:"available_capacity_bucket"`
		Assets            []assetState `json:"assets"`
	}{
		Regime:            report.Market.Regime,
		Permission:        string(report.Market.Permission),
		PlanState:         string(report.Market.PlanState),
		Risk:              string(report.Market.Risk),
		Falling:           string(report.Market.FallingKnifeRisk),
		FOMO:              string(report.Market.FOMORisk),
		FlowBias:          report.Market.FlowBias,
		Urgency:           report.Market.Urgency,
		AvailableCapacity: ratioBucket(report.Capital.AvailableCycleCapacityUSDT, report.Capital.CycleDeploymentCapUSDT, 10),
		Assets:            assets,
	}
	b, _ := json.Marshal(stable)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func ratioBucket(value, cap float64, step int) int {
	if step <= 0 {
		step = 10
	}
	if cap <= 0 {
		if value > 0 {
			return 100
		}
		return 0
	}
	pct := clamp01(value/cap) * 100
	return int(math.Floor(pct/float64(step)+1e-9)) * step
}

func semanticTrigger(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	var b strings.Builder
	inNumber := false
	for _, r := range value {
		isNumeric := r >= '0' && r <= '9'
		if isNumeric || (inNumber && strings.ContainsRune(".,%+-$", r)) {
			if !inNumber {
				b.WriteByte('#')
			}
			inNumber = true
			continue
		}
		inNumber = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func finite(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func round2(v float64) float64 { return math.Round(finite(v)*100) / 100 }

func positiveOr(v, fallback int) int {
	if v > 0 {
		return v
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
