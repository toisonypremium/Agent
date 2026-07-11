package microstructure

import (
	"strings"
	"testing"
	"time"
)

func TestBuildSignalsComputesFlowAndBook(t *testing.T) {
	s := BuildSignals(Snapshot{
		Symbol:    "BTCUSDT",
		Timestamp: time.Unix(100, 0),
		SpotFlow:  SpotFlow{VolumeBase: 10, QuoteVolumeUSDT: 1000, TakerBuyBase: 6, TakerBuyQuoteUSDT: 650},
		OrderBook: OrderBook{BestBid: 99, BestAsk: 101, BidDepthUSDT: 700, AskDepthUSDT: 300},
		Futures:   FuturesObservation{FundingRate: 0.0001, BasisPct: 0.2},
	})
	if s.SpotFlow.TakerSellQuoteUSDT != 350 {
		t.Fatalf("unexpected taker sell quote: %.2f", s.SpotFlow.TakerSellQuoteUSDT)
	}
	if s.SpotFlow.TakerBuyRatio != 0.65 {
		t.Fatalf("unexpected taker buy ratio: %.4f", s.SpotFlow.TakerBuyRatio)
	}
	if s.SpotFlow.CVDQuoteUSDT != 300 {
		t.Fatalf("unexpected cvd: %.2f", s.SpotFlow.CVDQuoteUSDT)
	}
	if s.OrderBook.SpreadBps != 200 {
		t.Fatalf("unexpected spread bps: %.2f", s.OrderBook.SpreadBps)
	}
	if s.OrderBook.Imbalance != 0.4 {
		t.Fatalf("unexpected imbalance: %.4f", s.OrderBook.Imbalance)
	}
	if !s.Signals.Supportive || !s.Signals.AbsorptionHint || s.Signals.Risky {
		t.Fatalf("expected supportive absorption signal: %+v", s.Signals)
	}
}

func TestEvaluateHealthAndSummaryBlockStale(t *testing.T) {
	now := time.Unix(1000, 0)
	fresh := EvaluateHealth(BuildSignals(Snapshot{Symbol: "BTCUSDT", Timestamp: now.Add(-5 * time.Minute), SpotFlow: SpotFlow{QuoteVolumeUSDT: 100, TakerBuyQuoteUSDT: 60}, OrderBook: OrderBook{BestBid: 99, BestAsk: 100}}), now, 30*time.Minute)
	stale := EvaluateHealth(BuildSignals(Snapshot{Symbol: "ETHUSDT", Timestamp: now.Add(-2 * time.Hour), SpotFlow: SpotFlow{QuoteVolumeUSDT: 100, TakerBuyQuoteUSDT: 40}}), now, 30*time.Minute)
	if !fresh.Health.Fresh {
		t.Fatalf("expected fresh snapshot: %+v", fresh.Health)
	}
	if stale.Health.Fresh || len(stale.Health.Blockers) == 0 {
		t.Fatalf("expected stale blocker: %+v", stale.Health)
	}
	summary := BuildSummary(true, "BTCUSDT", []Snapshot{fresh, stale}, 2, now)
	if summary.Status != StatusBlock {
		t.Fatalf("expected block status, got %s", summary.Status)
	}
	if !BlocksActive(summary) {
		t.Fatal("expected BlocksActive true")
	}
	if !strings.Contains(summary.Summary, "fresh=1") {
		t.Fatalf("summary missing fresh count: %s", summary.Summary)
	}
}
