package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App struct {
		Mode                     string `yaml:"mode"`
		Timezone                 string `yaml:"timezone"`
		DailyRunTime             string `yaml:"daily_run_time"`
		ReconcileIntervalMinutes int    `yaml:"reconcile_interval_minutes"`
	} `yaml:"app"`
	Storage struct {
		Path string `yaml:"path"`
	} `yaml:"storage"`
	Portfolio struct {
		BaseCurrency     string             `yaml:"base_currency"`
		TotalCapital     float64            `yaml:"total_capital"`
		Allocation       map[string]float64 `yaml:"allocation"`
		ReserveCashRatio float64            `yaml:"reserve_cash_ratio"`
	} `yaml:"portfolio"`
	Risk struct {
		MaxTotalDeploymentPerCycle    float64 `yaml:"max_total_deployment_per_cycle"`
		MaxSingleAssetDeployment      float64 `yaml:"max_single_asset_deployment"`
		MinRewardRisk                 float64 `yaml:"min_reward_risk"`
		DecisionProfile               string  `yaml:"decision_profile"`
		BTCTrendArmedThreshold        float64 `yaml:"btc_trend_armed_threshold"`
		BTCTrendAllowedThreshold      float64 `yaml:"btc_trend_allowed_threshold"`
		BTCFlowPromoteThreshold       float64 `yaml:"btc_flow_promote_threshold"`
		BTCPermissionMinRewardRisk    float64 `yaml:"btc_permission_min_reward_risk"`
		MinWatchReadinessForProbe     float64 `yaml:"min_watch_readiness_for_probe"`
		StopOnPanicSelling            bool    `yaml:"stop_on_panic_selling"`
		DisableRelativeStrengthFilter bool    `yaml:"disable_relative_strength_filter"`
		RelativeStrengthLookbackDays  int     `yaml:"relative_strength_lookback_days"`
		MinRelativeStrength           float64 `yaml:"min_relative_strength"`
		MinAssetMomentum              float64 `yaml:"min_asset_momentum"`
		DisableRotationScoreFilter    bool    `yaml:"disable_rotation_score_filter"`
		MinRotationScore              float64 `yaml:"min_rotation_score"`
		MaxRotationRank               int     `yaml:"max_rotation_rank"`
		DisableAssetFlowEntryFilter   bool    `yaml:"disable_asset_flow_entry_filter"`
		MinAssetFlowBullScore         float64 `yaml:"min_asset_flow_bull_score"`
		AllowNeutralReclaimEntry      bool    `yaml:"allow_neutral_reclaim_entry"`
		StrictAssetFlowEntry          bool    `yaml:"strict_asset_flow_entry"`
		FlowBearHardBlockScore        float64 `yaml:"flow_bear_hard_block_score"`
		MinScoutRewardRisk            float64 `yaml:"min_scout_reward_risk"`
		StrictRotationRank            bool    `yaml:"strict_rotation_rank"`
		AllowScoutInDowntrend         bool    `yaml:"allow_scout_in_downtrend"`
		DiscountZonePremiumPct        float64 `yaml:"discount_zone_premium_pct"`
		NoFutures                     bool    `yaml:"no_futures"`
		NoLeverage                    bool    `yaml:"no_leverage"`
		SpotLimitOnly                 bool    `yaml:"spot_limit_only"`
	} `yaml:"risk"`
	BTCCycle struct {
		StressPriceReference         float64 `yaml:"stress_price_reference"`
		UseDynamicCycleZone          bool    `yaml:"use_dynamic_cycle_zone"`
		RequireReclaimBeforeMajorBuy bool    `yaml:"require_reclaim_before_major_buy"`
	} `yaml:"btc_cycle"`
	Data struct {
		BinanceBaseURL string `yaml:"binance_base_url"`
		Symbols        struct {
			BTC              string   `yaml:"btc"`
			Assets           []string `yaml:"assets"`
			ResearchUniverse []string `yaml:"research_universe"`
		} `yaml:"symbols"`
		Intervals   []string `yaml:"intervals"`
		CandleLimit int      `yaml:"candle_limit"`
	} `yaml:"data"`
	Maintenance struct {
		Enabled                     bool   `yaml:"enabled"`
		SchedulerEnabled            bool   `yaml:"scheduler_enabled"`
		SchedulerTime               string `yaml:"scheduler_time"`
		ReportRetentionDays         int    `yaml:"report_retention_days"`
		EventRetentionDays          int    `yaml:"event_retention_days"`
		MaxReportFiles              int    `yaml:"max_report_files"`
		MaxClosedPaperOrders        int    `yaml:"max_closed_paper_orders"`
		MaxCandlesPerSymbolInterval int    `yaml:"max_candles_per_symbol_interval"`
		MaxAnalysisRows             int    `yaml:"max_analysis_rows"`
		MaxPlanRows                 int    `yaml:"max_plan_rows"`
		WALCheckpoint               bool   `yaml:"wal_checkpoint_on_maintenance"`
	} `yaml:"maintenance"`
	Notify struct {
		Enabled        bool   `yaml:"enabled"`
		Provider       string `yaml:"provider"`
		TelegramToken  string `yaml:"telegram_token"`
		TelegramChatID string `yaml:"telegram_chat_id"`
		NtfyTopic      string `yaml:"ntfy_topic"`
	} `yaml:"notify"`
	AI struct {
		Enabled         bool    `yaml:"enabled"`
		Provider        string  `yaml:"provider"`
		Model           string  `yaml:"model"`
		BaseURLEnv      string  `yaml:"base_url_env"`
		APIKeyEnv       string  `yaml:"api_key_env"`
		MaxTokens       int     `yaml:"max_tokens"`
		Temperature     float64 `yaml:"temperature"`
		TelegramEnabled bool    `yaml:"telegram_enabled"`
	} `yaml:"ai"`
	Research struct {
		Enabled               bool `yaml:"enabled"`
		BriefIntervalMinutes  int  `yaml:"brief_interval_minutes"`
		MaxSourcesPerCycle    int  `yaml:"max_sources_per_cycle"`
		RequestTimeoutSeconds int  `yaml:"request_timeout_seconds"`
		RSS                   struct {
			Enabled bool     `yaml:"enabled"`
			Feeds   []string `yaml:"feeds"`
		} `yaml:"rss"`
	} `yaml:"research"`
	Monitoring struct {
		Enabled                       bool `yaml:"enabled"`
		MarketScanIntervalMinutes     int  `yaml:"market_scan_interval_minutes"`
		TelegramDigestIntervalMinutes int  `yaml:"telegram_digest_interval_minutes"`
		NotifyOnStateChange           bool `yaml:"notify_on_state_change"`
		NotifyOnCritical              bool `yaml:"notify_on_critical"`
		CriticalRepeatMinutes         int  `yaml:"critical_repeat_minutes"`
		MaxConsecutiveScanErrors      int  `yaml:"max_consecutive_scan_errors"`
	} `yaml:"monitoring"`
	Microstructure struct {
		Enabled                 bool   `yaml:"enabled"`
		FetchOnMarketWatch      bool   `yaml:"fetch_on_market_watch"`
		RequireFreshForActive   bool   `yaml:"require_fresh_for_active"`
		BinanceSpotBaseURL      string `yaml:"binance_spot_base_url"`
		BinanceFuturesBaseURL   string `yaml:"binance_futures_base_url"`
		Interval                string `yaml:"interval"`
		LookbackLimit           int    `yaml:"lookback_limit"`
		MaxAgeMinutes           int    `yaml:"max_age_minutes"`
		OrderBookDepthLimit     int    `yaml:"orderbook_depth_limit"`
		MinFreshSymbolsRequired int    `yaml:"min_fresh_symbols_required"`
	} `yaml:"microstructure"`
	Live struct {
		Enabled                           bool    `yaml:"enabled"`
		Exchange                          string  `yaml:"exchange"`
		APIKeyEnv                         string  `yaml:"api_key_env"`
		APISecretEnv                      string  `yaml:"api_secret_env"`
		APIPassphraseEnv                  string  `yaml:"api_passphrase_env"`
		MaxOrderNotionalUSDT              float64 `yaml:"max_order_notional_usdt"`
		MinAccountFreeUSDT                float64 `yaml:"min_account_free_usdt"`
		RequirePostOnly                   bool    `yaml:"require_post_only"`
		RequireManualConfirm              bool    `yaml:"require_manual_confirm"`
		AutoExecute                       bool    `yaml:"auto_execute"`
		LiveAutoMode                      bool    `yaml:"live_auto_mode"`
		LiveAutoMaxNotionalUSDT           float64 `yaml:"live_auto_max_notional_usdt"`
		OrderManagementEnabled            bool    `yaml:"order_management_enabled"`
		MaxAutoLayersPerAsset             int     `yaml:"max_auto_layers_per_asset"`
		MaxOpenLiveOrdersPerAsset         int     `yaml:"max_open_live_orders_per_asset"`
		MaxOpenLiveOrdersTotal            int     `yaml:"max_open_live_orders_total"`
		MaxLiveNotionalPerOrderUSDT       float64 `yaml:"max_live_notional_per_order_usdt"`
		MaxLiveNotionalPerAssetUSDT       float64 `yaml:"max_live_notional_per_asset_usdt"`
		MaxLiveNotionalTotalUSDT          float64 `yaml:"max_live_notional_total_usdt"`
		CancelIfPlanNotActive             bool    `yaml:"cancel_if_plan_not_active"`
		CancelIfPriceAboveDiscountZonePct float64 `yaml:"cancel_if_price_above_discount_zone_pct"`
		ReplaceIfPriceDriftPct            float64 `yaml:"replace_if_price_drift_pct"`
		CancelStaleAfterMinutes           int     `yaml:"cancel_stale_after_minutes"`
		CancelOnBTCPermissionNotAllowed   bool    `yaml:"cancel_on_btc_permission_not_allowed"`
		SupervisorEnabled                 bool    `yaml:"supervisor_enabled"`
		ManagementIntervalMinutes         int     `yaml:"management_interval_minutes"`
		HeartbeatIntervalMinutes          int     `yaml:"heartbeat_interval_minutes"`
		AutoHaltAfterErrors               int     `yaml:"auto_halt_after_errors"`
		NotifyOnNoAction                  bool    `yaml:"notify_on_no_action"`
		ProofOnly                         bool    `yaml:"proof_only"`
		LiquidityGateEnabled              bool    `yaml:"liquidity_gate_enabled"`
		RequireOrderBookLiquidity         bool    `yaml:"require_orderbook_liquidity"`
		MaxSpreadBps                      float64 `yaml:"max_spread_bps"`
		MaxSlippageBps                    float64 `yaml:"max_slippage_bps"`
		MinBidDepthToOrderRatio           float64 `yaml:"min_bid_depth_to_order_ratio"`
		MaxOrderToAvg5mQuoteVolumePct     float64 `yaml:"max_order_to_avg_5m_quote_volume_pct"`
		MMGateSamples                     int     `yaml:"mm_gate_samples"`
		MMGateSampleDelayMs               int     `yaml:"mm_gate_sample_delay_ms"`
	} `yaml:"live"`
	Execution struct {
		PaperTrading       bool      `yaml:"paper_trading"`
		RealTradingEnabled bool      `yaml:"real_trading_enabled"`
		OrderExpiryHours   int       `yaml:"order_expiry_hours"`
		LayerDistribution  []float64 `yaml:"layer_distribution"`
	} `yaml:"execution"`
}

func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return c, err
	}
	return c, nil
}

// LiveAutoMode returns the current live-auto mode flag.
func LiveAutoMode(c Config) bool {
	return c.Live.LiveAutoMode
}

// LiveAutoMaxNotionalUSDT returns the current live-auto cap.
func LiveAutoMaxNotionalUSDT(c Config) float64 {
	return c.Live.LiveAutoMaxNotionalUSDT
}

func validClockTime(value string) bool {
	if len(value) != 5 || value[2] != ':' {
		return false
	}
	for _, i := range []int{0, 1, 3, 4} {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	min := int(value[3]-'0')*10 + int(value[4]-'0')
	return hour >= 0 && hour <= 23 && min >= 0 && min <= 59
}

func (c Config) Validate() error {
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
	if !c.Execution.PaperTrading && !c.Execution.RealTradingEnabled {
		return errors.New("paper_trading or real_trading_enabled must be true")
	}
	if LiveAutoMode(c) && (c.Live.Enabled || c.Execution.RealTradingEnabled) {
		if LiveAutoMaxNotionalUSDT(c) <= 0 {
			return errors.New("live live_auto_max_notional_usdt must be positive when live_auto_mode is enabled")
		}
		if LiveAutoMaxNotionalUSDT(c) > c.Live.MaxOrderNotionalUSDT {
			return fmt.Errorf("live live_auto_max_notional_usdt (%.2f) cannot exceed max_order_notional_usdt (%.2f)", LiveAutoMaxNotionalUSDT(c), c.Live.MaxOrderNotionalUSDT)
		}
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
	if !c.Risk.NoFutures || !c.Risk.NoLeverage || !c.Risk.SpotLimitOnly {
		return errors.New("risk flags must enforce no futures, no leverage, spot limit only")
	}
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
	if c.Data.BinanceBaseURL == "" || c.Data.Symbols.BTC == "" || len(c.Data.Symbols.Assets) == 0 || len(c.Data.Intervals) == 0 {
		return errors.New("data source/symbols/intervals required")
	}
	if c.Data.CandleLimit < 100 {
		return errors.New("data.candle_limit must be >=100")
	}
	if c.Maintenance.ReportRetentionDays < 0 || c.Maintenance.EventRetentionDays < 0 || c.Maintenance.MaxReportFiles < 0 || c.Maintenance.MaxClosedPaperOrders < 0 || c.Maintenance.MaxCandlesPerSymbolInterval < 0 || c.Maintenance.MaxAnalysisRows < 0 || c.Maintenance.MaxPlanRows < 0 {
		return errors.New("maintenance retention values cannot be negative")
	}
	if c.Maintenance.SchedulerTime != "" && !validClockTime(c.Maintenance.SchedulerTime) {
		return errors.New("maintenance.scheduler_time must be HH:MM")
	}
	if c.Research.BriefIntervalMinutes < 0 {
		return errors.New("research.brief_interval_minutes cannot be negative")
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
	if c.Storage.Path == "" {
		return errors.New("storage.path required")
	}
	return nil
}
