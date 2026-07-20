package outbox

import (
	"context"
	"errors"
	"time"
)

const (
	StatusPending    = "PENDING"
	StatusProcessing = "PROCESSING"
	StatusDelivered  = "DELIVERED"
	StatusDeadLetter = "DEAD_LETTER"
)

type Item struct {
	ID             string    `json:"id"`
	EventType      string    `json:"event_type"`
	Destination    string    `json:"destination"`
	Payload        []byte    `json:"payload"`
	IdempotencyKey string    `json:"idempotency_key"`
	Status         string    `json:"status"`
	RetryCount     int       `json:"retry_count"`
	NextRetryAt    time.Time `json:"next_retry_at"`
	LastError      string    `json:"last_error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type Store interface {
	EnqueueOutbox(ctx context.Context, item Item) error
	ClaimOutbox(ctx context.Context, now time.Time, limit int) ([]Item, error)
	MarkOutboxDelivered(ctx context.Context, id string) error
	RetryOutbox(ctx context.Context, id, reason string, next time.Time, dead bool) error
}

type Publisher interface {
	Publish(context.Context, Item) error
}

type Worker struct {
	Store      Store
	Publishers map[string]Publisher
	MaxRetries int
	Now        func() time.Time
}

func (w Worker) RunOnce(ctx context.Context, limit int) error {
	if w.Store == nil {
		return errors.New("outbox store required")
	}
	if w.Now == nil {
		w.Now = time.Now
	}
	if w.MaxRetries < 1 {
		w.MaxRetries = 8
	}
	items, err := w.Store.ClaimOutbox(ctx, w.Now().UTC(), limit)
	if err != nil {
		return err
	}
	for _, item := range items {
		publisher := w.Publishers[item.Destination]
		if publisher == nil {
			_ = w.Store.RetryOutbox(ctx, item.ID, "publisher unavailable", w.Now().UTC(), true)
			continue
		}
		if err := publisher.Publish(ctx, item); err != nil {
			dead := item.RetryCount+1 >= w.MaxRetries
			backoff := time.Minute * time.Duration(1<<min(item.RetryCount, 6))
			_ = w.Store.RetryOutbox(ctx, item.ID, err.Error(), w.Now().UTC().Add(backoff), dead)
			continue
		}
		_ = w.Store.MarkOutboxDelivered(ctx, item.ID)
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
