package live

// Package live intentionally contains read/proof types first.
// Real order placement is not implemented here; liveguard builds a candidate
// and reports readiness without calling an exchange order endpoint.

type TradeFill struct {
	InstID      string  `json:"inst_id"`
	TradeID     string  `json:"trade_id"`
	OrderID     string  `json:"order_id"`
	Side        string  `json:"side"`
	Price       float64 `json:"price"`
	Quantity    float64 `json:"quantity"`
	Fee         float64 `json:"fee"`
	FeeCurrency string  `json:"fee_currency"`
	Timestamp   int64   `json:"timestamp"`
}

type InventoryBasis struct {
	Quantity float64
	Cost     float64
	AvgPrice float64
	Complete bool
}

type Balance struct {
	Asset    string  `json:"asset"`
	Free     float64 `json:"free"`
	Total    float64 `json:"total"`
	Equity   float64 `json:"equity,omitempty"`
	AvgPrice float64 `json:"avg_price,omitempty"`
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

type CancelOrderRequest struct {
	InstID        string `json:"inst_id"`
	OrderID       string `json:"order_id"`
	ClientOrderID string `json:"client_order_id"`
}

type CancelOrderResult struct {
	InstID        string `json:"inst_id"`
	OrderID       string `json:"order_id"`
	ClientOrderID string `json:"client_order_id"`
	Canceled      bool   `json:"canceled"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
}

const (
	StatusPlanned                 = "PLANNED"
	StatusSubmitted               = "SUBMITTED"
	StatusPartialFill             = "PARTIAL_FILL"
	StatusFilled                  = "FILLED"
	StatusCancelled               = "CANCELLED"
	StatusExpired                 = "EXPIRED"
	StatusRejected                = "REJECTED"
	StatusUnknownNeedsManualCheck = "UNKNOWN_NEEDS_MANUAL_CHECK"

	// Legacy status names kept for existing DB rows and exchange compatibility.
	StatusLiveOpen        = "LIVE_OPEN"
	StatusPartiallyFilled = "PARTIALLY_FILLED"
	StatusCanceled        = "CANCELED"
)

func NormalizeOrderStatus(status string) string {
	switch status {
	case StatusLiveOpen:
		return StatusSubmitted
	case StatusPartiallyFilled:
		return StatusPartialFill
	case StatusCanceled:
		return StatusCancelled
	default:
		return status
	}
}

func IsOpenStatus(status string) bool {
	switch NormalizeOrderStatus(status) {
	case StatusPlanned, StatusSubmitted, StatusPartialFill:
		return true
	default:
		return false
	}
}

type OrderStatus struct {
	InstID               string  `json:"inst_id"`
	OrderID              string  `json:"order_id"`
	ClientOrderID        string  `json:"client_order_id"`
	State                string  `json:"state"` // OKX raw: live, partially_filled, filled, canceled
	Status               string  `json:"status"`
	Side                 string  `json:"side"`
	OrderType            string  `json:"order_type"`
	Price                float64 `json:"price"`
	Quantity             float64 `json:"quantity"`
	FilledQuantity       float64 `json:"filled_quantity"`
	AvgPrice             float64 `json:"avg_price"`
	AccumulatedFillSz    float64 `json:"accumulated_fill_sz"`
	Fee                  float64 `json:"fee"`
	FeeCurrency          string  `json:"fee_currency"`
	UpdatedAt            int64   `json:"updated_at"`
	Symbol               string  `json:"symbol,omitempty"`
	Notional             float64 `json:"notional,omitempty"`
	LayerIndex           int     `json:"layer_index,omitempty"`
	Source               string  `json:"source,omitempty"`
	InvalidationPrice    float64 `json:"invalidation_price,omitempty"`
	ExpiresAt            int64   `json:"expires_at,omitempty"`
	DecisionReason       string  `json:"decision_reason,omitempty"`
	LastManagementAction string  `json:"last_management_action,omitempty"`
	SubmittedAt          int64   `json:"submitted_at,omitempty"`
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
	OpenedAt      int64   `json:"opened_at,omitempty"`
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
