package outbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

type store struct {
	items     []Item
	delivered int
	retries   int
	dead      bool
}

func (s *store) EnqueueOutbox(context.Context, Item) error                   { return nil }
func (s *store) ClaimOutbox(context.Context, time.Time, int) ([]Item, error) { return s.items, nil }
func (s *store) MarkOutboxDelivered(context.Context, string) error           { s.delivered++; return nil }
func (s *store) RetryOutbox(_ context.Context, _, _ string, _ time.Time, dead bool) error {
	s.retries++
	s.dead = dead
	return nil
}

type publisher struct{ err error }

func (p publisher) Publish(context.Context, Item) error { return p.err }
func TestWorkerDeliversAndRetriesWithoutFailingBatch(t *testing.T) {
	now := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	s := &store{items: []Item{{ID: "1", Destination: "ok"}, {ID: "2", Destination: "bad", RetryCount: 7}, {ID: "3", Destination: "missing"}}}
	w := Worker{Store: s, Publishers: map[string]Publisher{"ok": publisher{}, "bad": publisher{err: errors.New("down")}}, MaxRetries: 8, Now: func() time.Time { return now }}
	if err := w.RunOnce(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if s.delivered != 1 || s.retries != 2 || !s.dead {
		t.Fatalf("delivered=%d retries=%d dead=%v", s.delivered, s.retries, s.dead)
	}
}
