package liveguard

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

const (
	ReadyForManualLiveProofOrder = "READY_FOR_MANUAL_LIVE_PROOF_ORDER"
	NotReadyNoDeterministicOrder = "NOT_READY_NO_DETERMINISTIC_ORDER"
	NotReadyBalance              = "NOT_READY_BALANCE"
	NotReadyFilters              = "NOT_READY_FILTERS"
	NotReadyConfig               = "NOT_READY_CONFIG"
)

type BalanceReader interface {
	AccountBalance(ctx context.Context) ([]live.Balance, error)
}

type CandidateOrder struct {
	Symbol   string  `json:"symbol"`
	Side     string  `json:"side"`
	Type     string  `json:"type"`
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
	Notional float64 `json:"notional"`
	PostOnly bool    `json:"post_only"`
	Source   string  `json:"source"`
}

type AccountCheck struct {
	Enabled         bool    `json:"enabled"`
	AuthOK          bool    `json:"auth_ok"`
	BalanceOK       bool    `json:"balance_ok"`
	BaseCurrency    string  `json:"base_currency"`
	FreeUSDT        float64 `json:"free_usdt,omitempty"`
	MinRequiredUSDT float64 `json:"min_required_usdt"`
	Error           string  `json:"error,omitempty"`
}

type Proof struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Status      string          `json:"status"`
	Reasons     []string        `json:"reasons,omitempty"`
	Candidate   CandidateOrder  `json:"candidate,omitempty"`
	Account     AccountCheck    `json:"account"`
	Preflight   PreflightResult `json:"preflight"`
	Summary     string          `json:"summary"`
}

func BuildProof(cfg config.Config, plan agent2.Plan) Proof {
	return BuildProofWithAccount(context.Background(), cfg, plan, nil)
}

func BuildProofWithAccount(ctx context.Context, cfg config.Config, plan agent2.Plan, reader BalanceReader) Proof {
	return BuildProofWithChecks(ctx, cfg, plan, reader, nil)
}

func BuildProofWithChecks(ctx context.Context, cfg config.Config, plan agent2.Plan, reader BalanceReader, filterReader FilterReader) Proof {
	p := Proof{GeneratedAt: time.Now(), Status: ReadyForManualLiveProofOrder}
	if !cfg.Risk.NoFutures || !cfg.Risk.NoLeverage || !cfg.Risk.SpotLimitOnly {
		return notReady(NotReadyConfig, "risk flags must enforce no futures/no leverage/spot limit only")
	}
	if !cfg.Live.Enabled {
		return notReady(NotReadyConfig, "live.enabled=false")
	}
	if !cfg.Live.ProofOnly && !cfg.Execution.RealTradingEnabled {
		return notReady(NotReadyConfig, "live.proof_only=false requires execution.real_trading_enabled=true")
	}
	if cfg.Live.Exchange == "" || strings.ToLower(cfg.Live.Exchange) != "okx" {
		return notReady(NotReadyConfig, "only okx live proof is planned")
	}
	if cfg.Live.MaxOrderNotionalUSDT <= 0 {
		return notReady(NotReadyConfig, "live.max_order_notional_usdt must be positive")
	}
	for _, env := range []string{cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv} {
		if env == "" || os.Getenv(env) == "" {
			return notReady(NotReadyConfig, "required live credential env is not set: "+env)
		}
	}
	p.Account = AccountCheck{Enabled: reader != nil, BaseCurrency: "USDT", MinRequiredUSDT: cfg.Live.MinAccountFreeUSDT}
	if reader != nil {
		balances, err := reader.AccountBalance(ctx)
		if err != nil {
			p.Account.Error = sanitizeError(err.Error(), cfg)
			return notReadyWithAccount(NotReadyBalance, "OKX account balance check failed", p.Account)
		}
		p.Account.AuthOK = true
		p.Account.FreeUSDT = freeBalance(balances, "USDT")
		p.Account.BalanceOK = cfg.Live.MinAccountFreeUSDT <= 0 || p.Account.FreeUSDT >= cfg.Live.MinAccountFreeUSDT
		if !p.Account.BalanceOK {
			return notReadyWithAccount(NotReadyBalance, fmt.Sprintf("USDT free %.2f below required %.2f", p.Account.FreeUSDT, cfg.Live.MinAccountFreeUSDT), p.Account)
		}
	}
	candidate, ok := firstCandidate(cfg, plan)
	if !ok {
		return notReadyWithChecks(NotReadyNoDeterministicOrder, "no deterministic ACTIVE_LIMIT layer available", p.Account, p.Preflight)
	}
	if filterReader != nil {
		filters, err := filterReader.InstrumentFilters(ctx)
		if err != nil {
			p.Preflight = PreflightResult{Enabled: true, Pass: false, Symbol: candidate.Symbol, Reasons: []string{sanitizeError(err.Error(), cfg)}}
			return notReadyWithChecks(NotReadyFilters, "OKX instrument filter check failed", p.Account, p.Preflight)
		}
		candidate, p.Preflight = RunPreflight(cfg, candidate, filters)
		if !p.Preflight.Pass {
			return notReadyWithChecks(NotReadyFilters, "live preflight failed", p.Account, p.Preflight)
		}
	}
	p.Candidate = candidate
	p.Summary = fmt.Sprintf("Live proof ready: %s %s limit %.8f qty %.8f notional %.2f USDT; no order was placed", candidate.Side, candidate.Symbol, candidate.Price, candidate.Quantity, candidate.Notional)
	return p
}

func firstCandidate(cfg config.Config, plan agent2.Plan) (CandidateOrder, bool) {
	if plan.State != agent2.StateActiveLimit {
		return CandidateOrder{}, false
	}
	for _, asset := range plan.Assets {
		if asset.State != agent2.StateActiveLimit || len(asset.Layers) == 0 {
			continue
		}
		layer := asset.Layers[0]
		notional := layer.Notional
		if cap := cfg.Live.MaxOrderNotionalUSDT; cap > 0 && notional > cap {
			notional = cap
		}
		qty := 0.0
		if layer.Price > 0 {
			qty = notional / layer.Price
		}
		if layer.Price <= 0 || qty <= 0 || notional <= 0 {
			return CandidateOrder{}, false
		}
		return CandidateOrder{Symbol: asset.Symbol, Side: "BUY", Type: "limit", Price: layer.Price, Quantity: qty, Notional: notional, PostOnly: cfg.Live.RequirePostOnly, Source: "deterministic_agent2_layer_1"}, true
	}
	return CandidateOrder{}, false
}

func freeBalance(balances []live.Balance, asset string) float64 {
	for _, b := range balances {
		if strings.EqualFold(b.Asset, asset) {
			return b.Free
		}
	}
	return 0
}

func notReady(status, reason string) Proof {
	return notReadyWithChecks(status, reason, AccountCheck{}, PreflightResult{})
}

func notReadyWithAccount(status, reason string, account AccountCheck) Proof {
	return notReadyWithChecks(status, reason, account, PreflightResult{})
}

func notReadyWithChecks(status, reason string, account AccountCheck, preflight PreflightResult) Proof {
	return Proof{GeneratedAt: time.Now(), Status: status, Reasons: []string{reason}, Account: account, Preflight: preflight, Summary: status + ": " + reason}
}

func sanitizeError(s string, cfg config.Config) string {
	for _, env := range []string{cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv} {
		if env == "" {
			continue
		}
		if value := os.Getenv(env); value != "" {
			s = strings.ReplaceAll(s, value, "<REDACTED>")
		}
	}
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}
