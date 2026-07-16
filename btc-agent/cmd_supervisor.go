package main

import (
	"btc-agent/internal/config"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/market"
	"btc-agent/internal/storage"
	"btc-agent/internal/telegramreport"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

type liveSupervisorState struct {
	ConsecutiveErrors int
	LastHeartbeat     time.Time
	PeakTracker       *liveguard.PeakTracker
}

func runLiveSupervisorCycle(ctx context.Context, cfg config.Config, db *storage.DB, state *liveSupervisorState, dryRun bool) (liveguard.SupervisorResult, error) {
	doctor := buildLiveDoctorResult(ctx, cfg, db)
	if err := writeLiveDoctorResult(doctor); err != nil {
		log.Printf("live doctor report warning: %v", err)
	}
	return runLiveSupervisorCycleWithDoctor(ctx, cfg, db, state, dryRun, &doctor)
}

func runLiveSupervisorCycleWithDoctor(ctx context.Context, cfg config.Config, db *storage.DB, state *liveSupervisorState, dryRun bool, doctor *liveguard.RuntimeDoctorResult) (liveguard.SupervisorResult, error) {
	return runLiveSupervisorCycleWithDoctorNotify(ctx, cfg, db, state, dryRun, doctor, true)
}

func runLiveSupervisorCycleWithDoctorNotify(ctx context.Context, cfg config.Config, db *storage.DB, state *liveSupervisorState, dryRun bool, doctor *liveguard.RuntimeDoctorResult, notifyTelegram bool) (liveguard.SupervisorResult, error) {
	if state == nil {
		state = &liveSupervisorState{}
	}
	result := liveguard.SupervisorResult{GeneratedAt: time.Now(), Status: liveguard.SupervisorOK, Action: liveguard.SupervisorActionManagedCycle, ConsecutiveErrors: state.ConsecutiveErrors, Doctor: doctor}
	if doctor != nil && doctor.Status == liveguard.DoctorBlock && !dryRun {
		result.Action = liveguard.SupervisorActionReconcileOnly
		result.Reasons = append(result.Reasons, "live doctor block: "+doctor.Summary)
		if err := runReconcileLiveOrdersWithNotify(ctx, cfg, db, false); err != nil {
			state.ConsecutiveErrors++
			result.ConsecutiveErrors = state.ConsecutiveErrors
			result.Reasons = append(result.Reasons, "reconcile after doctor block: "+err.Error())
		}
		result.RefreshSummary()
		return result, writeLiveSupervisorResult(ctx, cfg, db, result, notifyTelegram && shouldNotifySupervisor(cfg, result, state))
	}
	if !cfg.Live.SupervisorEnabled {
		result.Action = liveguard.SupervisorActionSkipped
		result.Summary = "SUPERVISOR_OK: action=skipped | live supervisor disabled"
		result.RefreshSummary()
		return result, writeLiveSupervisorResult(ctx, cfg, db, result, false)
	}
	halted, err := db.IsHalted()
	if err != nil {
		state.ConsecutiveErrors++
		result.ConsecutiveErrors = state.ConsecutiveErrors
		result.Reasons = append(result.Reasons, "read operator halt: "+err.Error())
		result.RefreshSummary()
		return result, writeLiveSupervisorResult(ctx, cfg, db, result, notifyTelegram && shouldNotifySupervisor(cfg, result, state))
	}
	if halted {
		result.Action = liveguard.SupervisorActionReconcileOnly
		result.Reasons = append(result.Reasons, "operator halt active")
		if err := runReconcileLiveOrdersWithNotify(ctx, cfg, db, false); err != nil {
			state.ConsecutiveErrors++
			result.ConsecutiveErrors = state.ConsecutiveErrors
			result.Reasons = append(result.Reasons, "reconcile while halted: "+err.Error())
		} else {
			state.ConsecutiveErrors = 0
			result.ConsecutiveErrors = 0
		}
		result.RefreshSummary()
		return result, writeLiveSupervisorResult(ctx, cfg, db, result, notifyTelegram && shouldNotifySupervisor(cfg, result, state))
	}
	if dryRun {
		result.Action = liveguard.SupervisorActionHeartbeat
		if err := runAutoLiveOrderWithNotify(ctx, cfg, db, true, false); err != nil {
			state.ConsecutiveErrors++
			result.ConsecutiveErrors = state.ConsecutiveErrors
			result.Reasons = append(result.Reasons, "dry-run managed cycle: "+err.Error())
		} else {
			state.ConsecutiveErrors = 0
			result.ConsecutiveErrors = 0
			if managed, ok := loadLatestManagedCycleReport(); ok {
				result.Managed = &managed
			}
		}
	} else {
		if err := runAutoLiveOrderWithNotify(ctx, cfg, db, false, false); err != nil {
			state.ConsecutiveErrors++
			result.ConsecutiveErrors = state.ConsecutiveErrors
			result.Reasons = append(result.Reasons, "managed cycle: "+err.Error())
		} else {
			state.ConsecutiveErrors = 0
			result.ConsecutiveErrors = 0
			if managed, ok := loadLatestManagedCycleReport(); ok {
				result.Managed = &managed
			}
		}
	}
	// Evaluate exit conditions if exit manager is enabled.
	// EvaluateExits is report-only here — PlaceSellLimitOrder requires operator wire-up.
	if cfg.Exit.Enabled {
		if state.PeakTracker == nil {
			state.PeakTracker = liveguard.NewPeakTracker()
		}
		if positions, posErr := db.LivePositions(); posErr == nil && len(positions) > 0 {
			currentPrices := buildCurrentPricesFromDB(cfg, db)
			exits := liveguard.EvaluateExits(cfg, positions, currentPrices, state.PeakTracker)
			result.Exits = exits
			for _, ex := range exits {
				if ex.Action != liveguard.ExitHold {
					result.Reasons = append(result.Reasons, "exit signal: "+ex.Symbol+" → "+string(ex.Action)+": "+ex.Reason)
				}
			}
		}
	}

	if cfg.Live.AutoHaltAfterErrors > 0 && state.ConsecutiveErrors >= cfg.Live.AutoHaltAfterErrors {
		if err := db.SetHermesDemoted(true); err != nil {
			result.Reasons = append(result.Reasons, "Hermes circuit-breaker demotion failed: "+err.Error())
		} else {
			result.Reasons = append(result.Reasons, "Hermes circuit-breaker demoted after repeated supervisor errors")
		}
		if err := db.SetHaltStatus(true); err != nil {
			result.Reasons = append(result.Reasons, "auto-halt failed: "+err.Error())
		} else {
			result.AutoHalted = true
			result.Reasons = append(result.Reasons, "auto-halt activated after repeated supervisor errors")
		}
	}
	result.RefreshSummary()
	return result, writeLiveSupervisorResult(ctx, cfg, db, result, notifyTelegram && shouldNotifySupervisor(cfg, result, state))
}

func loadLatestManagedCycleReport() (liveguard.ManagedCycleResult, bool) {
	b, err := os.ReadFile(filepath.Join("reports", "auto_live_management_latest.json"))
	if err != nil {
		return liveguard.ManagedCycleResult{}, false
	}
	var result liveguard.ManagedCycleResult
	if err := json.Unmarshal(b, &result); err != nil {
		return liveguard.ManagedCycleResult{}, false
	}
	return result, true
}

func writeLiveSupervisorResult(ctx context.Context, cfg config.Config, db *storage.DB, result liveguard.SupervisorResult, notifyTelegram bool) error {
	if err := saveJSONFile("reports", "live_supervisor_latest.json", result); err != nil {
		return err
	}
	var scenario ScenarioReport
	scenarioOK := false
	if result.Managed == nil || !result.Managed.DryRun {
		if _, nextScenario, err := writeBotStateAndScenario(cfg, db, result); err != nil {
			log.Printf("bot state/scenario report warning: %v", err)
		} else {
			scenario = nextScenario
			scenarioOK = true
		}
	}
	md := liveSupervisorMarkdown(result)
	if scenarioOK {
		md += "\n" + scenarioMarkdown(scenario)
	}
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "live_supervisor_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && scenarioOK && shouldSendNearTriggerAlert(scenario) {
		sendScheduledTelegram(ctx, cfg, "near-trigger", nearTriggerTelegram(scenario))
		if err := saveTelegramScenarioState(scenario); err != nil {
			log.Printf("telegram scenario state warning: %v", err)
		}
	}
	saveLiveSupervisorRuntimeEvent(db, result)
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && notifyTelegram {
		text := telegramreport.LiveSupervisorHumanText(result)
		if scenarioOK {
			text = liveSupervisorScenarioTelegram(scenario, result)
		}
		sendScheduledTelegram(ctx, cfg, "live-supervisor", text)
	}
	fmt.Println(md)
	return nil
}

func liveSupervisorMarkdown(result liveguard.SupervisorResult) string {
	md := fmt.Sprintf("LIVE SUPERVISOR\n\nGenerated: %s\nStatus: %s\nAction: %s\nConsecutive errors: %d\nAuto halted: %v\nSummary: %s\n", result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Status, result.Action, result.ConsecutiveErrors, result.AutoHalted, result.Summary)
	if len(result.Reasons) > 0 {
		md += "Reasons:\n"
		for _, reason := range result.Reasons {
			md += "- " + reason + "\n"
		}
	}
	if result.Doctor != nil {
		md += fmt.Sprintf("\nDoctor: %s | %s\n", result.Doctor.Status, result.Doctor.Summary)
	}
	if result.Managed != nil {
		m := result.Managed
		md += fmt.Sprintf("\nManaged cycle: %s\nDesired: %d | Kept: %d | Canceled: %d | Replaced: %d | Placed: %d | Blocked: %d\n", m.Status, len(m.Desired), len(m.Kept), len(m.Canceled), len(m.Replaced), len(m.Placed), len(m.Blocked))
		if m.DataHealth.Status != "" {
			md += fmt.Sprintf("Data health: %s | %s\n", m.DataHealth.Status, m.DataHealth.Summary)
		}
		if m.ReconcileSafety.Status != "" {
			md += fmt.Sprintf("Reconcile safety: %s | %s\n", m.ReconcileSafety.Status, m.ReconcileSafety.Summary)
		}
		if m.RiskGovernor.Status != "" {
			md += fmt.Sprintf("Risk governor: %s | %s\n", m.RiskGovernor.Status, m.RiskGovernor.Summary)
		}
	}
	if len(result.Exits) > 0 {
		md += "\nExit Evaluation:\n"
		for _, ex := range result.Exits {
			if ex.Action == liveguard.ExitHold {
				continue
			}
			md += fmt.Sprintf("  %s → %s pnl=%.2f%% qty=%.6f price=%.4f: %s\n",
				ex.Symbol, ex.Action, ex.PnLPct*100, ex.SellQuantity, ex.SellPrice, ex.Reason)
		}
		md += "  NOTE: exit signals require operator review — PlaceSellLimitOrder not auto-called.\n"
	}
	md += "\nSafety: spot limit BUY post-only only; no futures, no leverage, no market order.\n"
	return md
}

func shouldNotifySupervisor(cfg config.Config, result liveguard.SupervisorResult, state *liveSupervisorState) bool {
	if result.AutoHalted || result.Status == liveguard.SupervisorHalted || result.Status == liveguard.SupervisorWarn {
		return true
	}
	if result.Managed != nil && (result.Managed.Status == liveguard.ManagedCycleBlocked || result.Managed.Status == liveguard.ManagedCyclePartial || len(result.Managed.Placed) > 0 || len(result.Managed.Canceled) > 0 || len(result.Managed.Replaced) > 0 || len(result.Managed.Blocked) > 0) {
		return true
	}
	if cfg.Live.NotifyOnNoAction {
		return true
	}
	if cfg.Live.HeartbeatIntervalMinutes <= 0 || state == nil {
		return false
	}
	now := result.GeneratedAt
	if state.LastHeartbeat.IsZero() || now.Sub(state.LastHeartbeat) >= time.Duration(cfg.Live.HeartbeatIntervalMinutes)*time.Minute {
		state.LastHeartbeat = now
		return true
	}
	return false
}

// buildCurrentPricesFromDB returns a symbol→lastClose price map from DB 4h candles.
// Used by exit evaluation. Returns empty map on error; EvaluateExits skips missing prices.
func buildCurrentPricesFromDB(cfg config.Config, db *storage.DB) map[string]float64 {
	prices := map[string]float64{}
	symbols := append([]string{cfg.Data.Symbols.BTC}, cfg.Data.Symbols.Assets...)
	for _, sym := range symbols {
		candles, err := db.LoadCandles(sym, "4h", 2)
		if err != nil || len(candles) == 0 {
			continue
		}
		if p := market.LastClose(candles); p > 0 {
			prices[sym] = p
		}
	}
	return prices
}
