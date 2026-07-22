package hermesoperator

import "time"

type Intent string

const (
	IntentHold       Intent = "HOLD"
	IntentWatch      Intent = "WATCH"
	IntentProbeLimit Intent = "PROBE_LIMIT"
	IntentOpenLimit  Intent = "OPEN_LIMIT"
	IntentScaleLimit Intent = "SCALE_LIMIT"
	IntentCancel     Intent = "CANCEL"
	IntentReduce     Intent = "REDUCE"
	IntentExitLimit  Intent = "EXIT_LIMIT"
)

func (i Intent) IsKnown() bool {
	switch i {
	case IntentHold, IntentWatch, IntentProbeLimit, IntentOpenLimit, IntentScaleLimit, IntentCancel, IntentReduce, IntentExitLimit:
		return true
	default:
		return false
	}
}

func (i Intent) IncreasesExposure() bool {
	return i == IntentProbeLimit || i == IntentOpenLimit || i == IntentScaleLimit
}

func (i Intent) ReducesExposure() bool {
	return i == IntentCancel || i == IntentReduce || i == IntentExitLimit
}

type PortfolioRiskTier string

const (
	RiskBlocked    PortfolioRiskTier = "BLOCKED"
	RiskDefensive  PortfolioRiskTier = "DEFENSIVE"
	RiskNormal     PortfolioRiskTier = "NORMAL"
	RiskAggressive PortfolioRiskTier = "AGGRESSIVE"
)

type Decision struct {
	Version           int               `json:"version"`
	DecisionID        string            `json:"decision_id"`
	GeneratedAt       time.Time         `json:"generated_at"`
	ValidUntil        time.Time         `json:"valid_until"`
	MarketThesis      string            `json:"market_thesis"`
	PortfolioRiskTier PortfolioRiskTier `json:"portfolio_risk_tier"`
	Actions           []Action          `json:"actions"`
}

type Action struct {
	Symbol                string   `json:"symbol"`
	Intent                Intent   `json:"intent"`
	Confidence            float64  `json:"confidence"`
	EntryPrice            float64  `json:"entry_price,omitempty"`
	Invalidation          float64  `json:"invalidation,omitempty"`
	Target                float64  `json:"target,omitempty"`
	RequestedNotionalUSDT float64  `json:"requested_notional_usdt,omitempty"`
	MaxLayers             int      `json:"max_layers,omitempty"`
	ReasonCodes           []string `json:"reason_codes,omitempty"`
	CancelConditions      []string `json:"cancel_conditions,omitempty"`
}

type ValidationPolicy struct {
	Now                   time.Time
	MaxDecisionTTL        time.Duration
	MinConfidence         float64
	MaxActions            int
	MaxProbeNotionalUSDT  float64
	MaxActionNotionalUSDT float64
	AllowedSymbols        map[string]bool
}

type ValidationResult struct {
	Decision Decision `json:"decision"`
	Actions  []Action `json:"actions"`
	Reasons  []string `json:"reasons,omitempty"`
}
