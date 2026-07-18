package main

import (
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/hermesoperator"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/market"
	"btc-agent/internal/storage"
	"btc-agent/internal/telegramreport"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type liveSupervisorState struct {
	ConsecutiveErrors int
	LastHeartbeat     time.Time
	PeakTracker       *liveguard.PeakTracker
}

func attachPortfolioRiskTelemetry(cfg config.Config, db *storage.DB, result *liveguard.SupervisorResult, now time.Time) {
	if result == nil || db == nil {
		return
	}
	t := liveguard.PortfolioRiskTelemetry{}
	eq, err := db.EquityRiskState()
	if err != nil {
		t.Reason = "không đọc được trạng thái vốn"
		result.PortfolioRisk = t
		return
	}
	t.Known = true
	t.UpdatedAt = eq.UpdatedAt
	t.DrawdownPct = eq.DrawdownPct
	t.DrawdownLockActive = cfg.Risk.MaxTotalEquityDrawdownPct > 0 && eq.DrawdownPct >= cfg.Risk.MaxTotalEquityDrawdownPct
	if cfg.Risk.MaxDailyRealizedLossPct <= 0 {
		result.PortfolioRisk = t
		return
	}
	dayStart := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	daily, dailyErr := db.PortfolioRealizedPnLSnapshot(dayStart)
	basis, basisErr := db.DailyOpeningEquity(now, eq.CurrentEquity)
	if dailyErr != nil || basisErr != nil {
		t.Known = false
		t.Reason = "không đọc được PnL ngày hoặc vốn mở ngày"
		result.PortfolioRisk = t
		return
	}
	t.DailyRealizedPnL = daily.RealizedPnL
	t.DailyLossEquityBasis = basis.Equity
	t.DailyLossLockActive = daily.RealizedPnL <= -(basis.Equity * cfg.Risk.MaxDailyRealizedLossPct)
	result.PortfolioRisk = t
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
	if doctor == nil && !dryRun {
		result.Action = liveguard.SupervisorActionReconcileOnly
		result.Reasons = append(result.Reasons, "live doctor unavailable; supervisor fail-closed")
		if err := runReconcileLiveOrdersWithNotify(ctx, cfg, db, false); err != nil {
			result.Reasons = append(result.Reasons, "reconcile after missing doctor: "+err.Error())
		}
		result.RefreshSummary()
		attachPortfolioRiskTelemetry(cfg, db, &result, time.Now())
		return result, writeLiveSupervisorResult(ctx, cfg, db, result, false)
	}
	if doctor != nil && doctor.Status == liveguard.DoctorBlock && !dryRun {
		result.Action = liveguard.SupervisorActionReconcileOnly
		result.Reasons = append(result.Reasons, "live doctor block: "+doctor.Summary)
		if err := runReconcileLiveOrdersWithNotify(ctx, cfg, db, false); err != nil {
			state.ConsecutiveErrors++
			result.ConsecutiveErrors = state.ConsecutiveErrors
			result.Reasons = append(result.Reasons, "reconcile after doctor block: "+err.Error())
		}
		result.RefreshSummary()
		attachPortfolioRiskTelemetry(cfg, db, &result, time.Now())
		return result, writeLiveSupervisorResult(ctx, cfg, db, result, notifyTelegram && shouldNotifySupervisor(cfg, result, state))
	}
	if !cfg.Live.SupervisorEnabled {
		result.Action = liveguard.SupervisorActionSkipped
		result.Summary = "SUPERVISOR_OK: action=skipped | live supervisor disabled"
		result.RefreshSummary()
		attachPortfolioRiskTelemetry(cfg, db, &result, time.Now())
		return result, writeLiveSupervisorResult(ctx, cfg, db, result, false)
	}
	halted, err := db.IsHalted()
	if err != nil {
		state.ConsecutiveErrors++
		result.ConsecutiveErrors = state.ConsecutiveErrors
		result.Reasons = append(result.Reasons, "read operator halt: "+err.Error())
		result.RefreshSummary()
		attachPortfolioRiskTelemetry(cfg, db, &result, time.Now())
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
		attachPortfolioRiskTelemetry(cfg, db, &result, time.Now())
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
	if cfg.HermesOperator.CanExecute() && !halted {
		if adopted, updated, dust, e := syncHermesManagedPortfolio(ctx, cfg, db); e != nil {
			result.Reasons = append(result.Reasons, "portfolio sync: "+e.Error())
		} else if adopted+updated > 0 {
			result.Reasons = append(result.Reasons, fmt.Sprintf("portfolio sync: adopted=%d updated=%d dust=%d", adopted, updated, dust))
		}
	}
	if e := updatePersistedTotalEquity(ctx, cfg, db); e != nil {
		result.Reasons = append(result.Reasons, "persist total equity: "+e.Error())
	}
	if positions, e := db.LivePositions(); e == nil {
		exposure := 0.0
		for _, p := range positions {
			exposure += math.Max(0, p.CostBasis)
		}
		openBuy := 0.0
		for _, o := range currentOpenOrders(db) {
			if strings.EqualFold(o.Side, "BUY") {
				openBuy += math.Max(o.Notional, o.Price*o.Quantity)
			}
		}
		totalCapital := cfg.Portfolio.TotalCapital
		if eq, ee := db.EquityRiskState(); ee == nil && eq.CurrentEquity > 0 {
			totalCapital = eq.CurrentEquity
		}
		util := liveguard.EvaluateCapitalUtilization(liveguard.CapitalUtilizationInput{TotalCapital: totalCapital, ExistingExposure: exposure, OpenBuyNotional: openBuy, ReserveCashRatio: cfg.Portfolio.ReserveCashRatio, HardExposureCap: config.EffectiveHermesPortfolioExposure(cfg), MarketRegime: latestMarketRegime(db), AccumulationPhase: latestAccumulationPhase(db)})
		if b, je := json.Marshal(util); je == nil {
			_, _ = db.Exec(`INSERT INTO hermes_runtime_state(key,updated_at,payload_json) VALUES('capital_utilization',?,?) ON CONFLICT(key) DO UPDATE SET updated_at=excluded.updated_at,payload_json=excluded.payload_json`, time.Now().Unix(), string(b))
		}
	}
	telemetry, telemetryErr := collectExecutionTelemetry(ctx, cfg, db, time.Now())
	if telemetryErr != nil {
		result.Reasons = append(result.Reasons, "execution telemetry: "+telemetryErr.Error())
	} else if telemetry.Status != "TELEMETRY_OK" {
		result.Reasons = append(result.Reasons, telemetry.Summary)
	}
	if measured, e := db.UpdateExecutionMarkouts(time.Now()); e != nil {
		result.Reasons = append(result.Reasons, "execution markouts: "+e.Error())
	} else if measured > 0 {
		result.Reasons = append(result.Reasons, fmt.Sprintf("execution markouts measured=%d", measured))
	}
	if e := saveExecutionEvidenceReport(db, time.Now()); e != nil {
		result.Reasons = append(result.Reasons, "execution evidence: "+e.Error())
	}
	// Evaluate profit exits and loss warnings after the managed cycle.
	// Loss warnings never grant SELL authority; final execution blocks below-cost sells.
	if cfg.Exit.Enabled {
		if state.PeakTracker == nil {
			state.PeakTracker = loadExitPeakTracker()
			if persisted, e := db.ExitPeakStates(); e == nil {
				for _, item := range persisted {
					state.PeakTracker.PeakBySymbol[item.Symbol] = item.Peak
					state.PeakTracker.TrailActive[item.Symbol] = item.TrailActive
				}
			}
		}
		positions, posErr := db.LivePositions()
		if posErr == nil {
			activeSymbols := map[string]bool{}
			for _, p := range positions {
				if p.Quantity > 0 {
					activeSymbols[strings.ToUpper(p.Symbol)] = true
				}
			}
			for sym := range state.PeakTracker.PeakBySymbol {
				if !activeSymbols[sym] {
					delete(state.PeakTracker.PeakBySymbol, sym)
					delete(state.PeakTracker.TrailActive, sym)
				}
			}
		}
		if posErr == nil && len(positions) > 0 {
			currentPrices := buildCurrentPricesFromDB(cfg, db)
			if client, e := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv); e == nil {
				for _, p := range positions {
					sym := strings.ToUpper(p.Symbol)
					if currentPrices[sym] <= 0 {
						if price, pe := client.SpotLastPrice(ctx, sym); pe == nil {
							currentPrices[sym] = price
						} else {
							result.Reasons = append(result.Reasons, "managed holding price unavailable: "+sym)
						}
					}
				}
			}
			exits := liveguard.EvaluateExits(cfg, positions, currentPrices, state.PeakTracker)
			result.Exits = exits
			if err := saveExitPeakTracker(state.PeakTracker); err != nil {
				result.Reasons = append(result.Reasons, "persist exit peak tracker: "+err.Error())
			}
			peakStates := []storage.ExitPeakState{}
			for sym, peak := range state.PeakTracker.PeakBySymbol {
				peakStates = append(peakStates, storage.ExitPeakState{Symbol: sym, Peak: peak, TrailActive: state.PeakTracker.TrailActive[sym], UpdatedAt: time.Now()})
			}
			if err := db.SaveExitPeakStates(peakStates); err != nil {
				result.Reasons = append(result.Reasons, "persist exit peak DB: "+err.Error())
			}
			if !dryRun && cfg.HermesOperator.CanExecute() && !halted {
				if exitResult, err := executeAutonomousExits(ctx, cfg, db, exits, currentOpenOrders(db)); err != nil {
					result.Reasons = append(result.Reasons, "autonomous exit execution: "+err.Error())
				} else if exitResult != nil {
					result.Managed = exitResult
				}
			}
			for _, ex := range exits {
				if ex.Warning {
					result.Reasons = append(result.Reasons, "cảnh báo vị thế (không bán): "+ex.Symbol+": "+ex.Reason)
				} else if ex.Action != liveguard.ExitHold {
					result.Reasons = append(result.Reasons, "tín hiệu bảo vệ lợi nhuận: "+ex.Symbol+" → "+string(ex.Action)+": "+ex.Reason)
				}
			}
		}
		if posErr == nil && len(positions) == 0 {
			_ = db.SaveExitPeakStates(nil)
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

const exitPeakTrackerFile = "exit_peak_tracker.json"

func loadExitPeakTracker() *liveguard.PeakTracker {
	tracker := liveguard.NewPeakTracker()
	b, err := os.ReadFile(filepath.Join("reports", exitPeakTrackerFile))
	if err == nil {
		_ = json.Unmarshal(b, tracker)
	}
	if tracker.PeakBySymbol == nil {
		tracker.PeakBySymbol = map[string]float64{}
	}
	if tracker.TrailActive == nil {
		tracker.TrailActive = map[string]bool{}
	}
	return tracker
}

func saveExitPeakTracker(tracker *liveguard.PeakTracker) error {
	if tracker == nil {
		return nil
	}
	return saveJSONFile("reports", exitPeakTrackerFile, tracker)
}

func executeAutonomousExits(ctx context.Context, cfg config.Config, db *storage.DB, exits []liveguard.ExitDecision, open []live.OrderStatus) (*liveguard.ManagedCycleResult, error) {
	client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
	if err != nil {
		return nil, err
	}
	filters, err := client.InstrumentFilters(ctx)
	if err != nil {
		return nil, err
	}
	owned, err := db.HermesOwnedPositions()
	if err != nil {
		return nil, err
	}
	actions := liveguard.BuildAutonomousExitActions(exits, owned, open)
	result := &liveguard.ManagedCycleResult{GeneratedAt: time.Now(), Status: liveguard.ManagedCycleCompleted}
	for _, item := range actions {
		decision := liveguard.HermesActionDecision{Action: item.Action, Allowed: true, NotionalUSDT: item.Action.RequestedNotionalUSDT}
		decisionID := "exit-" + strings.ToLower(item.Decision.Symbol) + "-" + fmt.Sprint(item.Decision.GeneratedAt.UnixNano())
		var cycle liveguard.ManagedCycleResult
		if item.Action.Intent == hermesoperator.IntentReduce {
			cycle = liveguard.ExecuteHermesReduceActionsWithOpen(ctx, cfg, decisionID, []liveguard.HermesActionDecision{decision}, owned, open, filters, client, db, false)
		} else {
			cycle = liveguard.ExecuteHermesExitLimitActionsWithOpen(ctx, cfg, decisionID, []liveguard.HermesActionDecision{decision}, owned, open, filters, client, db, false)
		}
		result.Placed = append(result.Placed, cycle.Placed...)
		result.Blocked = append(result.Blocked, cycle.Blocked...)
		for _, placed := range cycle.Placed {
			if strings.EqualFold(placed.Desired.Side, "SELL") && placed.Desired.Quantity > 0 {
				open = append(open, live.OrderStatus{ClientOrderID: placed.PlaceResult.ClientOrderID, OrderID: placed.PlaceResult.OrderID, InstID: placed.Desired.InstID, Symbol: placed.Desired.Symbol, Side: "SELL", Quantity: placed.Desired.Quantity, Price: placed.Desired.Price, Status: live.StatusSubmitted, Source: placed.Desired.Source})
			}
		}
	}
	if len(result.Blocked) > 0 {
		result.Status = liveguard.ManagedCyclePartial
	}
	result.Summary = fmt.Sprintf("AUTONOMOUS_EXITS: placed=%d blocked=%d", len(result.Placed), len(result.Blocked))
	return result, nil
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
			if ex.Warning {
				md += fmt.Sprintf("  %s — CẢNH BÁO, KHÔNG BÁN: lãi/lỗ=%.2f%%: %s\n", ex.Symbol, ex.PnLPct*100, ex.Reason)
				continue
			}
			if ex.Action == liveguard.ExitHold {
				continue
			}
			md += fmt.Sprintf("  %s → %s lãi/lỗ=%.2f%% số lượng=%.6f giá=%.4f: %s\n", ex.Symbol, ex.Action, ex.PnLPct*100, ex.SellQuantity, ex.SellPrice, ex.Reason)
		}
		md += "  Chính sách: chỉ tự động bảo vệ lợi nhuận; không tự động bán dưới giá vốn. Khi lỗ chỉ cảnh báo và cung cấp dữ liệu xem xét DCA.\n"
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

func currentOpenOrders(db *storage.DB) []live.OrderStatus {
	if db == nil {
		return nil
	}
	orders, err := db.OpenLiveOrders()
	if err != nil {
		return nil
	}
	return orders
}

func updatePersistedTotalEquity(ctx context.Context, cfg config.Config, db *storage.DB) error {
	positions, err := db.LivePositions()
	if err != nil {
		return err
	}
	prices := buildCurrentPricesFromDB(cfg, db)
	client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
	if err != nil {
		return err
	}
	for _, p := range positions {
		sym := strings.ToUpper(p.Symbol)
		if prices[sym] <= 0 {
			if price, e := client.SpotLastPrice(ctx, sym); e == nil {
				prices[sym] = price
			}
		}
	}
	equity := 0.0
	for _, p := range positions {
		if price := prices[strings.ToUpper(p.Symbol)]; price > 0 {
			equity += p.Quantity * price
		}
	}
	balances, err := client.AccountBalance(ctx)
	if err != nil {
		return err
	}
	for _, b := range balances {
		if strings.EqualFold(b.Asset, "USDT") {
			qty := b.Total
			if qty <= 0 {
				qty = b.Free
			}
			equity += qty
		}
	}
	if equity <= 0 {
		return fmt.Errorf("calculated total equity is not positive")
	}
	_, err = db.UpdateEquityRiskState(equity, time.Now())
	return err
}

func syncHermesManagedPortfolio(ctx context.Context, cfg config.Config, db *storage.DB) (int, int, int, error) {
	client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
	if err != nil {
		return 0, 0, 0, err
	}
	balances, err := client.AccountBalance(ctx)
	if err != nil {
		return 0, 0, 0, err
	}
	current, err := db.HermesManagedHoldings()
	if err != nil {
		return 0, 0, 0, err
	}
	existing := map[string]storage.HermesManagedHolding{}
	for _, h := range current {
		existing[h.Symbol] = h
	}
	adopted, updated, dust := 0, 0, 0
	seen := map[string]bool{}
	for _, b := range balances {
		asset := strings.ToUpper(b.Asset)
		accountQty := b.Total
		if accountQty <= 0 {
			accountQty = b.Free
		}
		if asset == "USDT" || asset == "USDC" || accountQty <= 0 {
			continue
		}
		symbol := asset + "USDT"
		price := b.AvgPrice
		if price <= 0 {
			price, _ = client.SpotLastPrice(ctx, symbol)
		}
		if price <= 0 {
			dust++
			continue
		}
		notional := accountQty * price
		historyBasis := 0.0
		if fills, fe := client.SpotFillHistory(ctx, live.OKXInstID(symbol)); fe == nil {
			if rb := live.ReconstructInventoryBasis(fills, asset, accountQty); rb.Complete && rb.AvgPrice > 0 {
				historyBasis = rb.AvgPrice
			}
		}
		h, ok := existing[symbol]
		// Ignore exchange dust; material balances are delegated by the operator request.
		if !ok && notional < 1 {
			dust++
			continue
		}
		seen[symbol] = true
		if !ok {
			basis, source := price, "OKX_ACCOUNT_ADOPTION_MARK"
			if historyBasis > 0 {
				basis, source = historyBasis, "OKX_FILL_HISTORY_RECONSTRUCTED"
			}
			h = storage.HermesManagedHolding{Symbol: symbol, InstID: live.OKXInstID(symbol), Quantity: accountQty, AvgEntryPrice: basis, Source: source}
			adopted++
		} else {
			newBasis := h.AvgEntryPrice
			newSource := h.Source
			if historyBasis > 0 {
				newBasis, newSource = historyBasis, "OKX_FILL_HISTORY_RECONSTRUCTED"
			} else if b.AvgPrice > 0 {
				newBasis, newSource = b.AvgPrice, "OKX_ACCOUNT_AVG_PRICE"
			}

			qtyChanged := math.Abs(h.Quantity-accountQty) > math.Max(1e-12, math.Abs(h.Quantity)*1e-9)
			basisChanged := math.Abs(h.AvgEntryPrice-newBasis) > math.Max(1e-12, math.Abs(h.AvgEntryPrice)*1e-9)
			if !qtyChanged && !basisChanged {
				continue
			}
			h.Quantity = accountQty
			h.AvgEntryPrice = newBasis
			h.Source = newSource
			updated++
		}
		if err := db.SaveHermesManagedHolding(h); err != nil {
			return adopted, updated, dust, err
		}
	}
	for symbol := range existing {
		if !seen[symbol] {
			if err := db.DeleteHermesManagedHolding(symbol); err != nil {
				return adopted, updated, dust, err
			}
		}
	}
	return adopted, updated, dust, nil
}

func latestMarketRegime(db *storage.DB) string {
	if a, e := db.LatestAnalysis(); e == nil {
		return a.MarketRegime
	}
	return ""
}
func latestAccumulationPhase(db *storage.DB) string {
	if a, e := db.LatestAnalysis(); e == nil {
		return string(a.BTCAccumulation.Phase)
	}
	return ""
}
