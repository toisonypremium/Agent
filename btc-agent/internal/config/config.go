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
		Mode         string `yaml:"mode"`
		Timezone     string `yaml:"timezone"`
		DailyRunTime string `yaml:"daily_run_time"`
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
			BTC    string   `yaml:"btc"`
			Assets []string `yaml:"assets"`
		} `yaml:"symbols"`
		Intervals   []string `yaml:"intervals"`
		CandleLimit int      `yaml:"candle_limit"`
	} `yaml:"data"`
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
	Live struct {
		Enabled              bool    `yaml:"enabled"`
		Exchange             string  `yaml:"exchange"`
		APIKeyEnv            string  `yaml:"api_key_env"`
		APISecretEnv         string  `yaml:"api_secret_env"`
		APIPassphraseEnv     string  `yaml:"api_passphrase_env"`
		MaxOrderNotionalUSDT float64 `yaml:"max_order_notional_usdt"`
		MinAccountFreeUSDT   float64 `yaml:"min_account_free_usdt"`
		RequirePostOnly      bool    `yaml:"require_post_only"`
		RequireManualConfirm bool    `yaml:"require_manual_confirm"`
		ProofOnly            bool    `yaml:"proof_only"`
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

func (c Config) Validate() error {
	if c.App.Mode == "" {
		return errors.New("app.mode required")
	}
	if c.App.Mode != "paper" && c.App.Mode != "report" && c.App.Mode != "live" {
		return errors.New("app.mode must be paper, report, or live")
	}
	if c.Execution.RealTradingEnabled {
		if !c.Live.Enabled || c.Live.ProofOnly || !c.Live.RequireManualConfirm {
			return errors.New("real trading requires live.enabled=true, live.proof_only=false, require_manual_confirm=true")
		}
		if c.Live.MaxOrderNotionalUSDT <= 0 || c.Live.MaxOrderNotionalUSDT > 10 {
			return errors.New("manual live proof max_order_notional_usdt must be >0 and <=10")
		}
	}
	if !c.Execution.PaperTrading && !c.Execution.RealTradingEnabled {
		return errors.New("paper_trading or real_trading_enabled must be true")
	}
	if !c.Risk.NoFutures || !c.Risk.NoLeverage || !c.Risk.SpotLimitOnly {
		return errors.New("risk flags must enforce no futures, no leverage, spot limit only")
	}
	if c.Portfolio.TotalCapital <= 0 {
		return errors.New("portfolio.total_capital must be positive")
	}
	if strings.ToUpper(c.Portfolio.BaseCurrency) != "USDT" {
		return errors.New("only USDT base currency supported")
	}
	for _, s := range []string{"ETHUSDT", "SOLUSDT", "RENDERUSDT"} {
		if c.Portfolio.Allocation[s] <= 0 {
			return fmt.Errorf("missing allocation for %s", s)
		}
	}
	sum := 0.0
	for _, v := range c.Portfolio.Allocation {
		if v < 0 {
			return errors.New("allocation cannot be negative")
		}
		sum += v
	}
	if sum > 1.000001 {
		return errors.New("allocation sum must be <=1")
	}
	if c.Risk.MinRewardRisk <= 0 {
		return errors.New("risk.min_reward_risk must be positive")
	}
	if c.Data.BinanceBaseURL == "" || c.Data.Symbols.BTC == "" || len(c.Data.Symbols.Assets) == 0 || len(c.Data.Intervals) == 0 {
		return errors.New("data source/symbols/intervals required")
	}
	if c.Data.CandleLimit < 100 {
		return errors.New("data.candle_limit must be >=100")
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
