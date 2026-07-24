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
	db        *storage.DB
	now       Clock
	lease     string
	haltDB    *storage.DB
	health    RuntimeHealthSource
	okxAssets OKXAssetSource
}

func NewService(db *storage.DB, now Clock) (*Service, error) {
	if db == nil {
		return nil, fmt.Errorf("web console database required")
	}
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, haltDB: db, now: now, lease: "okx-live"}, nil
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
	return s.PaperOrdersFiltered(limit, "", time.Time{})
}

func (s *Service) PaperOrdersFiltered(limit int, status string, since time.Time) (PaperOrdersPage, error) {
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
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "" && !paperOrderStatusAllowed(status) {
		return PaperOrdersPage{}, fmt.Errorf("invalid paper order status")
	}
	filtered := make([]agent2.PaperOrder, 0, len(orders))
	for _, order := range orders {
		if status != "" && strings.ToUpper(order.Status) != status {
			continue
		}
		if !since.IsZero() && order.Timestamp.Before(since) {
			continue
		}
		filtered = append(filtered, order)
		if len(filtered) == limit {
			break
		}
	}
	out := PaperOrdersPage{Orders: make([]PaperOrder, 0, len(filtered)), Limit: limit}
	for _, order := range filtered {
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

type Event struct {
	ID        int64  `json:"id"`
	Timestamp string `json:"timestamp"`
	Source    string `json:"source"`
	Type      string `json:"type"`
	Severity  string `json:"severity"`
}

type EventsPage struct {
	Events []Event `json:"events"`
	Limit  int     `json:"limit"`
}

// Events exposes ledger metadata only. Runtime payload JSON is deliberately
// excluded because event producers may include unreviewed nested data.
func (s *Service) Events(limit int) (EventsPage, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.db.PendingRuntimeEvents(limit)
	if err != nil {
		return EventsPage{}, fmt.Errorf("read runtime events: %w", err)
	}
	out := EventsPage{Events: make([]Event, 0, len(rows)), Limit: limit}
	for _, row := range rows {
		out.Events = append(out.Events, Event{ID: row.ID, Timestamp: row.Timestamp.UTC().Format(time.RFC3339), Source: row.Source, Type: row.Type, Severity: row.Severity})
	}
	return out, nil
}

// SetHaltDB installs the write-only, narrow halt authority. A nil value disables halt.
func (s *Service) SetHaltDB(db *storage.DB) { s.haltDB = db }
func (s *Service) RequestHalt(identity, reasonCode, summary, key string) (storage.WebHaltReceipt, error) {
	if s.haltDB == nil {
		return storage.WebHaltReceipt{}, fmt.Errorf("halt authority unavailable")
	}
	return s.haltDB.RequestWebHalt(identity, reasonCode, summary, key, s.now().UTC())
}

func paperOrderStatusAllowed(status string) bool {
	switch status {
	case "OPEN", "FILLED", "INVALIDATED", "EXPIRED", "CANCELLED":
		return true
	}
	return false
}
