package microstructure

import (
	"testing"
	"time"
)

func fpSnapshots(coreCVD, coreTaker, coreBid, ask bool) []Snapshot {
	now := time.Now()
	out := make([]Snapshot, 10)
	for i := 0; i < 10; i++ {
		cvd := 0.0
		if coreCVD {
			cvd = float64(10-i) * 100000
		}
		ratio := .50
		if coreTaker && i == 0 {
			ratio = .60
		}
		bias := "NEUTRAL"
		if coreBid && i < 4 {
			bias = "BID_SUPPORT"
		}
		if ask && i == 0 {
			bias = "ASK_PRESSURE"
		}
		out[i] = Snapshot{Timestamp: now.Add(-time.Duration(i) * 15 * time.Minute), SpotFlow: SpotFlow{CVDQuoteUSDT: cvd, QuoteVolumeUSDT: 1000000, TakerBuyRatio: ratio}, OrderBook: OrderBook{BestBid: 100 - float64(i)*.02}, Signals: Signals{OrderBookBias: bias}}
	}
	return out
}
func TestFootprintContextAloneNoVerdict(t *testing.T) {
	s := fpSnapshots(false, false, false, false)
	for i := range s {
		s[i].Futures.FundingRate = -.001
		s[i].Futures.BasisPct = -.1
	}
	r := AnalyzeMMFootprint(s)
	if r.Verdict != "NO_SIGNAL" {
		t.Fatalf("context created signal: %+v", r)
	}
}
func TestFootprintRequiresTwoCoreForAccumulating(t *testing.T) {
	r := AnalyzeMMFootprint(fpSnapshots(true, false, false, false))
	if r.Verdict == "MM_ACCUMULATING" {
		t.Fatalf("one core promoted: %+v", r)
	}
	r = AnalyzeMMFootprint(fpSnapshots(true, true, true, false))
	if r.Verdict != "MM_ACCUMULATING" {
		t.Fatalf("expected accumulating: %+v", r)
	}
}
func TestFootprintAskPressurePreventsAccumulating(t *testing.T) {
	r := AnalyzeMMFootprint(fpSnapshots(true, true, true, true))
	if r.Verdict == "MM_ACCUMULATING" {
		t.Fatalf("ask pressure promoted: %+v", r)
	}
}
