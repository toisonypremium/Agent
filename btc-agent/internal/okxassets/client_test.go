package okxassets

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type recordingTransport struct {
	request *http.Request
	body    string
}

func (t *recordingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.request = r
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(t.body)), Header: make(http.Header)}, nil
}
func TestReadOnlyClientCallsOnlySpotBalanceEndpoint(t *testing.T) {
	transport := &recordingTransport{body: `{"code":"0","data":[{"details":[{"ccy":"USDT","availBal":"1","frozenBal":"0","cashBal":"1"}]}]}`}
	client := NewReadOnlyClient("https://www.okx.com", "key", "secret", "pass", &http.Client{Transport: transport}, func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) })
	if _, err := client.SpotBalance(context.Background()); err != nil {
		t.Fatal(err)
	}
	if transport.request.Method != "GET" || transport.request.URL.Path != "/api/v5/account/balance" {
		t.Fatalf("request=%s %s", transport.request.Method, transport.request.URL)
	}
	if transport.request.Header.Get("OK-ACCESS-SIGN") == "" {
		t.Fatal("missing signed read-only request")
	}
}
func TestReadOnlyClientRejectsNonOKXBaseURL(t *testing.T) {
	if _, err := NewReadOnlyClient("http://www.okx.com", "k", "s", "p", http.DefaultClient, time.Now).SpotBalance(context.Background()); err == nil {
		t.Fatal("insecure URL accepted")
	}
}

func TestReadOnlyClientPricesUsePublicTickerWithoutCredentials(t *testing.T) {
	transport := &recordingTransport{body: `{"code":"0","data":[{"instId":"BTC-USDT","last":"60000"},{"instId":"BTC-USDC","last":"1"}]}`}
	client := NewReadOnlyClient("https://www.okx.com", "", "", "", &http.Client{Transport: transport}, time.Now)
	prices, err := client.SpotUSDTPrices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if prices["BTC"] != "60000" || transport.request.URL.Path != "/api/v5/market/tickers" || transport.request.Header.Get("OK-ACCESS-KEY") != "" {
		t.Fatalf("prices=%v request=%+v", prices, transport.request)
	}
}
