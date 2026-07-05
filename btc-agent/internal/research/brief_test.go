package research

import "testing"

func TestClassifyTagsAndRisk(t *testing.T) {
	tags, risk := classify("BTC ETH SOL RENDER OKX outage liquidation")
	if risk != RiskWarn {
		t.Fatalf("risk=%s want %s", risk, RiskWarn)
	}
	want := map[string]bool{"BTC": true, "ETH": true, "SOL": true, "RENDER": true, "OKX": true}
	for _, tag := range tags {
		delete(want, tag)
	}
	if len(want) > 0 {
		t.Fatalf("missing tags: %v got %v", want, tags)
	}
}
