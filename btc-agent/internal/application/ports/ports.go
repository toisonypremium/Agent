package ports

import (
	"context"
	"io"
	"time"
)

type Balance struct {
	Currency         string
	Available, Total float64
	CapturedAt       time.Time
}
type Position struct {
	Symbol                 string
	Quantity, AverageEntry float64
	ReconciledAt           time.Time
}
type Order struct {
	ExchangeOrderID, ClientOrderID, Symbol, Status string
	UpdatedAt                                      time.Time
}
type Fill struct {
	ExchangeFillID, ExchangeOrderID, Symbol string
	Quantity, Price, Fee                    float64
	FilledAt                                time.Time
}
type OrderRequest struct {
	ClientOrderID, IdempotencyKey, CorrelationID, DecisionID, PlanID, InstanceID, Symbol, Side string
	Price, Quantity                                                                            float64
	FencingToken                                                                               int64
}
type OrderResult struct{ ExchangeOrderID, ClientOrderID, Status string }

type Exchange interface {
	GetBalances(context.Context) ([]Balance, error)
	GetPositions(context.Context) ([]Position, error)
	GetOpenOrders(context.Context) ([]Order, error)
	GetFills(context.Context, time.Time) ([]Fill, error)
	SubmitOrder(context.Context, OrderRequest) (OrderResult, error)
	CancelOrder(context.Context, string) error
}

type Event struct {
	ID, Type, CorrelationID, IdempotencyKey string
	Payload                                 []byte
	CreatedAt                               time.Time
}
type EventPublisher interface {
	Publish(context.Context, Event) error
}
type ObjectStorage interface {
	Put(context.Context, string, string, io.Reader, int64, string) error
	Delete(context.Context, string) error
}
