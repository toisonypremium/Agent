package okxassets

import "testing"

func TestParseSpotBalanceRejectsNegativeBalance(t *testing.T) {
	body := []byte(`{"code":"0","data":[{"details":[{"ccy":"USDT","availBal":"-1","frozenBal":"0","cashBal":"0"}]}]}`)
	if _, err := ParseSpotBalance(body); err == nil {
		t.Fatal("negative balance accepted")
	}
}

func TestParseSpotBalanceDerivesTotalFromAvailableAndFrozen(t *testing.T) {
	body := []byte(`{"code":"0","data":[{"details":[{"ccy":"USDT","availBal":"12.50","frozenBal":"2.50","cashBal":"999.99"}]}]}`)
	s, err := ParseSpotBalance(body)
	if err != nil {
		t.Fatal(err)
	}
	if s.Assets[0].Total != "15" {
		t.Fatalf("asset=%+v", s.Assets[0])
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

func TestParseSpotBalanceExcludesZeroHoldings(t *testing.T) {
	body := []byte(`{"code":"0","data":[{"details":[{"ccy":"BTC","availBal":"0","frozenBal":"0","cashBal":"0"},{"ccy":"USDT","availBal":"1","frozenBal":"0","cashBal":"1"}]}]}`)
	s, err := ParseSpotBalance(body)
	if err != nil || len(s.Assets) != 1 || s.Assets[0].Currency != "USDT" {
		t.Fatalf("snapshot=%+v err=%v", s, err)
	}
}
