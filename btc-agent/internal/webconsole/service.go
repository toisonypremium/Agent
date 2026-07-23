// Package webconsole exposes a typed, read-only projection of agent state.
// It has no exchange, configuration, scheduler, or execution authority.
package webconsole

import (
	"context"
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/paper"
	"btc-agent/internal/runtime/ownership"
	"btc-agent/internal/storage"
)

const SchemaVersion = 1

// Clock is injected so freshness and evidence results are deterministic in tests.
type Clock func() time.Time

type Service struct {
	db    *storage.DB
	now   Clock
	lease string
}

func NewService(db *storage.DB, now Clock) (*Service, error) {
	if db == nil {
		return nil, fmt.Errorf("web console database required")
	}
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, now: now, lease: "okx-live"}, nil
}

type Freshness struct {
	State      string `json:"state"`
	AgeSeconds int64  `json:"age_seconds"`
}

type Overview struct {
	Halted bool          `json:"halted"`
	Market MarketSummary `json:"market"`
	Lease  LeaseSummary  `json:"lease"`
	Paper  PaperSummary  `json:"paper"`
}

type MarketSummary struct {
	Available         bool   `json:"available"`
	Regime            string `json:"regime,omitempty"`
	Permission        string `json:"permission,omitempty"`
	PlanState         string `json:"plan_state,omitempty"`
	AccumulationPhase string `json:"accumulation_phase,omitempty"`
	GeneratedAt       string `json:"generated_at,omitempty"`
}

type LeaseSummary struct {
	Available    bool   `json:"available"`
	InstanceID   string `json:"instance_id,omitempty"`
	FencingToken int64  `json:"fencing_token,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	Fresh        bool   `json:"fresh"`
}

type PaperSummary struct {
	TotalOrders      int    `json:"total_orders"`
	TerminalOrders   int    `json:"terminal_orders"`
	Readiness        string `json:"readiness"`
	UnknownStatuses  int    `json:"unknown_statuses"`
	MissingCloseTime int    `json:"missing_terminal_timestamps"`
}

type PaperOrder struct {
	ID        string  `json:"id"`
	Timestamp string  `json:"timestamp"`
	Symbol    string  `json:"symbol"`
	Side      string  `json:"side"`
	Layer     int     `json:"layer"`
	Price     float64 `json:"price"`
	Quantity  float64 `json:"quantity"`
	Notional  float64 `json:"notional"`
	Status    string  `json:"status"`
	ExpiresAt string  `json:"expires_at"`
	ClosedAt  string  `json:"closed_at,omitempty"`
	Reason    string  `json:"reason,omitempty"`
}

type PaperOrdersPage struct {
	Orders []PaperOrder `json:"orders"`
	Limit  int          `json:"limit"`
}

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	now := s.now().UTC()
	halted, err := s.db.IsHalted()
	if err != nil {
		return Overview{}, fmt.Errorf("read halt status: %w", err)
	}
	orders, err := s.db.PaperOrders()
	if err != nil {
		return Overview{}, fmt.Errorf("read paper orders: %w", err)
	}
	score := paper.BuildScorecard(now, orders)
	out := Overview{Halted: halted, Paper: paperSummary(score)}
	if analysis, err := s.db.LatestAnalysis(); err == nil {
		out.Market.Available = true
		out.Market.Regime = analysis.MarketRegime
		out.Market.Permission = string(analysis.ActionPermission)
		out.Market.AccumulationPhase = string(analysis.BTCAccumulation.Phase)
		out.Market.GeneratedAt = analysis.Timestamp.UTC().Format(time.RFC3339)
	}
	if plan, err := s.db.LatestPlan(); err == nil {
		out.Market.PlanState = string(plan.State)
	}
	lease, ok, err := s.db.CurrentExecutionLease(ctx, s.lease)
	if err != nil {
		return Overview{}, fmt.Errorf("read execution lease: %w", err)
	}
	if ok {
		out.Lease = leaseSummary(now, lease)
	}
	return out, nil
}

func (s *Service) Scorecard() (paper.Scorecard, error) {
	orders, err := s.db.PaperOrders()
	if err != nil {
		return paper.Scorecard{}, fmt.Errorf("read paper orders: %w", err)
	}
	return paper.BuildScorecard(s.now().UTC(), orders), nil
}

func (s *Service) PaperOrders(limit int) (PaperOrdersPage, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	orders, err := s.db.PaperOrders()
	if err != nil {
		return PaperOrdersPage{}, fmt.Errorf("read paper orders: %w", err)
	}
	if len(orders) > limit {
		orders = orders[:limit]
	}
	out := PaperOrdersPage{Orders: make([]PaperOrder, 0, len(orders)), Limit: limit}
	for _, order := range orders {
		out.Orders = append(out.Orders, paperOrderDTO(order))
	}
	return out, nil
}

func paperSummary(score paper.Scorecard) PaperSummary {
	return PaperSummary{TotalOrders: score.TotalOrders, TerminalOrders: score.TerminalOrders, Readiness: score.Readiness, UnknownStatuses: score.UnknownStatuses, MissingCloseTime: score.MissingTerminalTimestamps}
}

func leaseSummary(now time.Time, lease ownership.Lease) LeaseSummary {
	return LeaseSummary{Available: true, InstanceID: lease.InstanceID, FencingToken: lease.FencingToken, ExpiresAt: lease.ExpiresAt.UTC().Format(time.RFC3339), Fresh: lease.ExpiresAt.After(now)}
}

func paperOrderDTO(order agent2.PaperOrder) PaperOrder {
	out := PaperOrder{ID: order.ID, Timestamp: order.Timestamp.UTC().Format(time.RFC3339), Symbol: strings.ToUpper(strings.TrimSpace(order.Symbol)), Side: order.Side, Layer: order.Layer, Price: order.Price, Quantity: order.Quantity, Notional: order.Notional, Status: order.Status, ExpiresAt: order.ExpiresAt.UTC().Format(time.RFC3339), Reason: order.Reason}
	if !order.ClosedAt.IsZero() {
		out.ClosedAt = order.ClosedAt.UTC().Format(time.RFC3339)
	}
	return out
}
