package simulator

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"btc-agent/internal/exchange/live"
)

const defaultMakerFeeRate = 0.0008

type FillAssumption struct {
	BestBid      float64 `json:"best_bid"`
	BestAsk      float64 `json:"best_ask"`
	FillQuantity float64 `json:"fill_quantity"`
	FillPrice    float64 `json:"fill_price"`
	FeeRate      float64 `json:"fee_rate"`
}

// FakeOKX is an in-memory OKX-compatible simulator for tests and backtest research.
type FakeOKX struct {
	mu          sync.Mutex
	filters     map[string]live.InstrumentFilter
	balances    map[string]float64
	orders      map[string]live.OrderStatus
	nextOrderID int
	Now         func() time.Time
}

func NewFakeOKX() *FakeOKX {
	return &FakeOKX{
		filters:  map[string]live.InstrumentFilter{},
		balances: map[string]float64{},
		orders:   map[string]live.OrderStatus{},
		Now:      time.Now,
	}
}

func (f *FakeOKX) SetFilter(instID string, filter live.InstrumentFilter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if filter.InstID == "" {
		filter.InstID = instID
	}
	f.filters[instID] = filter
}

func (f *FakeOKX) SetBalance(asset string, free float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.balances[strings.ToUpper(asset)] = free
}

func (f *FakeOKX) AccountBalance(ctx context.Context, ccy string) ([]live.Balance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.TrimSpace(ccy) != "" {
		asset := strings.ToUpper(ccy)
		return []live.Balance{{Asset: asset, Free: f.balances[asset]}}, nil
	}
	out := make([]live.Balance, 0, len(f.balances))
	for asset, free := range f.balances {
		out = append(out, live.Balance{Asset: asset, Free: free})
	}
	return out, nil
}

func (f *FakeOKX) InstrumentFilter(ctx context.Context, instID string) (live.InstrumentFilter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	filter, ok := f.filters[instID]
	if !ok {
		return live.InstrumentFilter{}, fmt.Errorf("simulator instrument not found: %s", instID)
	}
	return filter, nil
}

func (f *FakeOKX) PlaceSpotLimitOrder(ctx context.Context, req live.LimitOrderRequest) (live.OrderResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if req.ClientOrderID == "" {
		return live.OrderResult{}, fmt.Errorf("client order id required")
	}
	if _, exists := f.orders[req.ClientOrderID]; exists {
		return live.OrderResult{}, fmt.Errorf("duplicate client order id: %s", req.ClientOrderID)
	}
	if req.Price <= 0 || req.Quantity <= 0 {
		return live.OrderResult{}, fmt.Errorf("price and quantity must be positive")
	}
	if err := f.validateFilter(req); err != nil {
		return live.OrderResult{}, err
	}
	if err := f.reserveBalance(req); err != nil {
		return live.OrderResult{}, err
	}

	f.nextOrderID++
	orderID := fmt.Sprintf("SIM-%06d", f.nextOrderID)
	now := f.nowUnix()
	f.orders[req.ClientOrderID] = live.OrderStatus{
		InstID:        req.InstID,
		OrderID:       orderID,
		ClientOrderID: req.ClientOrderID,
		State:         "live",
		Status:        live.StatusSubmitted,
		Side:          req.Side,
		OrderType:     "limit",
		Price:         req.Price,
		Quantity:      req.Quantity,
		UpdatedAt:     now,
		SubmittedAt:   now,
	}
	return live.OrderResult{InstID: req.InstID, OrderID: orderID, ClientOrderID: req.ClientOrderID, Submitted: true}, nil
}

func (f *FakeOKX) CancelOrder(ctx context.Context, req live.CancelOrderRequest) (live.CancelOrderResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	order, key, ok := f.findOrder(req.InstID, req.OrderID, req.ClientOrderID)
	if !ok {
		return live.CancelOrderResult{}, fmt.Errorf("order not found")
	}
	if order.Status == live.StatusFilled || order.Status == live.StatusCancelled {
		return live.CancelOrderResult{}, fmt.Errorf("order already %s", order.Status)
	}
	f.releaseReserved(order)
	order.Status = live.StatusCancelled
	order.State = "canceled"
	order.UpdatedAt = f.nowUnix()
	f.orders[key] = order
	return live.CancelOrderResult{InstID: order.InstID, OrderID: order.OrderID, ClientOrderID: order.ClientOrderID, Canceled: true}, nil
}

func (f *FakeOKX) GetOrder(ctx context.Context, instID, ordID, clOrdID string) (live.OrderStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	order, _, ok := f.findOrder(instID, ordID, clOrdID)
	if !ok {
		return live.OrderStatus{}, fmt.Errorf("order not found")
	}
	return order, nil
}

func (f *FakeOKX) SimFill(clOrdID string, qty, price float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	order, ok := f.orders[clOrdID]
	if !ok {
		return fmt.Errorf("order not found")
	}
	return f.fillLocked(clOrdID, order, qty, price, defaultMakerFeeRate)
}

func (f *FakeOKX) SimulateFill(clOrdID string, assumption FillAssumption) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	order, ok := f.orders[clOrdID]
	if !ok {
		return fmt.Errorf("order not found")
	}
	if order.Status == live.StatusCancelled || order.Status == live.StatusFilled {
		return fmt.Errorf("cannot fill %s order", order.Status)
	}
	if !crossesBook(order, assumption) {
		return nil
	}
	qty := assumption.FillQuantity
	if qty <= 0 {
		qty = order.Quantity - order.FilledQuantity
	}
	price := assumption.FillPrice
	if price <= 0 {
		price = order.Price
	}
	feeRate := assumption.FeeRate
	if feeRate <= 0 {
		feeRate = defaultMakerFeeRate
	}
	return f.fillLocked(clOrdID, order, qty, price, feeRate)
}

func (f *FakeOKX) validateFilter(req live.LimitOrderRequest) error {
	filter, ok := f.filters[req.InstID]
	if !ok {
		return fmt.Errorf("simulator instrument not found: %s", req.InstID)
	}
	if filter.MinSize > 0 && req.Quantity < filter.MinSize {
		return fmt.Errorf("quantity %.8f below min size %.8f", req.Quantity, filter.MinSize)
	}
	if filter.MinNotional > 0 && req.Price*req.Quantity < filter.MinNotional {
		return fmt.Errorf("notional %.4f below min notional %.4f", req.Price*req.Quantity, filter.MinNotional)
	}
	if filter.TickSize > 0 && !multipleOf(req.Price, filter.TickSize) {
		return fmt.Errorf("price %.8f violates tick size %.8f", req.Price, filter.TickSize)
	}
	if filter.StepSize > 0 && !multipleOf(req.Quantity, filter.StepSize) {
		return fmt.Errorf("quantity %.8f violates step size %.8f", req.Quantity, filter.StepSize)
	}
	return nil
}

func (f *FakeOKX) reserveBalance(req live.LimitOrderRequest) error {
	asset := "USDT"
	cost := req.Price * req.Quantity
	if strings.EqualFold(req.Side, "buy") {
		if f.balances[asset] > 0 && f.balances[asset] < cost {
			return fmt.Errorf("insufficient %s balance", asset)
		}
		if f.balances[asset] > 0 {
			f.balances[asset] -= cost
		}
	}
	return nil
}

func (f *FakeOKX) releaseReserved(order live.OrderStatus) {
	if strings.EqualFold(order.Side, "buy") && order.Status != live.StatusFilled {
		remaining := (order.Quantity - order.FilledQuantity) * order.Price
		if remaining > 0 {
			f.balances["USDT"] += remaining
		}
	}
}

func (f *FakeOKX) fillLocked(key string, order live.OrderStatus, qty, price, feeRate float64) error {
	if order.Status == live.StatusCancelled || order.Status == live.StatusFilled {
		return fmt.Errorf("cannot fill %s order", order.Status)
	}
	remaining := order.Quantity - order.FilledQuantity
	if qty <= 0 {
		return fmt.Errorf("fill quantity must be positive")
	}
	if qty > remaining {
		qty = remaining
	}
	newFilled := order.FilledQuantity + qty
	if order.AvgPrice == 0 {
		order.AvgPrice = price
	} else {
		order.AvgPrice = ((order.AvgPrice * order.FilledQuantity) + (price * qty)) / newFilled
	}
	order.FilledQuantity = newFilled
	order.AccumulatedFillSz = newFilled
	order.Fee += qty * price * feeRate
	order.FeeCurrency = "USDT"
	order.UpdatedAt = f.nowUnix()
	if newFilled >= order.Quantity {
		order.Status = live.StatusFilled
		order.State = "filled"
	} else {
		order.Status = live.StatusPartialFill
		order.State = "partially_filled"
	}
	f.orders[key] = order
	return nil
}

func (f *FakeOKX) findOrder(instID, ordID, clOrdID string) (live.OrderStatus, string, bool) {
	if clOrdID != "" {
		order, ok := f.orders[clOrdID]
		return order, clOrdID, ok && (instID == "" || order.InstID == instID)
	}
	for key, order := range f.orders {
		if order.OrderID == ordID && (instID == "" || order.InstID == instID) {
			return order, key, true
		}
	}
	return live.OrderStatus{}, "", false
}

func (f *FakeOKX) nowUnix() int64 {
	if f.Now == nil {
		return time.Now().Unix()
	}
	return f.Now().Unix()
}

func crossesBook(order live.OrderStatus, assumption FillAssumption) bool {
	if strings.EqualFold(order.Side, "buy") {
		return assumption.BestAsk > 0 && order.Price >= assumption.BestAsk
	}
	return assumption.BestBid > 0 && order.Price <= assumption.BestBid
}

func multipleOf(value, step float64) bool {
	if step <= 0 {
		return true
	}
	q := value / step
	return math.Abs(q-math.Round(q)) < 1e-8
}
