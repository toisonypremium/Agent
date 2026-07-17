package live

import (
	"math"
	"testing"
)

func TestReconstructInventoryBasisComplete(t *testing.T) {
	fills := []TradeFill{{Side: "BUY", Price: 10, Quantity: 10}, {Side: "BUY", Price: 20, Quantity: 10}, {Side: "SELL", Price: 30, Quantity: 5}}
	r := ReconstructInventoryBasis(fills, "ABC", 15)
	if !r.Complete || math.Abs(r.AvgPrice-15) > .0001 {
		t.Fatalf("unexpected basis %+v", r)
	}
}
func TestReconstructInventoryBasisRejectsIncompleteHistory(t *testing.T) {
	r := ReconstructInventoryBasis([]TradeFill{{Side: "BUY", Price: 10, Quantity: 2}}, "ABC", 10)
	if r.Complete {
		t.Fatalf("partial history must not be trusted %+v", r)
	}
}
func TestReconstructInventoryBasisIncludesFees(t *testing.T) {
	r := ReconstructInventoryBasis([]TradeFill{{Side: "BUY", Price: 10, Quantity: 10, Fee: -.1, FeeCurrency: "ABC"}}, "ABC", 9.9)
	if !r.Complete || math.Abs(r.AvgPrice-100.0/9.9) > .0001 {
		t.Fatalf("base fee basis wrong %+v", r)
	}
}
