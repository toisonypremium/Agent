package config

import (
	"errors"
	"fmt"
	"strings"
)

// Validate checks all configuration sections for correctness.
// It returns the first error found, or nil if the configuration is valid.
func (c Config) Validate() error {
	if err := c.validateApp(); err != nil {
		return err
	}
	if err := c.validateExecution(); err != nil {
		return err
	}
	if err := c.validateLive(); err != nil {
		return err
	}
	if err := c.validateHermesOperator(); err != nil {
		return err
	}
	if err := c.validateRisk(); err != nil {
		return err
	}
	if err := c.validatePortfolio(); err != nil {
		return err
	}
	if err := c.validateData(); err != nil {
		return err
	}
	if err := c.validateMaintenance(); err != nil {
		return err
	}
	if err := c.validateResearch(); err != nil {
		return err
	}
	if err := c.validateMonitoring(); err != nil {
		return err
	}
	if err := c.validateMicrostructure(); err != nil {
		return err
	}
	if err := c.validateStorage(); err != nil {
		return err
	}
	return nil
}

func (c Config) validateApp() error {
	if c.App.Mode == "" {
		return errors.New("app.mode required")
	}
	if c.App.Mode != "paper" && c.App.Mode != "report" && c.App.Mode != "live" {
		return errors.New("app.mode must be paper, report, or live")
	}
	if c.App.ReconcileIntervalMinutes < 0 {
		return errors.New("app.reconcile_interval_minutes cannot be negative")
	}
	if c.App.DailyRunTime != "" && !validClockTime(c.App.DailyRunTime) {
		return errors.New("app.daily_run_time must be HH:MM")
	}
	return nil
}

func (c Config) validateExecution() error {
	if !c.Execution.PaperTrading && !c.Execution.RealTradingEnabled {
		return errors.New("paper_trading or real_trading_enabled must be true")
	}
	if c.Execution.OrderExpiryHours <= 0 {
		return errors.New("execution.order_expiry_hours must be >0")
	}
	if len(c.Execution.LayerDistribution) == 0 {
		return errors.New("execution.layer_distribution required")
	}
	ls := 0.0
	for _, v := range c.Execution.LayerDistribution {
		if v <= 0 {
			return errors.New("layer_distribution values must be positive")
		}
		ls += v
	}
	if ls < 0.999 || ls > 1.001 {
		return errors.New("layer_distribution must sum to 1")
	}
	return nil
}

func (c Config) validateLive() error {
	if c.Execution.RealTradingEnabled {
		if !c.Live.Enabled || c.Live.ProofOnly {
			return errors.New("real trading requires live.enabled=true and live.proof_only=false")
		}
		if c.Live.AutoExecute {
			if c.Live.RequireManualConfirm {
				return errors.New("auto live execution requires live.require_manual_confirm=false")
			}
		} else if !c.Live.RequireManualConfirm {
			return errors.New("manual live execution requires live.require_manual_confirm=true unless live.auto_execute=true")
		}
		if c.Live.MaxOrderNotionalUSDT <= 0 || c.Live.MaxOrderNotionalUSDT > 10000 {
			return errors.New("live max_order_notional_usdt must be >0 and <=10000")
		}
	}
	if c.Live.Enabled && c.Live.MaxOrderNotionalUSDT > 10000 {
		return errors.New("live max_order_notional_usdt must be <=10000 when live.enabled=true")
	}
	if LiveAutoMode(c) && (c.Live.Enabled || c.Execution.RealTradingEnabled) {
		if !c.Live.Enabled || !c.Live.AutoExecute || c.Live.RequireManualConfirm || c.Live.ProofOnly || !c.Execution.RealTradingEnabled {
			return errors.New("live_auto_mode requires live enabled, auto_execute, no manual confirm, proof_only=false, and real trading enabled")
		}
		if !c.Live.SupervisorEnabled || !c.Live.OrderManagementEnabled {
			return errors.New("live_auto_mode requires live supervisor and order management")
		}
		if LiveAutoMaxNotionalUSDT(c) <= 0 {
			return errors.New("live live_auto_max_notional_usdt must be positive when live_auto_mode is enabled")
		}
		if LiveAutoMaxNotionalUSDT(c) > c.Live.MaxOrderNotionalUSDT {
			return fmt.Errorf("live live_auto_max_notional_usdt (%.2f) cannot exceed max_order_notional_usdt (%.2f)", LiveAutoMaxNotionalUSDT(c), c.Live.MaxOrderNotionalUSDT)
		}
	}
	if c.Live.FirstOrderMaxNotionalUSDT < 0 {
		return errors.New("live.first_order_max_notional_usdt cannot be negative")
	}
	if c.Live.FirstOrderQuarantineEnabled && c.Live.FirstOrderMaxNotionalUSDT > 0 && c.Live.MaxLiveNotionalPerOrderUSDT > 0 && c.Live.FirstOrderMaxNotionalUSDT > c.Live.MaxLiveNotionalPerOrderUSDT {
		return errors.New("live.first_order_max_notional_usdt must be <= live.max_live_notional_per_order_usdt")
	}
	if c.Live.OrderManagementEnabled {
		if !c.Live.AutoExecute {
			return errors.New("order management requires live.auto_execute=true")
		}
		if c.Live.MaxAutoLayersPerAsset < 1 || c.Live.MaxAutoLayersPerAsset > 3 {
			return errors.New("live.max_auto_layers_per_asset must be between 1 and 3")
		}
		if c.Live.MaxOpenLiveOrdersPerAsset < 1 || c.Live.MaxOpenLiveOrdersPerAsset > 10 {
			return errors.New("live.max_open_live_orders_per_asset must be between 1 and 10")
		}
		if c.Live.MaxOpenLiveOrdersTotal < c.Live.MaxOpenLiveOrdersPerAsset {
			return errors.New("live.max_open_live_orders_total must be >= live.max_open_live_orders_per_asset")
		}
		if c.Live.MaxLiveNotionalPerOrderUSDT <= 0 || c.Live.MaxLiveNotionalPerOrderUSDT > c.Live.MaxOrderNotionalUSDT {
			return errors.New("live.max_live_notional_per_order_usdt must be >0 and <= live.max_order_notional_usdt")
		}
		if c.Live.MaxLiveNotionalPerAssetUSDT < c.Live.MaxLiveNotionalPerOrderUSDT {
			return errors.New("live.max_live_notional_per_asset_usdt must be >= live.max_live_notional_per_order_usdt")
		}
		if c.Live.MaxLiveNotionalTotalUSDT < c.Live.MaxLiveNotionalPerAssetUSDT {
			return errors.New("live.max_live_notional_total_usdt must be >= live.max_live_notional_per_asset_usdt")
		}
		if c.Live.CancelIfPriceAboveDiscountZonePct < 0 {
			return errors.New("live.cancel_if_price_above_discount_zone_pct cannot be negative")
		}
		if c.Live.ReplaceIfPriceDriftPct < 0 {
			return errors.New("live.replace_if_price_drift_pct cannot be negative")
		}
		if c.Live.CancelStaleAfterMinutes < 0 {
			return errors.New("live.cancel_stale_after_minutes cannot be negative")
		}
	}
	if c.Live.HeartbeatIntervalMinutes < 0 {
		return errors.New("live.heartbeat_interval_minutes cannot be negative")
	}
	if c.Live.MaxSpreadBps < 0 || c.Live.MaxSlippageBps < 0 || c.Live.MinBidDepthToOrderRatio < 0 || c.Live.MaxOrderToAvg5mQuoteVolumePct < 0 {
		return errors.New("live liquidity gate values cannot be negative")
	}
	if c.Live.MMGateSamples < 0 || c.Live.MMGateSampleDelayMs < 0 {
		return errors.New("live MM gate sample values cannot be negative")
	}
	if c.Live.MMGateSamples > 3 {
		return errors.New("live.mm_gate_samples must be <=3")
	}
	if c.Live.AutoHaltAfterErrors < 0 {
		return errors.New("live.auto_halt_after_errors cannot be negative")
	}
	if c.Live.MinAccountFreeUSDT < 0 {
		return errors.New("live.min_account_free_usdt must be >=0")
	}
	if c.Live.SupervisorEnabled {
		if !c.Live.Enabled {
			return errors.New("live supervisor requires live.enabled=true")
		}
		if !c.Live.AutoExecute {
			return errors.New("live supervisor requires live.auto_execute=true")
		}
		if !c.Live.OrderManagementEnabled {
			return errors.New("live supervisor requires live.order_management_enabled=true")
		}
		if c.Live.ManagementIntervalMinutes < 1 {
			return errors.New("live.management_interval_minutes must be >=1 when supervisor is enabled")
		}
	}
	return nil
}

func (c Config) validateHermesOperator() error {
	if !c.HermesOperator.Enabled {
		return nil
	}
	mode := c.HermesOperator.NormalizedMode()
	if mode != "observe" && mode != "shadow" && mode != "canary" && mode != "autonomous" {
		return errors.New("hermes_operator.mode must be observe, shadow, canary, or autonomous")
	}
	if c.HermesOperator.DecisionTTLSeconds < 15 || c.HermesOperator.DecisionTTLSeconds > 600 {
		return errors.New("hermes_operator.decision_ttl_seconds must be between 15 and 600")
	}
	if c.HermesOperator.MinConfidence < 0 || c.HermesOperator.MinConfidence > 1 {
		return errors.New("hermes_operator.min_confidence must be between 0 and 1")
	}
	if c.HermesOperator.MaxActionsPerCycle < 1 || c.HermesOperator.MaxActionsPerCycle > 20 {
		return errors.New("hermes_operator.max_actions_per_cycle must be between 1 and 20")
	}
	if c.HermesOperator.MaxProbeNotionalUSDT <= 0 || c.HermesOperator.MaxActionNotionalUSDT <= 0 {
		return errors.New("hermes_operator notional caps must be positive")
	}
	if c.HermesOperator.MaxProbeNotionalUSDT > c.HermesOperator.MaxActionNotionalUSDT {
		return errors.New("hermes_operator.max_probe_notional_usdt must be <= max_action_notional_usdt")
	}
	if c.Live.MaxLiveNotionalPerOrderUSDT > 0 && c.HermesOperator.MaxActionNotionalUSDT > c.Live.MaxLiveNotionalPerOrderUSDT {
		return errors.New("hermes_operator.max_action_notional_usdt must be <= live.max_live_notional_per_order_usdt")
	}
	if c.HermesOperator.MaxPortfolioExposureUSDT <= 0 || (c.Live.MaxLiveNotionalTotalUSDT > 0 && c.HermesOperator.MaxPortfolioExposureUSDT > c.Live.MaxLiveNotionalTotalUSDT) {
		return errors.New("hermes_operator.max_portfolio_exposure_usdt must be positive and <= live.max_live_notional_total_usdt")
	}
	if c.HermesOperator.CanExecute() && (!c.Live.Enabled || !c.Live.AutoExecute || !c.Execution.RealTradingEnabled) {
		return errors.New("Hermes canary/autonomous requires live enabled, auto_execute, and real trading enabled")
	}
	return nil
}

func (c Config) validateRisk() error {
	if !c.Risk.NoFutures || !c.Risk.NoLeverage || !c.Risk.SpotLimitOnly {
		return errors.New("risk flags must enforce no futures, no leverage, spot limit only")
	}
	if c.Risk.MaxTotalDeploymentPerCycle <= 0 || c.Risk.MaxTotalDeploymentPerCycle > 1 {
		return errors.New("risk.max_total_deployment_per_cycle must be >0 and <=1")
	}
	if c.Risk.MaxSingleAssetDeployment <= 0 || c.Risk.MaxSingleAssetDeployment > 1 {
		return errors.New("risk.max_single_asset_deployment must be >0 and <=1")
	}
	if c.Risk.MinRewardRisk <= 0 {
		return errors.New("risk.min_reward_risk must be positive")
	}
	if c.Risk.BTCTrendArmedThreshold < 0 || c.Risk.BTCTrendAllowedThreshold < 0 || c.Risk.BTCFlowPromoteThreshold < 0 || c.Risk.BTCPermissionMinRewardRisk < 0 {
		return errors.New("BTC decision thresholds cannot be negative")
	}
	if c.Risk.RelativeStrengthLookbackDays < 0 {
		return errors.New("risk.relative_strength_lookback_days must be >0 when set")
	}
	if c.Risk.MaxRotationRank < 0 {
		return errors.New("risk.max_rotation_rank must be >0 when set")
	}
	if c.Risk.BTCTrendArmedThreshold > 0 && c.Risk.BTCTrendAllowedThreshold > 0 && c.Risk.BTCTrendAllowedThreshold < c.Risk.BTCTrendArmedThreshold {
		return errors.New("risk.btc_trend_allowed_threshold must be >= btc_trend_armed_threshold")
	}
	if c.Risk.MinWatchReadinessForProbe < 0 || c.Risk.MinWatchReadinessForProbe > 1 {
		return errors.New("risk.min_watch_readiness_for_probe must be between 0 and 1")
	}
	if c.Risk.FlowBearHardBlockScore < 0 || c.Risk.FlowBearHardBlockScore > 1 {
		return errors.New("risk.flow_bear_hard_block_score must be between 0 and 1")
	}
	if c.Risk.MinScoutRewardRisk < 0 {
		return errors.New("risk.min_scout_reward_risk cannot be negative")
	}
	if c.Risk.MinScoutRewardRisk > 0 && c.Risk.MinScoutRewardRisk > c.Risk.MinRewardRisk {
		return errors.New("risk.min_scout_reward_risk must be <= risk.min_reward_risk")
	}
	if c.Risk.DiscountZonePremiumPct < 0 {
		return errors.New("risk.discount_zone_premium_pct cannot be negative")
	}
	return nil
}

func (c Config) validatePortfolio() error {
	if c.Portfolio.TotalCapital <= 0 {
		return errors.New("portfolio.total_capital must be positive")
	}
	if c.Portfolio.ReserveCashRatio < 0 || c.Portfolio.ReserveCashRatio >= 1 {
		return errors.New("portfolio.reserve_cash_ratio must be >=0 and <1")
	}
	if strings.ToUpper(c.Portfolio.BaseCurrency) != "USDT" {
		return errors.New("only USDT base currency supported")
	}
	if len(c.Data.Symbols.Assets) != 3 {
		return errors.New("data.symbols.assets must contain exactly 3 accumulation assets")
	}
	assetSet := map[string]bool{}
	allocation := map[string]float64{}
	for sym, value := range c.Portfolio.Allocation {
		normalized := strings.ToUpper(strings.TrimSpace(sym))
		allocation[normalized] += value
	}
	btcSymbol := strings.ToUpper(strings.TrimSpace(c.Data.Symbols.BTC))
	for _, s := range c.Data.Symbols.Assets {
		sym := strings.ToUpper(strings.TrimSpace(s))
		if sym == "" {
			return errors.New("asset symbol cannot be empty")
		}
		if sym == btcSymbol {
			return errors.New("BTC symbol must not be included in accumulation assets")
		}
		if assetSet[sym] {
			return fmt.Errorf("duplicate asset symbol %s", s)
		}
		assetSet[sym] = true
		if allocation[sym] <= 0 {
			return fmt.Errorf("missing or zero allocation for %s", sym)
		}
	}
	researchSet := map[string]bool{}
	for _, s := range c.Data.Symbols.ResearchUniverse {
		sym := strings.ToUpper(strings.TrimSpace(s))
		if sym == "" {
			return errors.New("research universe symbol cannot be empty")
		}
		if sym == btcSymbol {
			return errors.New("BTC symbol must not be included in research universe")
		}
		if researchSet[sym] {
			return fmt.Errorf("duplicate research universe symbol %s", s)
		}
		researchSet[sym] = true
	}
	sum := 0.0
	for sym, v := range allocation {
		if v < 0 {
			return errors.New("allocation cannot be negative")
		}
		if sym == btcSymbol {
			return errors.New("portfolio allocation must not include BTC; BTC is market gate only")
		}
		if !assetSet[sym] {
			return fmt.Errorf("allocation for %s is not in data.symbols.assets", sym)
		}
		sum += v
	}
	if sum > 1.000001 {
		return errors.New("allocation sum must be <=1")
	}
	return nil
}

func (c Config) validateData() error {
	if c.Data.BinanceBaseURL == "" || c.Data.Symbols.BTC == "" || len(c.Data.Symbols.Assets) == 0 || len(c.Data.Intervals) == 0 {
		return errors.New("data source/symbols/intervals required")
	}
	if c.Data.CandleLimit < 100 {
		return errors.New("data.candle_limit must be >=100")
	}
	return nil
}

func (c Config) validateMaintenance() error {
	if c.Maintenance.ReportRetentionDays < 0 || c.Maintenance.EventRetentionDays < 0 || c.Maintenance.MaxReportFiles < 0 || c.Maintenance.MaxClosedPaperOrders < 0 || c.Maintenance.MaxCandlesPerSymbolInterval < 0 || c.Maintenance.MaxAnalysisRows < 0 || c.Maintenance.MaxPlanRows < 0 {
		return errors.New("maintenance retention values cannot be negative")
	}
	if c.Maintenance.SchedulerTime != "" && !validClockTime(c.Maintenance.SchedulerTime) {
		return errors.New("maintenance.scheduler_time must be HH:MM")
	}
	return nil
}

func (c Config) validateResearch() error {
	if c.Research.BriefIntervalMinutes < 0 {
		return errors.New("research.brief_interval_minutes cannot be negative")
	}
	if c.Research.ExpertIntervalMinutes < 0 {
		return errors.New("research.expert_interval_minutes cannot be negative")
	}
	if c.Research.ExpertMaxSections < 0 || c.Research.ExpertMaxItems < 0 {
		return errors.New("research expert limits cannot be negative")
	}
	if c.Research.MaxSourcesPerCycle < 0 {
		return errors.New("research.max_sources_per_cycle cannot be negative")
	}
	if c.Research.Enabled {
		if c.Research.RequestTimeoutSeconds < 1 {
			return errors.New("research.request_timeout_seconds must be >=1 when research is enabled")
		}
		if c.Research.RSS.Enabled && len(c.Research.RSS.Feeds) == 0 {
			return errors.New("research.rss.feeds required when research RSS is enabled")
		}
	}
	return nil
}

func (c Config) validateMonitoring() error {
	if c.Monitoring.MarketScanIntervalMinutes < 0 || c.Monitoring.TelegramDigestIntervalMinutes < 0 || c.Monitoring.CriticalRepeatMinutes < 0 || c.Monitoring.MaxConsecutiveScanErrors < 0 {
		return errors.New("monitoring interval/error values cannot be negative")
	}
	if c.Monitoring.Enabled {
		if c.Monitoring.MarketScanIntervalMinutes < 5 {
			return errors.New("monitoring.market_scan_interval_minutes must be >=5 when monitoring is enabled")
		}
		if c.Monitoring.TelegramDigestIntervalMinutes > 0 && c.Monitoring.TelegramDigestIntervalMinutes < 15 {
			return errors.New("monitoring.telegram_digest_interval_minutes must be 0 or >=15")
		}
		if c.Monitoring.CriticalRepeatMinutes > 0 && c.Monitoring.CriticalRepeatMinutes < 15 {
			return errors.New("monitoring.critical_repeat_minutes must be 0 or >=15")
		}
		if c.Monitoring.MaxConsecutiveScanErrors == 0 {
			return errors.New("monitoring.max_consecutive_scan_errors must be >=1 when monitoring is enabled")
		}
	}
	return nil
}

func (c Config) validateMicrostructure() error {
	if c.Microstructure.LookbackLimit < 0 || c.Microstructure.MaxAgeMinutes < 0 || c.Microstructure.OrderBookDepthLimit < 0 || c.Microstructure.MinFreshSymbolsRequired < 0 {
		return errors.New("microstructure values cannot be negative")
	}
	if c.Microstructure.Enabled {
		if strings.TrimSpace(c.Microstructure.BinanceSpotBaseURL) == "" {
			return errors.New("microstructure.binance_spot_base_url required when microstructure is enabled")
		}
		if strings.TrimSpace(c.Microstructure.BinanceFuturesBaseURL) == "" {
			return errors.New("microstructure.binance_futures_base_url required when microstructure is enabled")
		}
		if c.Microstructure.LookbackLimit > 1000 {
			return errors.New("microstructure.lookback_limit must be <=1000")
		}
		if c.Microstructure.OrderBookDepthLimit > 1000 {
			return errors.New("microstructure.orderbook_depth_limit must be <=1000")
		}
	}
	return nil
}

func (c Config) validateStorage() error {
	if c.Storage.Path == "" {
		return errors.New("storage.path required")
	}
	return nil
}
