package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/runtime/executionguard"
	"btc-agent/internal/runtime/ownership"
	"btc-agent/internal/storage"
)

func guardedLiveExchange(ctx context.Context, cfg config.Config, db *storage.DB, client *live.OKXClient) (liveguard.OrderPlacer, liveguard.OrderCanceler, error) {
	if client == nil {
		return nil, nil, fmt.Errorf("okx client required")
	}
	instance := os.Getenv("BTC_AGENT_INSTANCE_ID")
	if instance == "" {
		instance = fmt.Sprintf("pid-%d", os.Getpid())
	}
	manager, err := ownership.NewManager(db, "okx-live", instance, 90*time.Second)
	if err != nil {
		return nil, nil, err
	}
	lease, err := manager.Acquire(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("execution ownership required: %w", err)
	}
	guard := executionguard.GuardedExchange{Exchange: client, Manager: manager, Lease: lease}
	return guard, guard, nil
}
