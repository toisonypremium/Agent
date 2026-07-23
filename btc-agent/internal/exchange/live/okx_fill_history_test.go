package live

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestSpotFillHistoryPaginatesAndDeduplicates(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		query, _ := url.ParseQuery(r.URL.RawQuery)
		after := query.Get("after")
		data := []map[string]string{}
		if after == "" {
			for i := 200; i >= 101; i-- {
				data = append(data, map[string]string{"instId": "BTC-USDT", "tradeId": strconv.Itoa(i), "ordId": fmt.Sprint("o", i), "side": "buy", "fillPx": "10", "fillSz": "1", "ts": strconv.Itoa(i * 1000)})
			}
		} else {
			data = append(data, map[string]string{"instId": "BTC-USDT", "tradeId": "101", "ordId": "o101", "side": "buy", "fillPx": "10", "fillSz": "1", "ts": "101000"}, map[string]string{"instId": "BTC-USDT", "tradeId": "100", "ordId": "o100", "side": "buy", "fillPx": "9", "fillSz": "2", "ts": "100000"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "0", "data": data})
	}))
	defer server.Close()
	client := &OKXClient{baseURL: server.URL, key: "k", secret: "s", passphrase: "p", http: server.Client()}
	fills, err := client.SpotFillHistory(context.Background(), "BTC-USDT")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(fills) != 101 || fills[0].TradeID != "100" {
		t.Fatalf("calls=%d fills=%d first=%+v", calls, len(fills), fills[0])
	}
}

func TestParseOKXTradeFillsRejectsMalformedReconciliationData(t *testing.T) {
	cases := []struct {
		name string
		row  string
	}{
		{"missing trade ID", `{"instId":"BTC-USDT","tradeId":"","ordId":"o1","side":"buy","fillPx":"10","fillSz":"1","ts":"1000"}`},
		{"unknown side", `{"instId":"BTC-USDT","tradeId":"t1","ordId":"o1","side":"hold","fillPx":"10","fillSz":"1","ts":"1000"}`},
		{"nonfinite price", `{"instId":"BTC-USDT","tradeId":"t1","ordId":"o1","side":"buy","fillPx":"NaN","fillSz":"1","ts":"1000"}`},
		{"negative quantity", `{"instId":"BTC-USDT","tradeId":"t1","ordId":"o1","side":"buy","fillPx":"10","fillSz":"-1","ts":"1000"}`},
		{"missing timestamp", `{"instId":"BTC-USDT","tradeId":"t1","ordId":"o1","side":"buy","fillPx":"10","fillSz":"1","ts":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(`{"code":"0","data":[` + tc.row + `]}`)
			if _, err := parseOKXTradeFills(data); err == nil {
				t.Fatal("malformed fill must fail closed")
			}
		})
	}
}
