package config

import "testing"

func TestValidateManualLiveTradingRequiresManualConfirm(t *testing.T) {
	cfg := validTestConfig()
	cfg.Execution.RealTradingEnabled = true
	cfg.Execution.PaperTrading = false
	cfg.Live.Enabled = true
	cfg.Live.ProofOnly = false
	cfg.Live.RequireManualConfirm = false
	cfg.Live.AutoExecute = false
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected manual confirm validation error")
	}
}

func TestValidateAutoLiveTradingRequiresAutoConfirmShape(t *testing.T) {
	cfg := validTestConfig()
	cfg.Execution.RealTradingEnabled = true
	cfg.Execution.PaperTrading = false
	cfg.Live.Enabled = true
	cfg.Live.ProofOnly = false
	cfg.Live.RequireManualConfirm = false
	cfg.Live.AutoExecute = true
	cfg.Live.LiveAutoMode = true
	cfg.Live.LiveAutoMaxNotionalUSDT = 2
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateAutoLiveTradingRejectsManualConfirm(t *testing.T) {
	cfg := validTestConfig()
	cfg.Execution.RealTradingEnabled = true
	cfg.Execution.PaperTrading = false
	cfg.Live.Enabled = true
	cfg.Live.ProofOnly = false
	cfg.Live.RequireManualConfirm = true
	cfg.Live.AutoExecute = true
	cfg.Live.LiveAutoMode = true
	cfg.Live.LiveAutoMaxNotionalUSDT = 2
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected auto/manual confirm conflict")
	}
}

func TestValidateAutoLiveTradingDoesNotRequireLiveAutoMode(t *testing.T) {
	// Live auto mode is not required for auto live execution.
	// Auto live with live_auto_mode=false is now valid.
	cfg := validTestConfig()
	cfg.Execution.RealTradingEnabled = true
	cfg.Execution.PaperTrading = false
	cfg.Live.Enabled = true
	cfg.Live.ProofOnly = false
	cfg.Live.RequireManualConfirm = false
	cfg.Live.AutoExecute = true
	cfg.Live.LiveAutoMode = false
	if err := cfg.Validate(); err != nil {
		t.Fatalf("auto live without live_auto_mode should be valid: %v", err)
	}
}

func TestValidateLiveEnabledRejectsOversizedOrderCap(t *testing.T) {
	cfg := validTestConfig()
	cfg.Live.Enabled = true
	cfg.Live.MaxOrderNotionalUSDT = 10001
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected live max order cap validation error")
	}
}

func TestLiveAutoModeUsesOnlyCurrentFlag(t *testing.T) {
	cfg := validTestConfig()
	cfg.Live.Enabled = true
	cfg.Live.LiveAutoMode = true
	cfg.Live.LiveAutoMaxNotionalUSDT = 2.0
	cfg.Live.MaxOrderNotionalUSDT = 10.0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	cfg.Live.LiveAutoMode = false
	if LiveAutoMode(cfg) {
		t.Fatal("live-auto mode must follow only current flag")
	}
}

func TestValidateLiveAutoModeRejectsZeroOrNegativeMaxNotional(t *testing.T) {
	cfg := validTestConfig()
	cfg.Live.Enabled = true
	cfg.Live.LiveAutoMode = true

	cfg.Live.LiveAutoMaxNotionalUSDT = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for zero live auto max notional")
	}

	cfg.Live.LiveAutoMaxNotionalUSDT = -1.0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for negative live auto max notional")
	}
}

func TestValidateLiveAutoModeRejectsExceedingMaxOrderNotional(t *testing.T) {
	cfg := validTestConfig()
	cfg.Live.Enabled = true
	cfg.Live.LiveAutoMode = true
	cfg.Live.LiveAutoMaxNotionalUSDT = 15.0
	cfg.Live.MaxOrderNotionalUSDT = 10.0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error when live auto max exceeds max order notional")
	}
}

func TestValidateReconcileIntervalMinutes(t *testing.T) {
	cfg := validTestConfig()
	cfg.App.ReconcileIntervalMinutes = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for negative reconcile interval")
	}
	cfg.App.ReconcileIntervalMinutes = 15
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateDailyRunTime(t *testing.T) {
	cfg := validTestConfig()
	cfg.App.DailyRunTime = "08:00"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	cfg.App.DailyRunTime = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("empty daily run time should stay valid: %v", err)
	}

	for _, value := range []string{"invalid", "25:00", "08:99", "8", "8:00", "08:0"} {
		t.Run(value, func(t *testing.T) {
			cfg := validTestConfig()
			cfg.App.DailyRunTime = value
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateMaintenanceAllowsZeroDefaults(t *testing.T) {
	cfg := validTestConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateMaintenanceRejectsNegativeValues(t *testing.T) {
	cases := []struct {
		name string
		set  func(*Config)
	}{
		{"report retention", func(cfg *Config) { cfg.Maintenance.ReportRetentionDays = -1 }},
		{"event retention", func(cfg *Config) { cfg.Maintenance.EventRetentionDays = -1 }},
		{"max report files", func(cfg *Config) { cfg.Maintenance.MaxReportFiles = -1 }},
		{"max closed paper orders", func(cfg *Config) { cfg.Maintenance.MaxClosedPaperOrders = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validTestConfig()
			tc.set(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateMaintenanceSchedulerTime(t *testing.T) {
	cfg := validTestConfig()
	cfg.Maintenance.SchedulerEnabled = true
	cfg.Maintenance.SchedulerTime = "03:30"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	cfg.Maintenance.SchedulerTime = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("empty scheduler time should stay valid: %v", err)
	}

	for _, value := range []string{"invalid", "25:00", "08:99", "8", "8:00", "08:0"} {
		t.Run(value, func(t *testing.T) {
			cfg := validTestConfig()
			cfg.Maintenance.SchedulerTime = value
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateOrderManagementBounds(t *testing.T) {
	base := validTestConfig()
	base.Execution.RealTradingEnabled = true
	base.Execution.PaperTrading = false
	base.Live.Enabled = true
	base.Live.ProofOnly = false
	base.Live.RequireManualConfirm = false
	base.Live.AutoExecute = true
	base.Live.LiveAutoMode = true
	base.Live.LiveAutoMaxNotionalUSDT = 2
	base.Live.OrderManagementEnabled = true
	base.Live.MaxAutoLayersPerAsset = 3
	base.Live.MaxOpenLiveOrdersPerAsset = 3
	base.Live.MaxOpenLiveOrdersTotal = 9
	base.Live.MaxLiveNotionalPerOrderUSDT = 2
	base.Live.MaxLiveNotionalPerAssetUSDT = 6
	base.Live.MaxLiveNotionalTotalUSDT = 18
	if err := base.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	cases := []struct {
		name string
		set  func(*Config)
	}{
		{"zero layers", func(cfg *Config) { cfg.Live.MaxAutoLayersPerAsset = 0 }},
		{"asset cap below order", func(cfg *Config) { cfg.Live.MaxLiveNotionalPerAssetUSDT = 1 }},
		{"total cap below asset", func(cfg *Config) { cfg.Live.MaxLiveNotionalTotalUSDT = 5 }},
		{"negative cancel pct", func(cfg *Config) { cfg.Live.CancelIfPriceAboveDiscountZonePct = -1 }},
		{"negative stale", func(cfg *Config) { cfg.Live.CancelStaleAfterMinutes = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.set(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateSupervisorDefaultsDisabled(t *testing.T) {
	cfg := validTestConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateSupervisorRequiresManagedLiveShape(t *testing.T) {
	base := validTestConfig()
	base.Execution.RealTradingEnabled = true
	base.Execution.PaperTrading = false
	base.Live.Enabled = true
	base.Live.ProofOnly = false
	base.Live.RequireManualConfirm = false
	base.Live.AutoExecute = true
	base.Live.LiveAutoMode = true
	base.Live.LiveAutoMaxNotionalUSDT = 2
	base.Live.OrderManagementEnabled = true
	base.Live.MaxAutoLayersPerAsset = 3
	base.Live.MaxOpenLiveOrdersPerAsset = 3
	base.Live.MaxOpenLiveOrdersTotal = 9
	base.Live.MaxLiveNotionalPerOrderUSDT = 2
	base.Live.MaxLiveNotionalPerAssetUSDT = 6
	base.Live.MaxLiveNotionalTotalUSDT = 18
	base.Live.SupervisorEnabled = true
	base.Live.ManagementIntervalMinutes = 15
	base.Live.HeartbeatIntervalMinutes = 360
	base.Live.AutoHaltAfterErrors = 3
	if err := base.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	cases := []struct {
		name string
		set  func(*Config)
	}{
		{"live disabled", func(cfg *Config) { cfg.Live.Enabled = false }},
		{"auto disabled", func(cfg *Config) { cfg.Live.AutoExecute = false }},
		{"management disabled", func(cfg *Config) { cfg.Live.OrderManagementEnabled = false }},
		{"zero interval", func(cfg *Config) { cfg.Live.ManagementIntervalMinutes = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.set(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateSupervisorRejectsNegativeOptionalValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Config)
	}{
		{"heartbeat", func(cfg *Config) { cfg.Live.HeartbeatIntervalMinutes = -1 }},
		{"auto halt", func(cfg *Config) { cfg.Live.AutoHaltAfterErrors = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validTestConfig()
			tc.set(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateResearchDefaultsDisabled(t *testing.T) {
	cfg := validTestConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateResearchRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Config)
	}{
		{"negative interval", func(cfg *Config) { cfg.Research.BriefIntervalMinutes = -1 }},
		{"negative max sources", func(cfg *Config) { cfg.Research.MaxSourcesPerCycle = -1 }},
		{"zero timeout enabled", func(cfg *Config) { cfg.Research.Enabled = true; cfg.Research.RequestTimeoutSeconds = 0 }},
		{"rss enabled without feeds", func(cfg *Config) {
			cfg.Research.Enabled = true
			cfg.Research.RequestTimeoutSeconds = 1
			cfg.Research.RSS.Enabled = true
			cfg.Research.RSS.Feeds = nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validTestConfig()
			tc.set(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateResearchEnabledValid(t *testing.T) {
	cfg := validTestConfig()
	cfg.Research.Enabled = true
	cfg.Research.BriefIntervalMinutes = 360
	cfg.Research.MaxSourcesPerCycle = 20
	cfg.Research.RequestTimeoutSeconds = 12
	cfg.Research.RSS.Enabled = true
	cfg.Research.RSS.Feeds = []string{"https://example.com/rss"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateRequiresExactlyThreeAccumulationAssets(t *testing.T) {
	cfg := validTestConfig()
	cfg.Data.Symbols.Assets = []string{"ETHUSDT", "SOLUSDT"}
	delete(cfg.Portfolio.Allocation, "RENDERUSDT")
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected exactly 3 asset validation error")
	}
}

func TestValidateRejectsBTCAsAccumulationAsset(t *testing.T) {
	cfg := validTestConfig()
	cfg.Data.Symbols.Assets = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"}
	cfg.Portfolio.Allocation["BTCUSDT"] = 0.10
	delete(cfg.Portfolio.Allocation, "RENDERUSDT")
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected BTC asset validation error")
	}
}

func TestValidateRejectsAllocationOutsideAccumulationAssets(t *testing.T) {
	cfg := validTestConfig()
	cfg.Portfolio.Allocation["BNBUSDT"] = 0.01
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected extra allocation validation error")
	}
}

func TestValidateDynamicAssetAllocations(t *testing.T) {
	cfg := validTestConfig()
	cfg.Data.Symbols.Assets = []string{"ADAUSDT", "LINKUSDT", "AVAXUSDT"}
	cfg.Portfolio.Allocation = map[string]float64{"adausdt": 0.30, "LINKUSDT": 0.30, " AVAXUSDT ": 0.40}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected dynamic configured assets to validate: %v", err)
	}
}

func TestValidateRejectsMissingOrZeroConfiguredAssetAllocation(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Config)
	}{
		{"missing", func(cfg *Config) { delete(cfg.Portfolio.Allocation, "SOLUSDT") }},
		{"zero", func(cfg *Config) { cfg.Portfolio.Allocation["SOLUSDT"] = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validTestConfig()
			tc.set(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected allocation validation error")
			}
		})
	}
}

func TestValidateRejectsAllocationSumAboveOne(t *testing.T) {
	cfg := validTestConfig()
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": 0.50, "SOLUSDT": 0.30, "RENDERUSDT": 0.30}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected allocation sum validation error")
	}
}

func TestValidateRequiredBounds(t *testing.T) {
	cases := []struct {
		name string
		set  func(*Config)
	}{
		{"max total zero", func(cfg *Config) { cfg.Risk.MaxTotalDeploymentPerCycle = 0 }},
		{"max total above one", func(cfg *Config) { cfg.Risk.MaxTotalDeploymentPerCycle = 1.1 }},
		{"max single zero", func(cfg *Config) { cfg.Risk.MaxSingleAssetDeployment = 0 }},
		{"max single above one", func(cfg *Config) { cfg.Risk.MaxSingleAssetDeployment = 1.1 }},
		{"reserve negative", func(cfg *Config) { cfg.Portfolio.ReserveCashRatio = -0.01 }},
		{"reserve one", func(cfg *Config) { cfg.Portfolio.ReserveCashRatio = 1 }},
		{"expiry zero", func(cfg *Config) { cfg.Execution.OrderExpiryHours = 0 }},
		{"live min free negative", func(cfg *Config) { cfg.Live.MinAccountFreeUSDT = -1 }},
		{"relative lookback negative", func(cfg *Config) { cfg.Risk.RelativeStrengthLookbackDays = -1 }},
		{"rotation rank negative", func(cfg *Config) { cfg.Risk.MaxRotationRank = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validTestConfig()
			tc.set(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateDecisionThresholdBounds(t *testing.T) {
	cfg := validTestConfig()
	cfg.Risk.BTCTrendAllowedThreshold = 40
	cfg.Risk.BTCTrendArmedThreshold = 45
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected allowed threshold below armed threshold validation error")
	}
	cfg = validTestConfig()
	cfg.Risk.MinWatchReadinessForProbe = 1.1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected readiness probe threshold validation error")
	}
	cfg = validTestConfig()
	cfg.Risk.FlowBearHardBlockScore = 1.1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected flow bear hard-block threshold validation error")
	}
	cfg = validTestConfig()
	cfg.Risk.MinScoutRewardRisk = 4
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected scout RR above full RR validation error")
	}
}

func validTestConfig() Config {
	var cfg Config
	cfg.App.Mode = "live"
	cfg.Storage.Path = "data/test.db"
	cfg.Portfolio.BaseCurrency = "USDT"
	cfg.Portfolio.TotalCapital = 1000
	cfg.Portfolio.Allocation = map[string]float64{"ETHUSDT": 0.35, "SOLUSDT": 0.45, "RENDERUSDT": 0.20}
	cfg.Risk.NoFutures = true
	cfg.Risk.NoLeverage = true
	cfg.Risk.SpotLimitOnly = true
	cfg.Risk.MinRewardRisk = 3
	cfg.Risk.MaxTotalDeploymentPerCycle = 0.70
	cfg.Risk.MaxSingleAssetDeployment = 0.45
	cfg.Data.BinanceBaseURL = "https://api.binance.com"
	cfg.Data.Symbols.BTC = "BTCUSDT"
	cfg.Data.Symbols.Assets = []string{"ETHUSDT", "SOLUSDT", "RENDERUSDT"}
	cfg.Data.Intervals = []string{"1d"}
	cfg.Data.CandleLimit = 100
	cfg.Execution.PaperTrading = true
	cfg.Execution.OrderExpiryHours = 48
	cfg.Execution.LayerDistribution = []float64{1}
	cfg.Live.MaxOrderNotionalUSDT = 10
	return cfg
}
