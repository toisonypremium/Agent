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

func TestValidateFirstOrderQuarantineValid(t *testing.T) {
	cfg := validTestConfig()
	cfg.Live.Enabled = true
	cfg.Live.AutoExecute = true
	cfg.Live.RequireManualConfirm = false
	cfg.Live.ProofOnly = false
	cfg.Live.OrderManagementEnabled = true
	cfg.Live.MaxAutoLayersPerAsset = 2
	cfg.Live.MaxOpenLiveOrdersPerAsset = 2
	cfg.Live.MaxOpenLiveOrdersTotal = 6
	cfg.Live.MaxOrderNotionalUSDT = 100
	cfg.Live.MaxLiveNotionalPerOrderUSDT = 50
	cfg.Live.MaxLiveNotionalPerAssetUSDT = 100
	cfg.Live.MaxLiveNotionalTotalUSDT = 200
	cfg.Live.FirstOrderQuarantineEnabled = true
	cfg.Live.FirstOrderMaxNotionalUSDT = 25
	cfg.Execution.PaperTrading = false
	cfg.Execution.RealTradingEnabled = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateFirstOrderQuarantineRejectsInvalidValues(t *testing.T) {
	base := validTestConfig()
	base.Live.Enabled = true
	base.Live.AutoExecute = true
	base.Live.RequireManualConfirm = false
	base.Live.ProofOnly = false
	base.Live.OrderManagementEnabled = true
	base.Live.MaxAutoLayersPerAsset = 2
	base.Live.MaxOpenLiveOrdersPerAsset = 2
	base.Live.MaxOpenLiveOrdersTotal = 6
	base.Live.MaxOrderNotionalUSDT = 100
	base.Live.MaxLiveNotionalPerOrderUSDT = 50
	base.Live.MaxLiveNotionalPerAssetUSDT = 100
	base.Live.MaxLiveNotionalTotalUSDT = 200
	base.Live.FirstOrderQuarantineEnabled = true
	base.Execution.PaperTrading = false
	base.Execution.RealTradingEnabled = true
	for _, tc := range []struct {
		name string
		set  func(*Config)
	}{
		{"negative", func(cfg *Config) { cfg.Live.FirstOrderMaxNotionalUSDT = -1 }},
		{"above per order cap", func(cfg *Config) { cfg.Live.FirstOrderMaxNotionalUSDT = 51 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.set(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
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

func TestValidateMonitoringEnabledValid(t *testing.T) {
	cfg := validTestConfig()
	cfg.Monitoring.Enabled = true
	cfg.Monitoring.MarketScanIntervalMinutes = 15
	cfg.Monitoring.TelegramDigestIntervalMinutes = 60
	cfg.Monitoring.NotifyOnStateChange = true
	cfg.Monitoring.NotifyOnCritical = true
	cfg.Monitoring.CriticalRepeatMinutes = 60
	cfg.Monitoring.MaxConsecutiveScanErrors = 3
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateMonitoringRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		set  func(*Config)
	}{
		{"negative scan", func(cfg *Config) { cfg.Monitoring.MarketScanIntervalMinutes = -1 }},
		{"negative digest", func(cfg *Config) { cfg.Monitoring.TelegramDigestIntervalMinutes = -1 }},
		{"negative repeat", func(cfg *Config) { cfg.Monitoring.CriticalRepeatMinutes = -1 }},
		{"negative errors", func(cfg *Config) { cfg.Monitoring.MaxConsecutiveScanErrors = -1 }},
		{"short scan enabled", func(cfg *Config) {
			cfg.Monitoring.Enabled = true
			cfg.Monitoring.MarketScanIntervalMinutes = 4
			cfg.Monitoring.MaxConsecutiveScanErrors = 3
		}},
		{"short digest enabled", func(cfg *Config) {
			cfg.Monitoring.Enabled = true
			cfg.Monitoring.MarketScanIntervalMinutes = 15
			cfg.Monitoring.TelegramDigestIntervalMinutes = 10
			cfg.Monitoring.MaxConsecutiveScanErrors = 3
		}},
		{"short repeat enabled", func(cfg *Config) {
			cfg.Monitoring.Enabled = true
			cfg.Monitoring.MarketScanIntervalMinutes = 15
			cfg.Monitoring.CriticalRepeatMinutes = 10
			cfg.Monitoring.MaxConsecutiveScanErrors = 3
		}},
		{"zero max errors enabled", func(cfg *Config) {
			cfg.Monitoring.Enabled = true
			cfg.Monitoring.MarketScanIntervalMinutes = 15
			cfg.Monitoring.MaxConsecutiveScanErrors = 0
		}},
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

func TestValidateMicrostructureEnabledValid(t *testing.T) {
	cfg := validTestConfig()
	cfg.Microstructure.Enabled = true
	cfg.Microstructure.FetchOnMarketWatch = true
	cfg.Microstructure.RequireFreshForActive = true
	cfg.Microstructure.BinanceSpotBaseURL = "https://api.binance.com"
	cfg.Microstructure.BinanceFuturesBaseURL = "https://fapi.binance.com"
	cfg.Microstructure.Interval = "5m"
	cfg.Microstructure.LookbackLimit = 120
	cfg.Microstructure.MaxAgeMinutes = 30
	cfg.Microstructure.OrderBookDepthLimit = 100
	cfg.Microstructure.MinFreshSymbolsRequired = 4
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateMicrostructureRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		set  func(*Config)
	}{
		{"negative lookback", func(cfg *Config) { cfg.Microstructure.LookbackLimit = -1 }},
		{"negative max age", func(cfg *Config) { cfg.Microstructure.MaxAgeMinutes = -1 }},
		{"negative depth", func(cfg *Config) { cfg.Microstructure.OrderBookDepthLimit = -1 }},
		{"negative required", func(cfg *Config) { cfg.Microstructure.MinFreshSymbolsRequired = -1 }},
		{"missing spot", func(cfg *Config) {
			cfg.Microstructure.Enabled = true
			cfg.Microstructure.BinanceFuturesBaseURL = "https://fapi.binance.com"
		}},
		{"missing futures", func(cfg *Config) {
			cfg.Microstructure.Enabled = true
			cfg.Microstructure.BinanceSpotBaseURL = "https://api.binance.com"
		}},
		{"large lookback", func(cfg *Config) {
			cfg.Microstructure.Enabled = true
			cfg.Microstructure.BinanceSpotBaseURL = "https://api.binance.com"
			cfg.Microstructure.BinanceFuturesBaseURL = "https://fapi.binance.com"
			cfg.Microstructure.LookbackLimit = 1001
		}},
		{"large depth", func(cfg *Config) {
			cfg.Microstructure.Enabled = true
			cfg.Microstructure.BinanceSpotBaseURL = "https://api.binance.com"
			cfg.Microstructure.BinanceFuturesBaseURL = "https://fapi.binance.com"
			cfg.Microstructure.OrderBookDepthLimit = 1001
		}},
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
