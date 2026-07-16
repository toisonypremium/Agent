package microstructure

import "time"

const (
	StatusOK    = "MICROSTRUCTURE_OK"
	StatusWarn  = "MICROSTRUCTURE_WARN"
	StatusBlock = "MICROSTRUCTURE_BLOCK"
)

type Snapshot struct {
	Symbol    string             `json:"symbol"`
	Timestamp time.Time          `json:"timestamp"`
	Source    string             `json:"source"`
	SpotFlow  SpotFlow           `json:"spot_flow"`
	OrderBook OrderBook          `json:"order_book"`
	Futures   FuturesObservation `json:"futures"`
	Health    Health             `json:"health"`
	Signals   Signals            `json:"signals"`
}

type SpotFlow struct {
	VolumeBase         float64 `json:"volume_base"`
	QuoteVolumeUSDT    float64 `json:"quote_volume_usdt"`
	TakerBuyBase       float64 `json:"taker_buy_base"`
	TakerBuyQuoteUSDT  float64 `json:"taker_buy_quote_usdt"`
	TakerSellBase      float64 `json:"taker_sell_base"`
	TakerSellQuoteUSDT float64 `json:"taker_sell_quote_usdt"`
	TakerBuyRatio      float64 `json:"taker_buy_ratio"`
	CVDQuoteUSDT       float64 `json:"cvd_quote_usdt"`
}

type OrderBook struct {
	BestBid      float64 `json:"best_bid"`
	BestAsk      float64 `json:"best_ask"`
	SpreadBps    float64 `json:"spread_bps"`
	BidDepthUSDT float64 `json:"bid_depth_usdt"`
	AskDepthUSDT float64 `json:"ask_depth_usdt"`
	Imbalance    float64 `json:"imbalance"`
}

type FuturesObservation struct {
	OpenInterest float64 `json:"open_interest"`
	FundingRate  float64 `json:"funding_rate"`
	BasisPct     float64 `json:"basis_pct"`
}

type Health struct {
	Fresh    bool          `json:"fresh"`
	Age      time.Duration `json:"age"`
	MaxAge   time.Duration `json:"max_age"`
	Blockers []string      `json:"blockers,omitempty"`
	Warnings []string      `json:"warnings,omitempty"`
}

type Signals struct {
	BuyPressure    string   `json:"buy_pressure"`
	CVDTrend       string   `json:"cvd_trend"`
	OrderBookBias  string   `json:"orderbook_bias"`
	FundingBias    string   `json:"funding_bias"`
	BasisBias      string   `json:"basis_bias"`
	AbsorptionHint bool     `json:"absorption_hint"`
	Supportive     bool     `json:"supportive"`
	Risky          bool     `json:"risky"`
	Reasons        []string `json:"reasons,omitempty"`
}

type Summary struct {
	GeneratedAt   time.Time           `json:"generated_at"`
	Enabled       bool                `json:"enabled"`
	Status        string              `json:"status"`
	FreshSymbols  int                 `json:"fresh_symbols"`
	RequiredFresh int                 `json:"required_fresh"`
	Snapshots     []Snapshot          `json:"snapshots,omitempty"`
	BySymbol      map[string]Snapshot `json:"by_symbol,omitempty"`
	BTC           Snapshot            `json:"btc,omitempty"`
	Blockers      []string            `json:"blockers,omitempty"`
	Warnings      []string            `json:"warnings,omitempty"`
	Fingerprint   string              `json:"fingerprint"`
	Summary       string              `json:"summary"`
	MMFootprint         map[string]MMFootprintSignal `json:"mm_footprint,omitempty"`
}

type Report struct {
	GeneratedAt time.Time `json:"generated_at"`
	Summary     Summary   `json:"summary"`
}
