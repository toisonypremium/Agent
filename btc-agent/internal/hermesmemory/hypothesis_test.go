package hermesmemory

import (
	"strings"
	"testing"
)

func validHypothesis() Hypothesis {
	return Hypothesis{Title: "Reclaim", Statement: "BTC reclaim improves forward return", FalsificationRule: "Reject when 7d OOS return <= neutral baseline", FeatureContract: []string{"mm_quality", "trend"}, Symbols: []string{"btcusdt"}, Horizons: []string{"7D"}, SourceEpisodeID: "ep1", Authority: "research_only"}
}
func TestHypothesisNormalizesAndFingerprints(t *testing.T) {
	h := NormalizeHypothesis(validHypothesis())
	if h.Symbols[0] != "BTCUSDT" || h.Horizons[0] != "7d" || h.Status != "DRAFT" || !strings.HasPrefix(h.HypothesisID, "hyp:") {
		t.Fatalf("bad normalize %+v", h)
	}
}
func TestHypothesisRequiresFalsification(t *testing.T) {
	h := NormalizeHypothesis(validHypothesis())
	h.FalsificationRule = ""
	if ValidateHypothesis(h, nil) == nil {
		t.Fatal("missing falsification accepted")
	}
}
func TestHypothesisRejectsAuthority(t *testing.T) {
	h := NormalizeHypothesis(validHypothesis())
	h.Authority = "execution"
	if ValidateHypothesis(h, nil) == nil {
		t.Fatal("execution authority accepted")
	}
}
func TestHypothesisRejectsUnknownFeature(t *testing.T) {
	h := NormalizeHypothesis(validHypothesis())
	if ValidateHypothesis(h, map[string]bool{"trend": true}) == nil {
		t.Fatal("unknown feature accepted")
	}
}
func TestHypothesisRejectsSelfSupersession(t *testing.T) {
	h := NormalizeHypothesis(validHypothesis())
	h.SupersedesID = h.HypothesisID
	if ValidateHypothesis(h, nil) == nil {
		t.Fatal("self supersession accepted")
	}
}
