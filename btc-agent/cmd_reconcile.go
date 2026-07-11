package main

import (
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/storage"
	"btc-agent/internal/telegramreport"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

func runReconcileLiveOrders(ctx context.Context, cfg config.Config, db *storage.DB) error {
	return runReconcileLiveOrdersWithNotify(ctx, cfg, db, true)
}

func runReconcileLiveOrdersWithNotify(ctx context.Context, cfg config.Config, db *storage.DB, notifyTelegram bool) error {
	open, err := db.OpenLiveOrders()
	if err != nil {
		return fmt.Errorf("load open live orders: %w", err)
	}

	var result liveguard.ReconcileResult
	if len(open) == 0 {
		// #11: no open orders — skip OKX client creation entirely; ReconcileOrders with
		// nil reader is safe but pointless. Build empty clean result directly.
		result = liveguard.ReconcileOrders(ctx, nil, open)
	} else {
		client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
		if err != nil {
			return fmt.Errorf("create okx client: %w", err)
		}
		result = liveguard.ReconcileOrders(ctx, client, open)
	}

	ledgerReport := liveguard.LiveLedgerReport{GeneratedAt: time.Now(), ManualCheckRequired: []string{}, Events: []live.LivePositionEvent{}}
	for _, o := range result.Orders {
		if o.Status != live.StatusUnknownNeedsManualCheck {
			if err := db.SaveLiveOrderStatus(o); err != nil {
				return fmt.Errorf("save reconciled live order %s/%s: %w", o.ClientOrderID, o.OrderID, err)
			}
		}
		if err := db.SaveLiveOrderEvent(o); err != nil {
			return fmt.Errorf("save live order event %s/%s: %w", o.ClientOrderID, o.OrderID, err)
		}
		if err := applyLedgerUpdate(db, o, &ledgerReport); err != nil {
			return err
		}
	}

	positions, err := db.LivePositions()
	if err != nil {
		return fmt.Errorf("load live positions: %w", err)
	}
	ledgerReport.Positions = positions
	ledgerReport.Summary = liveguard.LiveLedgerSummary(ledgerReport)

	if err := saveJSONFile("reports", "live_reconcile_latest.json", result); err != nil {
		return err
	}

	md := reconcileMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "live_reconcile_latest.md"), []byte(md), 0600); err != nil {
		return err
	}

	if err := writeLivePositionReport(ledgerReport); err != nil {
		return err
	}

	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && notifyTelegram {
		sendTelegram(ctx, cfg, "reconcile-live-orders", telegramreport.ReconcileHumanText(result, ledgerReport))
	}

	fmt.Println(md)
	fmt.Println(livePositionMarkdown(ledgerReport))
	return nil
}

func applyLedgerUpdate(db *storage.DB, status live.OrderStatus, report *liveguard.LiveLedgerReport) error {
	if status.Status == live.StatusUnknownNeedsManualCheck {
		report.ManualCheckRequired = append(report.ManualCheckRequired, fmt.Sprintf("%s/%s status unknown", status.ClientOrderID, status.OrderID))
		return nil
	}
	if status.ClientOrderID == "" && status.OrderID == "" {
		report.ManualCheckRequired = append(report.ManualCheckRequired, fmt.Sprintf("%s missing order identifiers", status.InstID))
		return nil
	}
	previous, _, err := db.LiveFillSnapshot(status.ClientOrderID, status.OrderID)
	if err != nil {
		return fmt.Errorf("load live fill snapshot %s/%s: %w", status.ClientOrderID, status.OrderID, err)
	}
	event, ok, err := liveguard.BuildPositionEvent(previous, status, time.Now())
	if err != nil {
		report.ManualCheckRequired = append(report.ManualCheckRequired, err.Error())
		return nil
	}
	if !ok {
		return nil
	}
	position, err := db.ApplyLivePositionEvent(event)
	if err != nil {
		report.ManualCheckRequired = append(report.ManualCheckRequired, err.Error())
		return nil
	}
	event.PositionQty = position.Quantity
	event.AvgEntryPrice = position.AvgEntryPrice
	if err := db.SaveLivePositionEvent(event); err != nil {
		return fmt.Errorf("save live position event %s/%s: %w", event.ClientOrderID, event.OrderID, err)
	}
	snapshot := liveguard.FillSnapshotFromStatus(status)
	if snapshot.ClientOrderID == "" {
		snapshot.ClientOrderID = previous.ClientOrderID
	}
	if snapshot.ClientOrderID == "" {
		report.ManualCheckRequired = append(report.ManualCheckRequired, fmt.Sprintf("%s/%s missing client_order_id for fill snapshot", status.InstID, status.OrderID))
		return nil
	}
	if err := db.SaveLiveFillSnapshot(snapshot); err != nil {
		return fmt.Errorf("save live fill snapshot %s/%s: %w", snapshot.ClientOrderID, snapshot.OrderID, err)
	}
	report.Events = append(report.Events, event)
	report.Updated++
	return nil
}

func runLivePositions(cfg config.Config, db *storage.DB) error {
	positions, err := db.LivePositions()
	if err != nil {
		return fmt.Errorf("load live positions: %w", err)
	}
	report := liveguard.LiveLedgerReport{GeneratedAt: time.Now(), Positions: positions, Events: []live.LivePositionEvent{}, ManualCheckRequired: []string{}}
	report.Summary = liveguard.LiveLedgerSummary(report)
	if err := writeLivePositionReport(report); err != nil {
		return err
	}
	md := livePositionMarkdown(report)
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(context.Background(), cfg, "live-positions", telegramreport.PositionHumanText(report))
	}
	fmt.Println(md)
	return nil
}

func writeLivePositionReport(report liveguard.LiveLedgerReport) error {
	if err := saveJSONFile("reports", "live_position_latest.json", report); err != nil {
		return err
	}
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join("reports", "live_position_latest.md"), []byte(livePositionMarkdown(report)), 0600)
}

func reconcileMarkdown(result liveguard.ReconcileResult) string {
	md := fmt.Sprintf("LIVE RECONCILIATION REPORT\n\nGenerated: %s\nSummary: %s\nChecked: %d | Updated: %d | Unknown: %d\nSafety: %s | %s\n\n",
		result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Summary, result.Checked, result.Updated, result.Unknown, result.Safety.Status, result.Safety.Summary)
	md += "No order was placed.\n"
	if len(result.Orders) > 0 {
		md += "\nOrders:\n"
		for _, o := range result.Orders {
			md += fmt.Sprintf("- %s: clOrdId=%s ordId=%s status=%s px=%.2f qty=%.4f avgPx=%.2f\n",
				o.InstID, o.ClientOrderID, o.OrderID, o.Status, o.Price, o.Quantity, o.AvgPrice)
		}
	}
	return md
}

func livePositionMarkdown(result liveguard.LiveLedgerReport) string {
	md := fmt.Sprintf("LIVE POSITION LEDGER\n\nGenerated: %s\nSummary: %s\nLedger updates: %d | Manual checks: %d\n\n", result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Summary, result.Updated, len(result.ManualCheckRequired))
	md += "No order was placed.\n"
	if len(result.Positions) == 0 {
		md += "No live positions recorded.\n"
	} else {
		md += "\nPositions:\n"
		for _, p := range result.Positions {
			md += fmt.Sprintf("- %s: qty=%.8f avg_entry=%.8f cost=%.2f fee_total=%.8f fee_ccy=%s\n", p.Symbol, p.Quantity, p.AvgEntryPrice, p.CostBasis, p.FeeTotal, p.FeeCurrency)
		}
	}
	if len(result.Events) > 0 {
		md += "\nNew ledger events:\n"
		for _, e := range result.Events {
			md += fmt.Sprintf("- %s: order=%s delta_qty=%.8f fill_price=%.8f fee_delta=%.8f status=%s\n", e.Symbol, firstNonEmpty(e.ClientOrderID, e.OrderID), e.DeltaQuantity, e.FillPrice, e.FeeDelta, e.Status)
		}
	}
	if len(result.ManualCheckRequired) > 0 {
		md += "\nManual check required:\n"
		for _, item := range result.ManualCheckRequired {
			md += "- " + item + "\n"
		}
	}
	return md
}

func runCancelAllLiveOrders(ctx context.Context, cfg config.Config, db *storage.DB, dryRun bool) error {
	open, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		return fmt.Errorf("load open live orders: %w", err)
	}
	result := liveguard.ManagedCycleResult{GeneratedAt: time.Now(), Status: liveguard.ManagedCycleCompleted, PlanState: agent2.StateNoTrade, DryRun: dryRun}
	if len(open) == 0 {
		result.Summary = "cancel-all: no open live orders"
	} else if dryRun {
		for _, order := range open {
			result.Canceled = append(result.Canceled, liveguard.ManagedOrderDecision{Action: "would_cancel", Symbol: live.InternalSymbol(order.InstID), LayerIndex: order.LayerIndex, Order: order, Reason: "emergency cancel all dry-run"})
		}
		result.Status = liveguard.ManagedCycleDryRun
		result.Summary = fmt.Sprintf("cancel-all dry-run: would cancel %d open live orders", len(result.Canceled))
	} else {
		client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
		if err != nil {
			return fmt.Errorf("create okx client: %w", err)
		}
		for _, order := range open {
			decision := liveguard.ManagedOrderDecision{Action: "cancel", Symbol: live.InternalSymbol(order.InstID), LayerIndex: order.LayerIndex, Order: order, Reason: "emergency cancel all"}
			cancel, err := client.CancelOrder(ctx, live.CancelOrderRequest{InstID: order.InstID, OrderID: order.OrderID, ClientOrderID: order.ClientOrderID})
			decision.CancelResult = cancel
			if err != nil {
				decision.Error = err.Error()
				result.Blocked = append(result.Blocked, decision)
				result.Status = liveguard.ManagedCyclePartial
				continue
			}
			result.Canceled = append(result.Canceled, decision)
			status := order
			status.Status = live.StatusCanceled
			status.UpdatedAt = time.Now().Unix()
			status.LastManagementAction = "emergency cancel all"
			if err := db.SaveLiveOrderStatus(status); err != nil {
				return fmt.Errorf("save canceled order: %w", err)
			}
			if err := db.SaveLiveOrderEvent(status); err != nil {
				return fmt.Errorf("save canceled order event: %w", err)
			}
		}
		if result.Status == "" || result.Status == liveguard.ManagedCycleCompleted {
			result.Status = liveguard.ManagedCycleCompleted
		}
		result.Summary = fmt.Sprintf("cancel-all: canceled=%d blocked=%d", len(result.Canceled), len(result.Blocked))
		if len(result.Canceled) > 0 {
			if err := runReconcileLiveOrders(ctx, cfg, db); err != nil {
				log.Printf("post-cancel-all reconcile warning: %v", err)
			}
		}
	}
	if result.Summary == "" {
		result.Summary = fmt.Sprintf("cancel-all: canceled=%d blocked=%d", len(result.Canceled), len(result.Blocked))
	}
	if err := saveJSONFile("reports", "cancel_all_live_orders_latest.json", result); err != nil {
		return err
	}
	md := autoLiveManagementMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "cancel_all_live_orders_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(ctx, cfg, "cancel-all-live-orders", telegramreport.LiveOrderManagementHumanText(result))
	}
	fmt.Println(md)
	return nil
}

func runOperatorHalt(ctx context.Context, cfg config.Config, db *storage.DB) error {
	if err := db.SetHaltStatus(true); err != nil {
		return fmt.Errorf("set halt status: %w", err)
	}
	text := "Operator halt: ACTIVE (Live trading halted)"
	fmt.Println(text)
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(ctx, cfg, "operator-halt", text)
	}
	return nil
}

func runOperatorResume(ctx context.Context, cfg config.Config, db *storage.DB) error {
	if err := db.SetHaltStatus(false); err != nil {
		return fmt.Errorf("clear halt status: %w", err)
	}
	text := "Operator halt: INACTIVE (Live trading resumed)"
	fmt.Println(text)
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(ctx, cfg, "operator-resume", text)
	}
	return nil
}

func runOperatorStatus(db *storage.DB) error {
	halted, err := db.IsHalted()
	if err != nil {
		return fmt.Errorf("read halt status: %w", err)
	}
	status := "INACTIVE"
	if halted {
		status = "ACTIVE (trading halted)"
	}
	fmt.Printf("Operator halt: %s\n", status)
	return nil
}
