package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/storage"
)

type canaryRehearsalReport struct {
	GeneratedAt       time.Time                        `json:"generated_at"`
	Status            string                           `json:"status"`
	NoRealExchange    bool                             `json:"no_real_exchange"`
	ForcedSimulation  liveguard.ForcedSimulationResult `json:"forced_simulation"`
	OrderStateMachine bool                             `json:"order_state_machine"`
	DuplicateBlocked  bool                             `json:"duplicate_blocked"`
	ReplayBlocked     bool                             `json:"replay_blocked"`
	RestartReplay     bool                             `json:"restart_replay_blocked"`
	Assertions        []string                         `json:"assertions"`
	Summary           string                           `json:"summary"`
}

func runCanaryRehearsal(ctx context.Context, cfg config.Config) error {
	_ = ctx
	report := canaryRehearsalReport{GeneratedAt: time.Now().UTC(), Status: "FAIL", NoRealExchange: true}
	report.ForcedSimulation = liveguard.RunForcedActiveLimitSimulation(cfg)
	if !report.ForcedSimulation.Passed || report.ForcedSimulation.ExchangeCalls != 0 {
		report.Assertions = append(report.Assertions, "forced simulation failed or attempted exchange call")
	}
	dir, err := os.MkdirTemp("", "btc-agent-canary-rehearsal-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "rehearsal.sqlite")
	db, err := storage.Open(path)
	if err != nil {
		return err
	}
	desired := liveguard.ManagedDesiredOrder{Symbol: "ETHUSDT", InstID: "ETH-USDT", LayerIndex: 1, Side: "BUY", Type: "limit", Price: 100, Quantity: .02, Notional: 2, PostOnly: true, Source: "CANARY_REHEARSAL", DecisionReason: "synthetic isolated rehearsal", DecisionID: "canary-rehearsal-decision", Intent: "PROBE_LIMIT"}
	clientID := "canary-rehearsal-order"
	if err := db.ReserveManagedLiveOrder(clientID, desired, "isolated rehearsal"); err != nil {
		report.Assertions = append(report.Assertions, err.Error())
	}
	if err := db.ReserveManagedLiveOrder(clientID, desired, "duplicate"); err != nil {
		report.DuplicateBlocked = true
	} else {
		report.Assertions = append(report.Assertions, "duplicate order reservation accepted")
	}
	sequence := []live.OrderStatus{
		{ClientOrderID: clientID, OrderID: "fake-order", Status: live.StatusSubmitted},
		{ClientOrderID: clientID, OrderID: "fake-order", Status: live.StatusPartialFill},
		{ClientOrderID: clientID, OrderID: "fake-order", Status: live.StatusFilled},
	}
	stateOK := true
	for _, state := range sequence {
		if err := db.SaveLiveOrderStatus(state); err != nil {
			stateOK = false
			report.Assertions = append(report.Assertions, err.Error())
			break
		}
	}
	if err := db.SaveLiveOrderStatus(live.OrderStatus{ClientOrderID: clientID, Status: live.StatusSubmitted}); err == nil {
		stateOK = false
		report.Assertions = append(report.Assertions, "terminal order downgraded to submitted")
	}
	report.OrderStateMachine = stateOK
	hash, _ := storage.HermesDecisionPayloadHash(map[string]any{"decision_id": "canary-rehearsal-receipt", "action": "PROBE_LIMIT"})
	if err := db.ReserveHermesExecution("canary-rehearsal-receipt", hash, time.Now()); err != nil {
		report.Assertions = append(report.Assertions, err.Error())
	}
	if err := db.ReserveHermesExecution("canary-rehearsal-receipt", hash, time.Now()); err != nil {
		report.ReplayBlocked = true
	} else {
		report.Assertions = append(report.Assertions, "Hermes replay accepted")
	}
	_ = db.Close()
	db, err = storage.Open(path)
	if err != nil {
		return err
	}
	if err := db.ReserveHermesExecution("canary-rehearsal-receipt", hash, time.Now()); err != nil {
		report.RestartReplay = true
	} else {
		report.Assertions = append(report.Assertions, "Hermes replay accepted after restart")
	}
	_ = db.Close()
	if report.ForcedSimulation.Passed && report.ForcedSimulation.ExchangeCalls == 0 && report.OrderStateMachine && report.DuplicateBlocked && report.ReplayBlocked && report.RestartReplay && len(report.Assertions) == 0 {
		report.Status = "PASS"
		report.Summary = "CANARY_REHEARSAL_PASS: isolated dry-run, no real exchange calls, state/replay invariants passed"
	} else {
		report.Summary = "CANARY_REHEARSAL_FAIL"
	}
	if err := saveJSONFile("reports", "canary_rehearsal_latest.json", report); err != nil {
		return err
	}
	fmt.Println(report.Summary)
	if report.Status != "PASS" {
		return errors.New(report.Summary)
	}
	return nil
}
