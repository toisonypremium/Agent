package okxassets

import "testing"

func TestApplyUSDTPricesCalculatesValueAndPreservesUnknownPrice(t *testing.T) {
	snapshot := Snapshot{Source: SourceOKXSpotReadOnly, Assets: []Asset{
		{Currency: "USDT", Available: "10", Frozen: "0", Total: "10", ThesisLink: ThesisNotApplicable},
		{Currency: "BTC", Available: "0.01", Frozen: "0", Total: "0.01", ThesisLink: ThesisUnlinked},
		{Currency: "DUST", Available: "1", Frozen: "0", Total: "1", ThesisLink: ThesisUnlinked},
	}}
	got, warnings, err := ApplyUSDTPrices(snapshot, map[string]string{"BTC": "60000"})
	if err != nil {
		t.Fatal(err)
	}
	byCurrency := map[string]Asset{}
	for _, asset := range got.Assets {
		byCurrency[asset.Currency] = asset
	}
	if byCurrency["USDT"].ValueUSDT != "10" || byCurrency["USDT"].PriceUSDT != "1" {
		t.Fatalf("usdt=%+v", byCurrency["USDT"])
	}
	if byCurrency["BTC"].ValueUSDT != "600" || byCurrency["BTC"].PriceUSDT != "60000" {
		t.Fatalf("btc=%+v", byCurrency["BTC"])
	}
	if byCurrency["DUST"].ValuationState != ValuationUnavailable || len(warnings) != 1 {
		t.Fatalf("dust=%+v warnings=%v", byCurrency["DUST"], warnings)
	}
}

func TestApplyUSDTPricesRejectsNegativePrice(t *testing.T) {
	snapshot := Snapshot{Source: SourceOKXSpotReadOnly, Assets: []Asset{{Currency: "BTC", Available: "1", Frozen: "0", Total: "1", ThesisLink: ThesisUnlinked}}}
	if _, _, err := ApplyUSDTPrices(snapshot, map[string]string{"BTC": "-1"}); err == nil {
		t.Fatal("negative price accepted")
	}
}
