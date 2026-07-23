package ownership

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrNotOwner = errors.New("execution ownership not held")

type Lease struct {
	Name         string    `json:"name"`
	InstanceID   string    `json:"instance_id"`
	FencingToken int64     `json:"fencing_token"`
	AcquiredAt   time.Time `json:"acquired_at"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type Store interface {
	AcquireExecutionLease(ctx context.Context, name, instanceID string, now time.Time, ttl time.Duration) (Lease, bool, error)
	RenewExecutionLease(ctx context.Context, lease Lease, now time.Time, ttl time.Duration) (Lease, bool, error)
	ReleaseExecutionLease(ctx context.Context, lease Lease) error
	CurrentExecutionLease(ctx context.Context, name string) (Lease, bool, error)
}

type Manager struct {
	store      Store
	name       string
	instanceID string
	ttl        time.Duration
	now        func() time.Time
}

func NewManager(store Store, name, instanceID string, ttl time.Duration) (*Manager, error) {
	if store == nil || name == "" || instanceID == "" || ttl <= 0 {
		return nil, errors.New("ownership store, name, instance ID and positive TTL required")
	}
	return &Manager{store: store, name: name, instanceID: instanceID, ttl: ttl, now: time.Now}, nil
}

func (m *Manager) Acquire(ctx context.Context) (Lease, error) {
	lease, ok, err := m.store.AcquireExecutionLease(ctx, m.name, m.instanceID, m.now().UTC(), m.ttl)
	if err != nil {
		return Lease{}, err
	}
	if !ok {
		return Lease{}, ErrNotOwner
	}
	return lease, nil
}

func (m *Manager) Renew(ctx context.Context, lease Lease) (Lease, error) {
	if err := m.Verify(ctx, lease); err != nil {
		return Lease{}, err
	}
	next, ok, err := m.store.RenewExecutionLease(ctx, lease, m.now().UTC(), m.ttl)
	if err != nil {
		return Lease{}, err
	}
	if !ok {
		return Lease{}, ErrNotOwner
	}
	return next, nil
}

func (m *Manager) Verify(ctx context.Context, lease Lease) error {
	current, ok, err := m.store.CurrentExecutionLease(ctx, m.name)
	if err != nil {
		return err
	}
	now := m.now().UTC()
	if !ok || lease.Name != m.name || lease.InstanceID != m.instanceID || lease.FencingToken <= 0 || current.InstanceID != m.instanceID || current.FencingToken != lease.FencingToken || !current.ExpiresAt.After(now) {
		return fmt.Errorf("%w: name=%s instance=%s fence=%d", ErrNotOwner, m.name, m.instanceID, lease.FencingToken)
	}
	return nil
}

func (m *Manager) Release(ctx context.Context, lease Lease) error {
	return m.store.ReleaseExecutionLease(ctx, lease)
}
