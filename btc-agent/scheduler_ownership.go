package main

import (
	"btc-agent/internal/runtime/ownership"
	"btc-agent/internal/storage"
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

type schedulerOwnership struct {
	manager *ownership.Manager
	mu      sync.RWMutex
	lease   ownership.Lease
}

func startSchedulerOwnership(parent context.Context, db *storage.DB) (*schedulerOwnership, context.Context, context.CancelFunc, error) {
	instance := os.Getenv("BTC_AGENT_INSTANCE_ID")
	if instance == "" {
		instance = fmt.Sprintf("pid-%d", os.Getpid())
	}
	manager, err := ownership.NewManager(db, "okx-live", instance, 90*time.Second)
	if err != nil {
		return nil, nil, func() {}, err
	}
	lease, err := manager.Acquire(parent)
	if err != nil {
		return nil, nil, func() {}, fmt.Errorf("scheduler execution ownership required: %w", err)
	}
	ctx, cancel := context.WithCancel(parent)
	controller := &schedulerOwnership{manager: manager, lease: lease}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				controller.mu.RLock()
				current := controller.lease
				controller.mu.RUnlock()
				_ = manager.Release(context.Background(), current)
				return
			case now := <-ticker.C:
				controller.mu.RLock()
				current := controller.lease
				controller.mu.RUnlock()
				next, err := manager.Renew(ctx, current)
				if err != nil {
					log.Printf("[Scheduler] FATAL execution lease renewal failed at %s: %v", now.UTC().Format(time.RFC3339), err)
					cancel()
					return
				}
				controller.mu.Lock()
				controller.lease = next
				controller.mu.Unlock()
				log.Printf("[Scheduler] Execution lease renewed fence=%d expires=%s", next.FencingToken, next.ExpiresAt.Format(time.RFC3339))
			}
		}
	}()
	return controller, ctx, func() { cancel(); <-done }, nil
}
func (s *schedulerOwnership) snapshot() ownership.Lease {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lease
}
