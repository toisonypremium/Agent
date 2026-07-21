package liveguard

import (
	"context"
	"fmt"
	"time"

	"btc-agent/internal/exchange/live"
)

type OrderStatusReader interface {
	OrderStatus(ctx context.Context, instID, orderID, clientOrderID string) (live.OrderStatus, error)
	PendingOrders(ctx context.Context, instID string) ([]live.OrderStatus, error)
}

const (
	ReconcileClean = "RECONCILE_CLEAN"
	ReconcileWarn  = "RECONCILE_WARN"
	ReconcileBlock = "RECONCILE_BLOCK"
)

type ReconcileResult struct {
	GeneratedAt       time.Time             `json:"generated_at"`
	Checked           int                   `json:"checked"`
	Updated           int                   `json:"updated"`
	Unknown           int                   `json:"unknown"`
	RemotePending     int                   `json:"remote_pending,omitempty"`
	RemoteOnlyOrders  []live.OrderStatus    `json:"remote_only_orders,omitempty"`
	IdentityConflicts int                   `json:"identity_conflicts,omitempty"`
	DiscoveryError    string                `json:"discovery_error,omitempty"`
	Orders            []live.OrderStatus    `json:"orders"`
	Safety            ReconcileSafetyResult `json:"safety,omitempty"`
	Summary           string                `json:"summary"`
}

type ReconcileSafetyResult struct {
	Status             string   `json:"status"`
	LocalOpen          int      `json:"local_open"`
	Unknown            int      `json:"unknown"`
	RemotePending      int      `json:"remote_pending,omitempty"`
	RemoteOnly         int      `json:"remote_only,omitempty"`
	IdentityConflicts  int      `json:"identity_conflicts,omitempty"`
	DiscoveryFailed    bool     `json:"discovery_failed,omitempty"`
	OperatorHalted     bool     `json:"operator_halted,omitempty"`
	OpenAfterReconcile int      `json:"open_after_reconcile,omitempty"`
	UnknownPositions   int      `json:"unknown_positions,omitempty"`
	Blockers           []string `json:"blockers,omitempty"`
	Warnings           []string `json:"warnings,omitempty"`
	Summary            string   `json:"summary"`
}

func ReconcileOrders(ctx context.Context, reader OrderStatusReader, open []live.OrderStatus) ReconcileResult {
	res := ReconcileResult{GeneratedAt: time.Now(), Orders: []live.OrderStatus{}, RemoteOnlyOrders: []live.OrderStatus{}}
	res.Checked = len(open)
	if reader == nil {
		for _, o := range open {
			o.Status = live.StatusUnknownNeedsManualCheck
			res.Unknown++
			res.Orders = append(res.Orders, o)
		}
		res.Summary = reconcileSummary(res)
		res.Safety = ReconcileSafety(res)
		return res
	}

	pending, err := reader.PendingOrders(ctx, "")
	if err != nil {
		res.DiscoveryError = err.Error()
	} else {
		res.RemotePending = len(pending)
		localByClientID := map[string]int{}
		localByOrderID := map[string]int{}
		for i, order := range open {
			if order.ClientOrderID != "" {
				if _, exists := localByClientID[order.ClientOrderID]; exists {
					res.IdentityConflicts++
				} else {
					localByClientID[order.ClientOrderID] = i
				}
			}
			if order.OrderID != "" {
				if _, exists := localByOrderID[order.OrderID]; exists {
					res.IdentityConflicts++
				} else {
					localByOrderID[order.OrderID] = i
				}
			}
		}
		for _, remote := range pending {
			clientIndex, clientMatch := localByClientID[remote.ClientOrderID]
			orderIndex, orderMatch := localByOrderID[remote.OrderID]
			if remote.ClientOrderID == "" && remote.OrderID == "" {
				res.IdentityConflicts++
				res.RemoteOnlyOrders = append(res.RemoteOnlyOrders, remote)
				continue
			}
			if clientMatch && orderMatch && clientIndex != orderIndex {
				res.IdentityConflicts++
				res.RemoteOnlyOrders = append(res.RemoteOnlyOrders, remote)
				continue
			}
			if clientMatch && remote.OrderID != "" && open[clientIndex].OrderID != "" && remote.OrderID != open[clientIndex].OrderID {
				res.IdentityConflicts++
				res.RemoteOnlyOrders = append(res.RemoteOnlyOrders, remote)
				continue
			}
			if orderMatch && remote.ClientOrderID != "" && open[orderIndex].ClientOrderID != "" && remote.ClientOrderID != open[orderIndex].ClientOrderID {
				res.IdentityConflicts++
				res.RemoteOnlyOrders = append(res.RemoteOnlyOrders, remote)
				continue
			}
			if !clientMatch && !orderMatch {
				res.RemoteOnlyOrders = append(res.RemoteOnlyOrders, remote)
			}
		}
	}

	for _, o := range open {
		remote, statusErr := reader.OrderStatus(ctx, o.InstID, o.OrderID, o.ClientOrderID)
		if statusErr != nil {
			o.Status = live.StatusUnknownNeedsManualCheck
			res.Unknown++
			res.Orders = append(res.Orders, o)
			continue
		}
		remote = withLocalOrderIdentity(remote, o)
		if live.NormalizeOrderStatus(remote.Status) != live.NormalizeOrderStatus(o.Status) {
			res.Updated++
		}
		res.Orders = append(res.Orders, remote)
	}

	res.Summary = reconcileSummary(res)
	res.Safety = ReconcileSafety(res)
	return res
}

func reconcileSummary(res ReconcileResult) string {
	return fmt.Sprintf("reconciled %d local orders: updated %d, unknown %d, remote_pending %d, remote_only %d, identity_conflicts %d", res.Checked, res.Updated, res.Unknown, res.RemotePending, len(res.RemoteOnlyOrders), res.IdentityConflicts)
}

func withLocalOrderIdentity(remote, local live.OrderStatus) live.OrderStatus {
	if remote.InstID == "" {
		remote.InstID = local.InstID
	}
	if remote.OrderID == "" {
		remote.OrderID = local.OrderID
	}
	if remote.ClientOrderID == "" {
		remote.ClientOrderID = local.ClientOrderID
	}
	return remote
}

func ReconcileSafety(result ReconcileResult) ReconcileSafetyResult {
	safety := ReconcileSafetyResult{Status: ReconcileClean, LocalOpen: result.Checked, Unknown: result.Unknown, RemotePending: result.RemotePending, RemoteOnly: len(result.RemoteOnlyOrders), IdentityConflicts: result.IdentityConflicts, DiscoveryFailed: result.DiscoveryError != ""}
	if result.Unknown > 0 {
		safety.Blockers = append(safety.Blockers, fmt.Sprintf("%d live order status unknown", result.Unknown))
	}
	if result.DiscoveryError != "" {
		safety.Blockers = append(safety.Blockers, "pending-order discovery failed: "+result.DiscoveryError)
	}
	if len(result.RemoteOnlyOrders) > 0 {
		safety.Blockers = append(safety.Blockers, fmt.Sprintf("%d remote pending order not present in local ledger", len(result.RemoteOnlyOrders)))
	}
	if result.IdentityConflicts > 0 {
		safety.Blockers = append(safety.Blockers, fmt.Sprintf("%d local/remote order identity conflict", result.IdentityConflicts))
	}
	for _, order := range result.Orders {
		if order.Status == live.StatusUnknownNeedsManualCheck {
			safety.Blockers = append(safety.Blockers, fmt.Sprintf("%s/%s needs manual check", order.ClientOrderID, order.OrderID))
		}
		if live.NormalizeOrderStatus(order.Status) == live.StatusPartialFill || live.NormalizeOrderStatus(order.Status) == live.StatusFilled {
			filled := order.AccumulatedFillSz
			if filled == 0 {
				filled = order.FilledQuantity
			}
			if filled <= 0 || (order.AvgPrice <= 0 && order.Price <= 0) {
				safety.Blockers = append(safety.Blockers, fmt.Sprintf("%s/%s fill status missing fill quantity/price", order.ClientOrderID, order.OrderID))
			}
		}
	}
	return finalizeReconcileSafety(safety)
}

// ApplyHaltedReconcileInvariant fails closed when a halted bot still has an
// exchange-open order or a positive local position whose identity/valuation is
// incomplete. Zero-quantity ledger rows are closed history and are ignored.
func ApplyHaltedReconcileInvariant(result ReconcileResult, positions []live.LivePosition, halted bool) ReconcileResult {
	if !halted {
		return result
	}
	result.Safety.OperatorHalted = true
	for _, order := range result.Orders {
		if live.IsOpenStatus(order.Status) {
			result.Safety.OpenAfterReconcile++
		}
	}
	for _, position := range positions {
		if position.Quantity <= 0 {
			continue
		}
		if position.Symbol == "" || position.InstID == "" || position.AvgEntryPrice <= 0 || position.CostBasis <= 0 {
			result.Safety.UnknownPositions++
		}
	}
	if result.Safety.OpenAfterReconcile > 0 {
		result.Safety.Blockers = append(result.Safety.Blockers, fmt.Sprintf("halted invariant: %d exchange-open live order", result.Safety.OpenAfterReconcile))
	}
	if result.Safety.UnknownPositions > 0 {
		result.Safety.Blockers = append(result.Safety.Blockers, fmt.Sprintf("halted invariant: %d live position needs manual check", result.Safety.UnknownPositions))
	}
	result.Safety = finalizeReconcileSafety(result.Safety)
	return result
}

func finalizeReconcileSafety(safety ReconcileSafetyResult) ReconcileSafetyResult {
	safety.Blockers = uniqueHealthStrings(safety.Blockers)
	safety.Warnings = uniqueHealthStrings(safety.Warnings)
	if len(safety.Blockers) > 0 {
		safety.Status = ReconcileBlock
	} else if len(safety.Warnings) > 0 {
		safety.Status = ReconcileWarn
	} else {
		safety.Status = ReconcileClean
	}
	safety.Summary = fmt.Sprintf("%s: local_open=%d unknown=%d remote_pending=%d remote_only=%d identity_conflicts=%d discovery_failed=%t open_after_reconcile=%d unknown_positions=%d blockers=%d warnings=%d", safety.Status, safety.LocalOpen, safety.Unknown, safety.RemotePending, safety.RemoteOnly, safety.IdentityConflicts, safety.DiscoveryFailed, safety.OpenAfterReconcile, safety.UnknownPositions, len(safety.Blockers), len(safety.Warnings))
	return safety
}
