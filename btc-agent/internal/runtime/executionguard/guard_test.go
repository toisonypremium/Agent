package executionguard

import (
	"context"
	"errors"
	"testing"
	"time"

	"btc-agent/internal/exchange/live"
	"btc-agent/internal/runtime/ownership"
)

type store struct{ lease ownership.Lease }

func (s *store) AcquireExecutionLease(context.Context, string, string, time.Time, time.Duration) (ownership.Lease, bool, error) {
	return s.lease, true, nil
}
func (s *store) RenewExecutionLease(context.Context, ownership.Lease, time.Time, time.Duration) (ownership.Lease, bool, error) {
	return s.lease, true, nil
}
func (s *store) ReleaseExecutionLease(context.Context, ownership.Lease) error { return nil }
func (s *store) CurrentExecutionLease(context.Context, string) (ownership.Lease, bool, error) {
	return s.lease, true, nil
}

type exchange struct{ placed, canceled int }

func (e *exchange) PlaceSpotLimitOrder(context.Context, live.LimitOrderRequest) (live.OrderResult, error) {
	e.placed++
	return live.OrderResult{Submitted: true}, nil
}
func (e *exchange) CancelOrder(context.Context, live.CancelOrderRequest) (live.CancelOrderResult, error) {
	e.canceled++
	return live.CancelOrderResult{}, nil
}
func TestGuardBlocksStaleFenceBeforeNetwork(t *testing.T) {
	now := time.Now().UTC()
	s := &store{lease: ownership.Lease{Name: "okx", InstanceID: "a", FencingToken: 2, ExpiresAt: now.Add(time.Minute)}}
	m, _ := ownership.NewManager(s, "okx", "a", time.Minute)
	e := &exchange{}
	g := GuardedExchange{Exchange: e, Manager: m, Lease: ownership.Lease{Name: "okx", InstanceID: "a", FencingToken: 1}}
	if _, err := g.PlaceSpotLimitOrder(context.Background(), live.LimitOrderRequest{}); !errors.Is(err, ownership.ErrNotOwner) {
		t.Fatalf("place err=%v", err)
	}
	if _, err := g.CancelOrder(context.Background(), live.CancelOrderRequest{}); !errors.Is(err, ownership.ErrNotOwner) {
		t.Fatalf("cancel err=%v", err)
	}
	if e.placed != 0 || e.canceled != 0 {
		t.Fatal("network exchange called under stale fence")
	}
}
func TestGuardAllowsCurrentOwner(t *testing.T) {
	now := time.Now().UTC()
	l := ownership.Lease{Name: "okx", InstanceID: "a", FencingToken: 2, ExpiresAt: now.Add(time.Minute)}
	s := &store{lease: l}
	m, _ := ownership.NewManager(s, "okx", "a", time.Minute)
	e := &exchange{}
	g := GuardedExchange{Exchange: e, Manager: m, Lease: l}
	if _, err := g.PlaceSpotLimitOrder(context.Background(), live.LimitOrderRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.CancelOrder(context.Background(), live.CancelOrderRequest{}); err != nil {
		t.Fatal(err)
	}
	if e.placed != 1 || e.canceled != 1 {
		t.Fatal("current owner not delegated")
	}
}
