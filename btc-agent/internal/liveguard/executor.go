package liveguard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

const ManualLiveConfirmPhrase = "I_UNDERSTAND_THIS_PLACES_A_REAL_SPOT_LIMIT_ORDER"

const (
	LiveOrderSubmitted = "LIVE_ORDER_SUBMITTED"
	LiveOrderBlocked   = "LIVE_ORDER_BLOCKED"
	LiveOrderRejected  = "LIVE_ORDER_REJECTED"
)

type OrderPlacer interface {
	PlaceSpotLimitOrder(ctx context.Context, req live.LimitOrderRequest) (live.OrderResult, error)
}

type ExecutionResult struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Status      string           `json:"status"`
	ProofStatus string           `json:"proof_status"`
	Candidate   CandidateOrder   `json:"candidate"`
	Preflight   PreflightResult  `json:"preflight"`
	Order       live.OrderResult `json:"order"`
	Reasons     []string         `json:"reasons,omitempty"`
	Summary     string           `json:"summary"`
}

func ExecuteManualProofOrder(ctx context.Context, cfg config.Config, proof Proof, confirm string, placer OrderPlacer) ExecutionResult {
	result := ExecutionResult{GeneratedAt: time.Now(), Status: LiveOrderBlocked, ProofStatus: proof.Status, Candidate: proof.Candidate, Preflight: proof.Preflight}
	reasons := manualOrderBlockers(cfg, proof, confirm, placer)
	if len(reasons) > 0 {
		result.Reasons = reasons
		result.Summary = LiveOrderBlocked + ": " + strings.Join(reasons, "; ")
		return result
	}
	req := live.LimitOrderRequest{
		InstID:        proof.Preflight.InstID,
		Side:          strings.ToLower(proof.Candidate.Side),
		Price:         proof.Candidate.Price,
		Quantity:      proof.Candidate.Quantity,
		PostOnly:      proof.Candidate.PostOnly,
		ClientOrderID: clientOrderID(proof.Candidate.Symbol),
	}
	order, err := placer.PlaceSpotLimitOrder(ctx, req)
	result.Order = order
	if err != nil {
		result.Status = LiveOrderRejected
		result.Reasons = []string{err.Error()}
		result.Summary = LiveOrderRejected + ": " + err.Error()
		return result
	}
	result.Status = LiveOrderSubmitted
	result.Summary = fmt.Sprintf("%s: %s order_id=%s client_order_id=%s", LiveOrderSubmitted, order.InstID, order.OrderID, order.ClientOrderID)
	return result
}

func manualOrderBlockers(cfg config.Config, proof Proof, confirm string, placer OrderPlacer) []string {
	reasons := []string{}
	if confirm != ManualLiveConfirmPhrase {
		reasons = append(reasons, "confirm phrase required")
	}
	if !cfg.Live.Enabled {
		reasons = append(reasons, "live.enabled=false")
	}
	if cfg.Live.ProofOnly {
		reasons = append(reasons, "live.proof_only=true")
	}
	if !cfg.Live.RequireManualConfirm {
		reasons = append(reasons, "live.require_manual_confirm=false")
	}
	if !cfg.Execution.RealTradingEnabled {
		reasons = append(reasons, "execution.real_trading_enabled=false")
	}
	if !cfg.Risk.NoFutures || !cfg.Risk.NoLeverage || !cfg.Risk.SpotLimitOnly {
		reasons = append(reasons, "risk flags must enforce no futures/no leverage/spot limit only")
	}
	proofReady := proof.Status == ReadyForManualLiveProofOrder
	if !proofReady {
		reasons = append(reasons, "proof not ready: "+proof.Status)
	}
	if !proof.Account.AuthOK || !proof.Account.BalanceOK {
		reasons = append(reasons, "account check not pass")
	}
	if proofReady {
		if !proof.Preflight.Pass {
			reasons = append(reasons, "preflight not pass")
		}
		if proof.Candidate.Side != "BUY" {
			reasons = append(reasons, "candidate side must be BUY")
		}
		if strings.ToLower(proof.Candidate.Type) != "limit" {
			reasons = append(reasons, "candidate type must be limit")
		}
		if cfg.Live.RequirePostOnly && !proof.Candidate.PostOnly {
			reasons = append(reasons, "candidate post_only required")
		}
		if cfg.Live.MaxOrderNotionalUSDT > 0 && proof.Candidate.Notional > cfg.Live.MaxOrderNotionalUSDT+1e-9 {
			reasons = append(reasons, "candidate notional above live max")
		}
		if proof.Preflight.InstID == "" {
			reasons = append(reasons, "preflight inst_id required")
		}
	}
	if placer == nil {
		reasons = append(reasons, "order placer unavailable")
	}
	return uniqueStrings(reasons)
}

func clientOrderID(symbol string) string {
	s := strings.ToLower(strings.ReplaceAll(symbol, "-", ""))
	return fmt.Sprintf("btcagent%s%d", s, time.Now().Unix())
}
