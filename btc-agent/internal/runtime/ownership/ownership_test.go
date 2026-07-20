package ownership

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu    sync.Mutex
	lease Lease
	found bool
	fence int64
}

func (s *memoryStore) AcquireExecutionLease(_ context.Context, name, id string, now time.Time, ttl time.Duration) (Lease, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.found && s.lease.ExpiresAt.After(now) && s.lease.InstanceID != id {
		return Lease{}, false, nil
	}
	s.fence++
	s.lease = Lease{Name: name, InstanceID: id, FencingToken: s.fence, AcquiredAt: now, ExpiresAt: now.Add(ttl)}
	s.found = true
	return s.lease, true, nil
}
func (s *memoryStore) RenewExecutionLease(_ context.Context, l Lease, now time.Time, ttl time.Duration) (Lease, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.found || s.lease.InstanceID != l.InstanceID || s.lease.FencingToken != l.FencingToken {
		return Lease{}, false, nil
	}
	s.lease.ExpiresAt = now.Add(ttl)
	return s.lease, true, nil
}
func (s *memoryStore) ReleaseExecutionLease(_ context.Context, l Lease) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.found && s.lease.InstanceID == l.InstanceID && s.lease.FencingToken == l.FencingToken {
		s.found = false
	}
	return nil
}
func (s *memoryStore) CurrentExecutionLease(_ context.Context, _ string) (Lease, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lease, s.found, nil
}

func TestOnlyOneOwnerAndExpiredLeaseGetsHigherFence(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{}
	now := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	a, _ := NewManager(store, "okx-live", "a", time.Minute)
	b, _ := NewManager(store, "okx-live", "b", time.Minute)
	a.now = func() time.Time { return now }
	b.now = a.now
	first, err := a.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.Acquire(ctx); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("second owner err=%v", err)
	}
	now = now.Add(2 * time.Minute)
	second, err := b.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.FencingToken <= first.FencingToken {
		t.Fatalf("fence did not increase: %d <= %d", second.FencingToken, first.FencingToken)
	}
	if err = a.Verify(ctx, first); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("stale owner verified: %v", err)
	}
}
