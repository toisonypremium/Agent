package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/hermesagent"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/storage"
)

const (
	hermesGateHalted                = "HALTED"
	hermesGateDemoted               = "HERMES_DEMOTED"
	hermesGateDoctorBlock           = "DOCTOR_BLOCK"
	hermesGateReconcileBlock        = "RECONCILE_BLOCK"
	hermesGateUnknownOrder          = "UNKNOWN_ORDER"
	hermesGateNoActionable          = "NO_ACTIONABLE_CANDIDATE"
	hermesGateNoCapital             = "NO_CAPITAL"
	hermesGateNoOwnedPosition       = "NO_OWNED_POSITION"
	hermesGateProtectionUnavailable = "PROTECTION_STATE_UNAVAILABLE"
	hermesGateStateUnchanged        = "STATE_UNCHANGED"
	hermesGateDecisionStillFresh    = "DECISION_STILL_FRESH"
)

type hermesDecisionEvaluation struct {
	Allowed   bool     `json:"allowed"`
	Reason    string   `json:"reason,omitempty"`
	StateHash string   `json:"state_hash"`
	Reasons   []string `json:"reasons,omitempty"`
}

type hermesDecisionHashInput struct {
	Mode        string                       `json:"mode"`
	Market      map[string]any               `json:"market"`
	Assets      []hermesagent.HermesAsset    `json:"assets"`
	Positions   []hermesagent.HermesPosition `json:"positions"`
	Owned       any                          `json:"owned_positions,omitempty"`
	Exits       []hermesagent.HermesExit     `json:"exits"`
	OpenOrders  any                          `json:"open_orders,omitempty"`
	Capital     any                          `json:"capital,omitempty"`
	Protections any                          `json:"protections,omitempty"`
	Reconcile   any                          `json:"reconcile,omitempty"`
	Safety      map[string]any               `json:"safety"`
	Policy      map[string]any               `json:"policy"`
}

func evaluateHermesDecisionPreCall(cfg config.Config, db *storage.DB, snap hermesagent.HermesSnapshot) hermesDecisionEvaluation {
	eval := hermesDecisionEvaluation{Allowed: false, StateHash: hermesDecisionStateHash(cfg, snap)}
	if db == nil {
		eval.Reason = hermesGateDoctorBlock
		eval.Reasons = []string{"runtime database unavailable"}
		return eval
	}
	orders, orderErr := db.OpenLiveOrdersDetailed()
	ownedPositions, ownedErr := db.HermesOwnedPositions()
	capital, capitalErr := db.BuildCapitalAuthoritySnapshot(cfg, snap.GeneratedAt)
	protections, protectionErr := db.ProtectionStatuses()
	reconcile, reconcileOK := loadLatestReconcileReport()
	halted, haltErr := db.IsHalted()
	eval.StateHash = hermesDecisionStateHashWithRuntime(cfg, snap, map[string]any{"orders": normalizedDecisionOrders(orders), "owned_positions": normalizedOwnedPositions(ownedPositions)}, normalizedDecisionCapital(capital), protections, normalizedDecisionReconcile(reconcile.Safety))
	if snap.OperatorHalted || haltErr != nil || halted {
		eval.Reason = hermesGateHalted
		return eval
	}
	if demoted, err := db.IsHermesDemoted(); err != nil || demoted {
		eval.Reason = hermesGateDemoted
		return eval
	}
	doctor := strings.ToUpper(strings.TrimSpace(snap.DoctorStatus))
	if doctor != "OK" && doctor != "DOCTOR_OK" {
		eval.Reason = hermesGateDoctorBlock
		return eval
	}
	if !reconcileOK || reconcile.Safety.Status == "RECONCILE_BLOCK" || reconcile.Safety.RemoteOnly > 0 || reconcile.Safety.IdentityConflicts > 0 || reconcile.Safety.DiscoveryFailed {
		eval.Reason = hermesGateReconcileBlock
		return eval
	}
	if orderErr != nil {
		eval.Reason = hermesGateUnknownOrder
		return eval
	}
	for _, order := range orders {
		if strings.Contains(strings.ToUpper(order.Status), "UNKNOWN") {
			eval.Reason = hermesGateUnknownOrder
			return eval
		}
	}
	if protectionErr != nil {
		eval.Reason = hermesGateProtectionUnavailable
		eval.Reasons = append(eval.Reasons, "protection state unavailable")
		return eval
	}
	if ownedErr != nil {
		eval.Reason = hermesGateNoOwnedPosition
		eval.Reasons = append(eval.Reasons, "Hermes owned-position ledger unavailable")
		return eval
	}
	actionable := false
	increasesExposure := false
	for _, asset := range snap.Assets {
		if asset.ProbeEligible || strings.EqualFold(asset.State, "ACTIVE_LIMIT") {
			actionable = true
			increasesExposure = true
			break
		}
	}
	if !actionable {
		owned := map[string]bool{}
		for _, position := range ownedPositions {
			if position.Quantity > 0 {
				owned[strings.ToUpper(position.Symbol)] = true
			}
		}
		for _, exit := range snap.Exits {
			if exit.Action != "" && !strings.EqualFold(exit.Action, "HOLD") && owned[strings.ToUpper(exit.Symbol)] {
				actionable = true
				break
			}
		}
	}
	if !actionable {
		for _, order := range orders {
			if strings.EqualFold(order.Source, "HERMES_OPERATOR") {
				actionable = true
				break
			}
		}
	}
	if !actionable {
		requestedExit := false
		for _, exit := range snap.Exits {
			if exit.Action != "" && !strings.EqualFold(exit.Action, "HOLD") {
				requestedExit = true
				break
			}
		}
		if requestedExit {
			eval.Reason = hermesGateNoOwnedPosition
		} else {
			eval.Reason = hermesGateNoActionable
		}
		return eval
	}
	if increasesExposure {
		if capitalErr != nil || capital.ConditionalCapacityUSDT <= 0 {
			eval.Reason = hermesGateNoCapital
			return eval
		}
	}
	eval.Allowed = true
	return eval
}

type normalizedDecisionOrder struct {
	ClientOrderID string  `json:"client_order_id"`
	OrderID       string  `json:"order_id"`
	Symbol        string  `json:"symbol"`
	Side          string  `json:"side"`
	Status        string  `json:"status"`
	Price         float64 `json:"price"`
	Quantity      float64 `json:"quantity"`
	Notional      float64 `json:"notional"`
	Source        string  `json:"source"`
}

type normalizedDecisionCapitalState struct {
	AccountEquityUSDT       float64  `json:"account_equity_usdt"`
	ExistingExposureUSDT    float64  `json:"existing_exposure_usdt"`
	OpenBuyNotionalUSDT     float64  `json:"open_buy_notional_usdt"`
	ConditionalCapacityUSDT float64  `json:"conditional_capacity_usdt"`
	HardExposureCapUSDT     float64  `json:"hard_exposure_cap_usdt"`
	Permission              string   `json:"permission"`
	Blockers                []string `json:"blockers,omitempty"`
}

type normalizedDecisionReconcileState struct {
	Status            string   `json:"status"`
	Unknown           int      `json:"unknown"`
	RemotePending     int      `json:"remote_pending"`
	RemoteOnly        int      `json:"remote_only"`
	IdentityConflicts int      `json:"identity_conflicts"`
	DiscoveryFailed   bool     `json:"discovery_failed"`
	Blockers          []string `json:"blockers,omitempty"`
}

func normalizedDecisionOrders(orders []live.OrderStatus) []normalizedDecisionOrder {
	out := make([]normalizedDecisionOrder, 0, len(orders))
	for _, order := range orders {
		out = append(out, normalizedDecisionOrder{ClientOrderID: order.ClientOrderID, OrderID: order.OrderID, Symbol: strings.ToUpper(order.Symbol), Side: strings.ToUpper(order.Side), Status: live.NormalizeOrderStatus(order.Status), Price: order.Price, Quantity: order.Quantity, Notional: order.Notional, Source: order.Source})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ClientOrderID+"|"+out[i].OrderID < out[j].ClientOrderID+"|"+out[j].OrderID
	})
	return out
}

func normalizedOwnedPositions(positions []live.LivePosition) []map[string]any {
	out := make([]map[string]any, 0, len(positions))
	for _, position := range positions {
		out = append(out, map[string]any{"symbol": strings.ToUpper(position.Symbol), "quantity": position.Quantity, "avg_entry_price": position.AvgEntryPrice, "cost_basis": position.CostBasis})
	}
	sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i]["symbol"]) < fmt.Sprint(out[j]["symbol"]) })
	return out
}

func normalizedDecisionCapital(capital storage.CapitalAuthoritySnapshot) normalizedDecisionCapitalState {
	return normalizedDecisionCapitalState{AccountEquityUSDT: capital.AccountEquityUSDT, ExistingExposureUSDT: capital.ExistingExposureUSDT, OpenBuyNotionalUSDT: capital.OpenBuyNotionalUSDT, ConditionalCapacityUSDT: capital.ConditionalCapacityUSDT, HardExposureCapUSDT: capital.HardExposureCapUSDT, Permission: capital.Permission, Blockers: capital.Blockers}
}

func normalizedDecisionReconcile(reconcile liveguard.ReconcileSafetyResult) normalizedDecisionReconcileState {
	return normalizedDecisionReconcileState{Status: reconcile.Status, Unknown: reconcile.Unknown, RemotePending: reconcile.RemotePending, RemoteOnly: reconcile.RemoteOnly, IdentityConflicts: reconcile.IdentityConflicts, DiscoveryFailed: reconcile.DiscoveryFailed, Blockers: reconcile.Blockers}
}

func hermesDecisionStateHash(cfg config.Config, snap hermesagent.HermesSnapshot) string {
	return hermesDecisionStateHashWithRuntime(cfg, snap, nil, nil, nil, nil)
}

func hermesDecisionStateHashWithRuntime(cfg config.Config, snap hermesagent.HermesSnapshot, openOrders, capital, protections, reconcile any) string {
	assets := append([]hermesagent.HermesAsset(nil), snap.Assets...)
	positions := append([]hermesagent.HermesPosition(nil), snap.Positions...)
	exits := append([]hermesagent.HermesExit(nil), snap.Exits...)
	sort.Slice(assets, func(i, j int) bool { return assets[i].Symbol < assets[j].Symbol })
	sort.Slice(positions, func(i, j int) bool { return positions[i].Symbol < positions[j].Symbol })
	sort.Slice(exits, func(i, j int) bool { return exits[i].Symbol < exits[j].Symbol })
	input := hermesDecisionHashInput{
		Mode:   cfg.HermesOperator.NormalizedMode(),
		Market: map[string]any{"phase": snap.BTCPhase, "permission": snap.BTCPermission, "regime": snap.BTCRegime, "trend": snap.BTCTrend, "mm_verdict": snap.BTCMMVerdict, "mm_quality": snap.BTCMMDataQuality, "plan_state": snap.PlanState},
		Assets: assets, Positions: positions, Exits: exits, OpenOrders: openOrders, Capital: capital, Protections: protections, Reconcile: reconcile,
		Safety: map[string]any{"audit": snap.AuditVerdict, "doctor": snap.DoctorStatus, "doctor_blockers": snap.DoctorBlockers, "halted": snap.OperatorHalted},
		Policy: map[string]any{"ttl": cfg.HermesOperator.DecisionTTLSeconds, "min_confidence": cfg.HermesOperator.MinConfidence, "max_actions": cfg.HermesOperator.MaxActionsPerCycle, "probe_cap": config.EffectiveHermesProbeNotional(cfg), "action_cap": config.EffectiveHermesActionNotional(cfg), "portfolio_cap": config.EffectiveHermesPortfolioExposure(cfg), "assets": cfg.Data.Symbols.Assets},
	}
	b, _ := json.Marshal(input)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
