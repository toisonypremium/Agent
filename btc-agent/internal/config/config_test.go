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
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected auto/manual confirm conflict")
	}
}

func TestValidateCanaryModeValid(t *testing.T) {
	cfg := validTestConfig()
	cfg.Live.Enabled = true
	cfg.Live.CanaryMode = true
	cfg.Live.CanaryMaxNotionalUSDT = 2.0
	cfg.Live.MaxOrderNotionalUSDT = 10.0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidateCanaryModeRejectsZeroOrNegativeMaxNotional(t *testing.T) {
	cfg := validTestConfig()
	cfg.Live.Enabled = true
	cfg.Live.CanaryMode = true

	cfg.Live.CanaryMaxNotionalUSDT = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for zero canary max notional")
	}

	cfg.Live.CanaryMaxNotionalUSDT = -1.0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for negative canary max notional")
	}
}

func TestValidateCanaryModeRejectsExceedingMaxOrderNotional(t *testing.T) {
	cfg := validTestConfig()
	cfg.Live.Enabled = true
	cfg.Live.CanaryMode = true
	cfg.Live.CanaryMaxNotionalUSDT = 15.0
	cfg.Live.MaxOrderNotionalUSDT = 10.0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error when canary max exceeds max order notional")
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
