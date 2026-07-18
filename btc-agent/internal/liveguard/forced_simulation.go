package liveguard

import (
	"context"
	"strings"
	"time"

	"btc-agent/internal/exchange/live"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/liquidity"
	"btc-agent/internal/market"
)

type ForcedSimulationResult struct {
	GeneratedAt   time.Time          `json:"generated_at"`
	Passed        bool               `json:"passed"`
	Status        string             `json:"status"`
	Desired       int                `json:"desired"`
	WouldPlace    int                `json:"would_place"`
	Blocked       int                `json:"blocked"`
	ExchangeCalls int                `json:"exchange_calls"`
	Reasons       []string           `json:"reasons,omitempty"`
	Managed       ManagedCycleResult `json:"managed"`
	Summary       string             `json:"summary"`
}

func RunForcedActiveLimitSimulation(cfg config.Config) ForcedSimulationResult {
	plan := forcedActiveLimitPlan(cfg)
	exchange := &forcedSimulationExchange{}
	execCtx := ManagedExecutionContext{BTCAccumulationPhase: "ACCUMULATION_CONFIRMED", FirstOrderDryRunApproved: true}
	result := ManageLiveOrdersWithRecorderAndContext(context.Background(), cfg, plan, nil, nil, nil, exchange, exchange, fakeHaltFalse{}, execCtx, nil, true)
	out := ForcedSimulationResult{GeneratedAt: time.Now(), Status: "PASS", Managed: result, Desired: len(result.Desired), WouldPlace: len(result.Placed), Blocked: len(result.Blocked), ExchangeCalls: exchange.calls}
	if out.Desired == 0 {
		out.Reasons = append(out.Reasons, "desired orders = 0")
	}
	if out.WouldPlace == 0 {
		out.Reasons = append(out.Reasons, "would_place orders = 0")
	}
	if out.ExchangeCalls != 0 {
		out.Reasons = append(out.Reasons, "exchange calls must be 0 in dry-run simulation")
	}
	for _, decision := range result.Placed {
		if decision.Action != "would_place" {
			out.Reasons = append(out.Reasons, "placed action is not would_place")
		}
		d := decision.Desired
		if strings.ToUpper(d.Side) != "BUY" || strings.ToLower(d.Type) != "limit" || !d.PostOnly {
			out.Reasons = append(out.Reasons, "desired order is not BUY limit post-only")
		}
		if cap := normalizedMaxLiveNotionalPerOrder(cfg); cap > 0 && d.Notional > cap+1e-9 {
			out.Reasons = append(out.Reasons, "desired notional above per-order cap")
		}
	}
	out.Passed = len(out.Reasons) == 0
	if !out.Passed {
		out.Status = "FAIL"
	}
	out.Summary = out.Status
	if out.Passed {
		out.Summary = "forced ACTIVE_LIMIT simulation passed: dry-run produced would_place without exchange calls"
	}
	return out
}

type forcedSimulationExchange struct {
	calls int
}

func (f *forcedSimulationExchange) PlaceSpotLimitOrder(ctx context.Context, req live.LimitOrderRequest) (live.OrderResult, error) {
	f.calls++
	return live.OrderResult{}, nil
}

func (f *forcedSimulationExchange) CancelOrder(ctx context.Context, req live.CancelOrderRequest) (live.CancelOrderResult, error) {
	f.calls++
	return live.CancelOrderResult{}, nil
}

func (f *forcedSimulationExchange) OrderBook(ctx context.Context, instID string) (liquidity.OrderBookSnapshot, error) {
	return liquidity.OrderBookSnapshot{BestBid: 99.95, BestAsk: 100.05, BidDepth1PctUSDT: 100000, AskDepth1PctUSDT: 100000}, nil
}

type fakeHaltFalse struct{}

func (fakeHaltFalse) IsHalted() (bool, error) { return false, nil }

func forcedActiveLimitPlan(cfg config.Config) agent2.Plan {
	symbol := "ETHUSDT"
	if len(cfg.Data.Symbols.Assets) > 0 && strings.TrimSpace(cfg.Data.Symbols.Assets[0]) != "" {
		symbol = strings.ToUpper(cfg.Data.Symbols.Assets[0])
	}
	notional := normalizedMaxLiveNotionalPerOrder(cfg)
	if notional <= 0 {
		notional = 10
	}
	if cfg.Live.FirstOrderMaxNotionalUSDT > 0 && notional > cfg.Live.FirstOrderMaxNotionalUSDT {
		notional = cfg.Live.FirstOrderMaxNotionalUSDT
	}
	price := 100.0
	return agent2.Plan{
		Timestamp:        time.Now(),
		State:            agent2.StateActiveLimit,
		ActionPermission: agent1.Allowed,
		Assets: []agent2.AssetPlan{{
			Symbol:       symbol,
			State:        agent2.StateActiveLimit,
			DiscountZone: market.Zone{Low: 90, High: 100, Name: "forced"},
			Invalidation: 88,
			RewardRisk:   3,
			Reason:       "forced ACTIVE_LIMIT simulation",
			Layers: []agent2.Layer{{
				Index:      1,
				Price:      price,
				Notional:   notional,
				Quantity:   notional / price,
				Target:     130,
				RewardRisk: 3,
			}},
		}},
	}
}
