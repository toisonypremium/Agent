package okxassets

import "testing"

func TestParseSpotBalanceRejectsNonSpotAndInvalidTotals(t *testing.T) {
	body := []byte(`{"code":"0","data":[{"details":[{"ccy":"USDT","availBal":"12.50","frozenBal":"2.50","cashBal":"15.00"},{"ccy":"BTC","availBal":"0.2","frozenBal":"0","cashBal":"0.1"}]}]}`)
	if _, err := ParseSpotBalance(body); err == nil {
		t.Fatal("invalid total accepted")
	}
}

func TestParseSpotBalanceProducesSafeObservation(t *testing.T) {
	body := []byte(`{"code":"0","data":[{"details":[{"ccy":"USDT","availBal":"12.50","frozenBal":"2.50","cashBal":"15.00"},{"ccy":"BTC","availBal":"0.2","frozenBal":"0","cashBal":"0.2"}]}]}`)
	s, err := ParseSpotBalance(body)
	if err != nil {
		t.Fatal(err)
	}
	if s.Source != SourceOKXSpotReadOnly || len(s.Assets) != 2 {
		t.Fatalf("snapshot=%+v", s)
	}
	for _, asset := range s.Assets {
		if asset.Currency == "BTC" && asset.ThesisLink != ThesisUnlinked {
			t.Fatalf("asset=%+v", asset)
		}
	}
}
