package live

// Package live intentionally contains read/proof types first.
// Real order placement is not implemented here; liveguard builds a candidate
// and reports readiness without calling an exchange order endpoint.

type Balance struct {
	Asset string  `json:"asset"`
	Free  float64 `json:"free"`
}

type InstrumentFilter struct {
	Symbol      string  `json:"symbol"`
	InstID      string  `json:"inst_id"`
	MinNotional float64 `json:"min_notional"`
	MinSize     float64 `json:"min_size"`
	TickSize    float64 `json:"tick_size"`
	StepSize    float64 `json:"step_size"`
}

type LimitOrderRequest struct {
	InstID        string  `json:"inst_id"`
	Side          string  `json:"side"`
	Price         float64 `json:"price"`
	Quantity      float64 `json:"quantity"`
	PostOnly      bool    `json:"post_only"`
	ClientOrderID string  `json:"client_order_id"`
}

type OrderResult struct {
	InstID        string `json:"inst_id"`
	OrderID       string `json:"order_id"`
	ClientOrderID string `json:"client_order_id"`
	Submitted     bool   `json:"submitted"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
}

const (
	StatusLiveOpen                = "LIVE_OPEN"
	StatusPartiallyFilled         = "PARTIALLY_FILLED"
	StatusFilled                  = "FILLED"
	StatusCanceled                = "CANCELED"
	StatusRejected                = "REJECTED"
	StatusUnknownNeedsManualCheck = "UNKNOWN_NEEDS_MANUAL_CHECK"
)

type OrderStatus struct {
	InstID            string  `json:"inst_id"`
	OrderID           string  `json:"order_id"`
	ClientOrderID     string  `json:"client_order_id"`
	State             string  `json:"state"` // OKX raw: live, partially_filled, filled, canceled
	Status            string  `json:"status"`
	Side              string  `json:"side"`
	OrderType         string  `json:"order_type"`
	Price             float64 `json:"price"`
	Quantity          float64 `json:"quantity"`
	FilledQuantity    float64 `json:"filled_quantity"`
	AvgPrice          float64 `json:"avg_price"`
	AccumulatedFillSz float64 `json:"accumulated_fill_sz"`
	Fee               float64 `json:"fee"`
	FeeCurrency       string  `json:"fee_currency"`
	UpdatedAt         int64   `json:"updated_at"`
}

type LiveFillSnapshot struct {
	ClientOrderID  string  `json:"client_order_id"`
	OrderID        string  `json:"order_id"`
	InstID         string  `json:"inst_id"`
	Symbol         string  `json:"symbol"`
	Side           string  `json:"side"`
	FilledQuantity float64 `json:"filled_quantity"`
	AvgPrice       float64 `json:"avg_price"`
	Fee            float64 `json:"fee"`
	FeeCurrency    string  `json:"fee_currency"`
	UpdatedAt      int64   `json:"updated_at"`
}

type LivePosition struct {
	Symbol        string  `json:"symbol"`
	InstID        string  `json:"inst_id"`
	Quantity      float64 `json:"quantity"`
	AvgEntryPrice float64 `json:"avg_entry_price"`
	CostBasis     float64 `json:"cost_basis"`
	FeeTotal      float64 `json:"fee_total"`
	FeeCurrency   string  `json:"fee_currency"`
	UpdatedAt     int64   `json:"updated_at"`
}

type LivePositionEvent struct {
	Timestamp     int64   `json:"timestamp"`
	ClientOrderID string  `json:"client_order_id"`
	OrderID       string  `json:"order_id"`
	InstID        string  `json:"inst_id"`
	Symbol        string  `json:"symbol"`
	Side          string  `json:"side"`
	DeltaQuantity float64 `json:"delta_quantity"`
	FillPrice     float64 `json:"fill_price"`
	NotionalDelta float64 `json:"notional_delta"`
	FeeDelta      float64 `json:"fee_delta"`
	FeeCurrency   string  `json:"fee_currency"`
	PositionQty   float64 `json:"position_qty"`
	AvgEntryPrice float64 `json:"avg_entry_price"`
	Status        string  `json:"status"`
	PayloadJSON   string  `json:"-"`
}
