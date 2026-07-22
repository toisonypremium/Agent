package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// HermesOperatorConfig controls LLM strategy proposal authority and exposure caps.
type HermesOperatorConfig struct {
	Enabled                  bool    `yaml:"enabled"`
	Mode                     string  `yaml:"mode"`
	DecisionTTLSeconds       int     `yaml:"decision_ttl_seconds"`
	MinConfidence            float64 `yaml:"min_confidence"`
	MaxActionsPerCycle       int     `yaml:"max_actions_per_cycle"`
	MaxProbeNotionalUSDT     float64 `yaml:"max_probe_notional_usdt"`
	MaxActionNotionalUSDT    float64 `yaml:"max_action_notional_usdt"`
	MaxPortfolioExposureUSDT float64 `yaml:"max_portfolio_exposure_usdt"`
}

func (c HermesOperatorConfig) NormalizedMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if mode == "" {
		return "observe"
	}
	return mode
}

func (c HermesOperatorConfig) CanExecute() bool {
	return c.Enabled && (c.NormalizedMode() == "canary" || c.NormalizedMode() == "autonomous")
}

type ExitConfig struct {
	Enabled               bool    `yaml:"enabled"`
	TakeProfitPct         float64 `yaml:"take_profit_pct"`
	PartialExitPct        float64 `yaml:"partial_exit_pct"`
	TrailingActivatePct   float64 `yaml:"trailing_activate_pct"`
	TrailingDistancePct   float64 `yaml:"trailing_distance_pct"`
	TimeStopDays          int     `yaml:"time_stop_days"`
	MinPnLForTimeStop     float64 `yaml:"min_pnl_for_time_stop"`
	PanicSellPnLThreshold float64 `yaml:"panic_sell_pnl_threshold"` // negative threshold e.g. -0.25 = sell all when loss >= 25%; 0 = disabled
}

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
		MaxTotalDeploymentPerCycle      float64 `yaml:"max_total_deployment_per_cycle"`
		MaxSingleAssetDeployment        float64 `yaml:"max_single_asset_deployment"`
		MinRewardRisk                   float64 `yaml:"min_reward_risk"`
		DecisionProfile                 string  `yaml:"decision_profile"`
		BTCTrendArmedThreshold          float64 `yaml:"btc_trend_armed_threshold"`
		BTCTrendAllowedThreshold        float64 `yaml:"btc_trend_allowed_threshold"`
		BTCFlowPromoteThreshold         float64 `yaml:"btc_flow_promote_threshold"`
		BTCPermissionMinRewardRisk      float64 `yaml:"btc_permission_min_reward_risk"`
		MinWatchReadinessForProbe       float64 `yaml:"min_watch_readiness_for_probe"`
		StopOnPanicSelling              bool    `yaml:"stop_on_panic_selling"`
		DisableRelativeStrengthFilter   bool    `yaml:"disable_relative_strength_filter"`
		RelativeStrengthLookbackDays    int     `yaml:"relative_strength_lookback_days"`
		MinRelativeStrength             float64 `yaml:"min_relative_strength"`
		MinAssetMomentum                float64 `yaml:"min_asset_momentum"`
		DisableRotationScoreFilter      bool    `yaml:"disable_rotation_score_filter"`
		MinRotationScore                float64 `yaml:"min_rotation_score"`
		MaxRotationRank                 int     `yaml:"max_rotation_rank"`
		DisableAssetFlowEntryFilter     bool    `yaml:"disable_asset_flow_entry_filter"`
		MinAssetFlowBullScore           float64 `yaml:"min_asset_flow_bull_score"`
		AllowNeutralReclaimEntry        bool    `yaml:"allow_neutral_reclaim_entry"`
		StrictAssetFlowEntry            bool    `yaml:"strict_asset_flow_entry"`
		FlowBearHardBlockScore          float64 `yaml:"flow_bear_hard_block_score"`
		MinScoutRewardRisk              float64 `yaml:"min_scout_reward_risk"`
		StrictRotationRank              bool    `yaml:"strict_rotation_rank"`
		AllowScoutInDowntrend           bool    `yaml:"allow_scout_in_downtrend"`
		ExceptionalRRBypassFallingKnife float64 `yaml:"exceptional_rr_bypass_falling_knife"` // 0=disabled; >0 = min RR to demote falling knife hard block to SCOUT
		DiscountZonePremiumPct          float64 `yaml:"discount_zone_premium_pct"`
		NoFutures                       bool    `yaml:"no_futures"`
		NoLeverage                      bool    `yaml:"no_leverage"`
		SpotLimitOnly                   bool    `yaml:"spot_limit_only"`
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
		Enabled                    bool    `yaml:"enabled"`
		Provider                   string  `yaml:"provider"`
		Model                      string  `yaml:"model"`
		BaseURLEnv                 string  `yaml:"base_url_env"`
		APIKeyEnv                  string  `yaml:"api_key_env"`
		MaxTokens                  int     `yaml:"max_tokens"`
		Temperature                float64 `yaml:"temperature"`
		TelegramEnabled            bool    `yaml:"telegram_enabled"`
		HermesIntervalMinutes      int     `yaml:"hermes_interval_minutes"`
		HermesFreshAuditMaxMinutes int     `yaml:"hermes_fresh_audit_max_minutes"`
		HermesMinAlertGapMinutes   int     `yaml:"hermes_min_alert_gap_minutes"`
		HermesEventDrivenEnabled   bool    `yaml:"hermes_event_driven_enabled"`
		HermesTelegramInteractive  bool    `yaml:"hermes_telegram_interactive"`
	} `yaml:"ai"`
	Research struct {
		Enabled               bool `yaml:"enabled"`
		BriefIntervalMinutes  int  `yaml:"brief_interval_minutes"`
		MaxSourcesPerCycle    int  `yaml:"max_sources_per_cycle"`
		RequestTimeoutSeconds int  `yaml:"request_timeout_seconds"`
		ExpertEnabled         bool `yaml:"expert_enabled"`
		ExpertIntervalMinutes int  `yaml:"expert_interval_minutes"`
		ExpertMaxSections     int  `yaml:"expert_max_sections"`
		ExpertMaxItems        int  `yaml:"expert_max_items"`
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
		FirstOrderQuarantineEnabled       bool    `yaml:"first_order_quarantine_enabled"`
		FirstOrderMaxNotionalUSDT         float64 `yaml:"first_order_max_notional_usdt"`
		FirstOrderRequireDryRun           bool    `yaml:"first_order_require_dry_run"`
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
		AuditIntervalMinutes              int     `yaml:"audit_interval_minutes"`
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
	HermesOperator HermesOperatorConfig `yaml:"hermes_operator"`
	Exit           ExitConfig           `yaml:"exit"`
	Execution      struct {
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
