package liveguard

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
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

var lastClientOrderIDNano int64

type HaltReader interface {
	IsHalted() (bool, error)
}

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

type LadderExecutionResult struct {
	GeneratedAt   time.Time          `json:"generated_at"`
	Status        string             `json:"status"`
	ProofStatus   string             `json:"proof_status"`
	Candidates    []CandidateOrder   `json:"candidates"`
	Orders        []live.OrderResult `json:"orders"`
	Reasons       []string           `json:"reasons,omitempty"`
	TotalNotional float64            `json:"total_notional"`
	Summary       string             `json:"summary"`
}

func ExecuteManualProofOrder(ctx context.Context, cfg config.Config, proof Proof, confirm string, placer OrderPlacer, haltReader HaltReader) ExecutionResult {
	result := ExecutionResult{GeneratedAt: time.Now(), Status: LiveOrderBlocked, ProofStatus: proof.Status, Candidate: proof.Candidate, Preflight: proof.Preflight}
	reasons := manualOrderBlockers(cfg, proof, confirm, placer, haltReader)
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
		safeErr := sanitizeExchangeError(cfg, err)
		result.Status = LiveOrderRejected
		result.Reasons = []string{safeErr}
		result.Summary = LiveOrderRejected + ": " + safeErr
		return result
	}
	result.Status = LiveOrderSubmitted
	result.Summary = fmt.Sprintf("%s: %s order_id=%s client_order_id=%s", LiveOrderSubmitted, order.InstID, order.OrderID, order.ClientOrderID)
	return result
}

func ExecuteAutoProofOrder(ctx context.Context, cfg config.Config, proof Proof, placer OrderPlacer, openOrders []live.OrderStatus, positions []live.LivePosition, haltReader HaltReader) ExecutionResult {
	result := ExecutionResult{GeneratedAt: time.Now(), Status: LiveOrderBlocked, ProofStatus: proof.Status, Candidate: proof.Candidate, Preflight: proof.Preflight}
	reasons := autoOrderBlockers(cfg, proof, placer, openOrders, positions, haltReader)
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
		safeErr := sanitizeExchangeError(cfg, err)
		result.Status = LiveOrderRejected
		result.Reasons = []string{safeErr}
		result.Summary = LiveOrderRejected + ": " + safeErr
		return result
	}
	result.Status = LiveOrderSubmitted
	result.Summary = fmt.Sprintf("%s: %s order_id=%s client_order_id=%s", LiveOrderSubmitted, order.InstID, order.OrderID, order.ClientOrderID)
	return result
}

func ExecuteAutoLadderProofOrder(ctx context.Context, cfg config.Config, proof LadderProof, placer OrderPlacer, openOrders []live.OrderStatus, positions []live.LivePosition, haltReader HaltReader) LadderExecutionResult {
	result := LadderExecutionResult{GeneratedAt: time.Now(), Status: LiveOrderBlocked, ProofStatus: proof.Status, Candidates: proof.Candidates, TotalNotional: proof.TotalNotional, Orders: []live.OrderResult{}}
	reasons := autoLadderBlockers(cfg, proof, placer, openOrders, positions, haltReader)
	if len(reasons) > 0 {
		result.Reasons = reasons
		result.Summary = LiveOrderBlocked + ": " + strings.Join(reasons, "; ")
		return result
	}
	for _, candidate := range proof.Candidates {
		instID := ""
		for _, preflight := range proof.Preflights {
			if preflight.Symbol == candidate.Symbol {
				instID = preflight.InstID
				break
			}
		}
		if instID == "" {
			instID = strings.ReplaceAll(candidate.Symbol, "USDT", "-USDT")
		}
		req := live.LimitOrderRequest{InstID: instID, Side: strings.ToLower(candidate.Side), Price: candidate.Price, Quantity: candidate.Quantity, PostOnly: candidate.PostOnly, ClientOrderID: clientOrderID(candidate.Symbol)}
		order, err := placer.PlaceSpotLimitOrder(ctx, req)
		result.Orders = append(result.Orders, order)
		if err != nil {
			safeErr := sanitizeExchangeError(cfg, err)
			result.Status = LiveOrderRejected
			result.Reasons = append(result.Reasons, safeErr)
			result.Summary = fmt.Sprintf("%s: submitted %d/%d before error: %s", LiveOrderRejected, len(result.Orders)-1, len(proof.Candidates), safeErr)
			return result
		}
	}
	result.Status = LiveOrderSubmitted
	result.Summary = fmt.Sprintf("%s: submitted %d ladder orders total %.2f USDT", LiveOrderSubmitted, len(result.Orders), result.TotalNotional)
	return result
}

func manualOrderBlockers(cfg config.Config, proof Proof, confirm string, placer OrderPlacer, haltReader HaltReader) []string {
	reasons := []string{}
	halted := true
	var err error
	if haltReader != nil {
		halted, err = haltReader.IsHalted()
		if err != nil {
			halted = true
		}
	}
	if halted {
		reasons = append(reasons, "operator halt active")
	}
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
	return fmt.Sprintf("btclive%s%s", s, nextClientOrderIDSuffix())
}

func sanitizeExchangeError(cfg config.Config, err error) string {
	if err == nil {
		return ""
	}
	out := err.Error()
	for _, envName := range []string{cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv} {
		if envName == "" {
			continue
		}
		if secret := os.Getenv(envName); secret != "" {
			out = strings.ReplaceAll(out, secret, "<REDACTED>")
		}
	}
	out = redactExchangeField(out, `(?i)(OK-ACCESS-KEY\s*[:=]\s*)[^\s,;]+`)
	out = redactExchangeField(out, `(?i)(OK-ACCESS-SIGN\s*[:=]\s*)[^\s,;]+`)
	out = redactExchangeField(out, `(?i)(OK-ACCESS-PASSPHRASE\s*[:=]\s*)[^\s,;]+`)
	out = redactExchangeField(out, `(?i)((?:api_?key|secret|passphrase)\s*[:=]\s*)[^\s,;]+`)
	if len(out) > 500 {
		out = out[:500] + "..."
	}
	return out
}

func redactExchangeField(s, pattern string) string {
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllString(s, `${1}<REDACTED>`)
}

func nextClientOrderIDSuffix() string {
	now := time.Now().UnixNano()
	for {
		prev := atomic.LoadInt64(&lastClientOrderIDNano)
		if now <= prev {
			now = prev + 1
		}
		if atomic.CompareAndSwapInt64(&lastClientOrderIDNano, prev, now) {
			return strconv.FormatInt(now, 36)
		}
	}
}

func autoOrderBlockers(cfg config.Config, proof Proof, placer OrderPlacer, openOrders []live.OrderStatus, positions []live.LivePosition, haltReader HaltReader) []string {
	reasons := []string{}
	halted := true
	var err error
	if haltReader != nil {
		halted, err = haltReader.IsHalted()
		if err != nil {
			halted = true
		}
	}
	if halted {
		reasons = append(reasons, "operator halt active")
	}
	if !cfg.Live.AutoExecute {
		reasons = append(reasons, "live.auto_execute=false")
	}
	if cfg.Live.RequireManualConfirm {
		reasons = append(reasons, "live.require_manual_confirm=true")
	}
	if !cfg.Live.Enabled {
		reasons = append(reasons, "live.enabled=false")
	}
	if cfg.Live.ProofOnly {
		reasons = append(reasons, "live.proof_only=true")
	}
	if !cfg.Execution.RealTradingEnabled {
		reasons = append(reasons, "execution.real_trading_enabled=false")
	}
	if !cfg.Risk.NoFutures || !cfg.Risk.NoLeverage || !cfg.Risk.SpotLimitOnly {
		reasons = append(reasons, "risk flags must enforce no futures/no leverage/spot limit only")
	}
	if len(openOrders) > 0 {
		reasons = append(reasons, "open live order exists; reconcile/fill it before auto execution")
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
		if !livePositionBudgetOK(cfg, proof.Candidate, positions) {
			reasons = append(reasons, "candidate would exceed configured live position budget")
		}
	}
	if placer == nil {
		reasons = append(reasons, "order placer unavailable")
	}
	return uniqueStrings(reasons)
}

func autoLadderBlockers(cfg config.Config, proof LadderProof, placer OrderPlacer, openOrders []live.OrderStatus, positions []live.LivePosition, haltReader HaltReader) []string {
	reasons := []string{}
	halted := true
	var err error
	if haltReader != nil {
		halted, err = haltReader.IsHalted()
		if err != nil {
			halted = true
		}
	}
	if halted {
		reasons = append(reasons, "operator halt active")
	}
	if !cfg.Live.AutoExecute {
		reasons = append(reasons, "live.auto_execute=false")
	}
	if !cfg.Live.AutoLadderEnabled {
		reasons = append(reasons, "live.auto_ladder_enabled=false")
	}
	if cfg.Live.RequireManualConfirm {
		reasons = append(reasons, "live.require_manual_confirm=true")
	}
	if !cfg.Live.Enabled {
		reasons = append(reasons, "live.enabled=false")
	}
	if cfg.Live.ProofOnly {
		reasons = append(reasons, "live.proof_only=true")
	}
	if !cfg.Execution.RealTradingEnabled {
		reasons = append(reasons, "execution.real_trading_enabled=false")
	}
	if len(openOrders) >= normalizedMaxOpenLiveOrdersExec(cfg) {
		reasons = append(reasons, "open live order limit reached")
	}
	if proof.Status != ReadyForManualLiveProofOrder {
		reasons = append(reasons, "proof not ready: "+proof.Status)
	}
	if !proof.Account.AuthOK || !proof.Account.BalanceOK {
		reasons = append(reasons, "account check not pass")
	}
	if len(proof.Candidates) == 0 {
		reasons = append(reasons, "no ladder candidates")
	}
	if proof.TotalNotional <= 0 {
		reasons = append(reasons, "ladder total notional must be positive")
	} else if proof.TotalNotional > normalizedAutoLadderMaxNotionalExec(cfg)+1e-9 {
		reasons = append(reasons, "ladder total notional above max")
	}
	for _, candidate := range proof.Candidates {
		if candidate.Side != "BUY" {
			reasons = append(reasons, "candidate side must be BUY")
		}
		if strings.ToLower(candidate.Type) != "limit" {
			reasons = append(reasons, "candidate type must be limit")
		}
		if cfg.Live.RequirePostOnly && !candidate.PostOnly {
			reasons = append(reasons, "candidate post_only required")
		}
		if !livePositionBudgetOK(cfg, candidate, positions) {
			reasons = append(reasons, "candidate would exceed configured live position budget")
		}
	}
	for _, preflight := range proof.Preflights {
		if !preflight.Pass {
			reasons = append(reasons, "preflight not pass")
		}
	}
	if placer == nil {
		reasons = append(reasons, "order placer unavailable")
	}
	return uniqueStrings(reasons)
}

func livePositionBudgetOK(cfg config.Config, candidate CandidateOrder, positions []live.LivePosition) bool {
	if candidate.Symbol == "" || candidate.Notional <= 0 {
		return false
	}
	allocation := cfg.Portfolio.Allocation[strings.ToUpper(candidate.Symbol)]
	if allocation <= 0 || cfg.Portfolio.TotalCapital <= 0 {
		return false
	}
	budget := cfg.Portfolio.TotalCapital * allocation * cfg.Risk.MaxTotalDeploymentPerCycle
	if cfg.Risk.MaxSingleAssetDeployment > 0 {
		maxSingle := cfg.Portfolio.TotalCapital * cfg.Risk.MaxSingleAssetDeployment
		if budget > maxSingle {
			budget = maxSingle
		}
	}
	if budget <= 0 {
		return false
	}
	current := 0.0
	want := strings.ToUpper(candidate.Symbol)
	for _, pos := range positions {
		if strings.EqualFold(pos.Symbol, want) {
			current = pos.CostBasis
			break
		}
	}
	return current+candidate.Notional <= budget+1e-9
}

func normalizedMaxOpenLiveOrdersExec(cfg config.Config) int {
	if cfg.Live.MaxOpenLiveOrders <= 0 {
		return 1
	}
	return cfg.Live.MaxOpenLiveOrders
}

func normalizedAutoLadderMaxNotionalExec(cfg config.Config) float64 {
	if cfg.Live.AutoLadderMaxNotionalUSDT > 0 {
		return cfg.Live.AutoLadderMaxNotionalUSDT
	}
	return config.LiveAutoMaxNotionalUSDT(cfg)
}
