package live

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewOKXFromEnvRejectsUnsafeCredentialValues(t *testing.T) {
	t.Setenv("OKX_TEST_KEY", " key")
	t.Setenv("OKX_TEST_SECRET", "secret")
	t.Setenv("OKX_TEST_PASSPHRASE", "pass")
	_, err := NewOKXFromEnv("", "OKX_TEST_KEY", "OKX_TEST_SECRET", "OKX_TEST_PASSPHRASE")
	if err == nil || !strings.Contains(err.Error(), "OKX_TEST_KEY") || strings.Contains(err.Error(), " key") {
		t.Fatalf("expected sanitized key env error, got %v", err)
	}

	t.Setenv("OKX_TEST_KEY", "key")
	t.Setenv("OKX_TEST_SECRET", "sec\nret")
	_, err = NewOKXFromEnv("", "OKX_TEST_KEY", "OKX_TEST_SECRET", "OKX_TEST_PASSPHRASE")
	if err == nil || !strings.Contains(err.Error(), "OKX_TEST_SECRET") || strings.Contains(err.Error(), "sec") {
		t.Fatalf("expected sanitized secret env error, got %v", err)
	}
}

func TestNewOKXFromEnvAcceptsCleanCredentialValues(t *testing.T) {
	t.Setenv("OKX_TEST_KEY", "key")
	t.Setenv("OKX_TEST_SECRET", "secret")
	t.Setenv("OKX_TEST_PASSPHRASE", "pass")
	client, err := NewOKXFromEnv("", "OKX_TEST_KEY", "OKX_TEST_SECRET", "OKX_TEST_PASSPHRASE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil || client.key != "key" || client.secret != "secret" || client.passphrase != "pass" {
		t.Fatalf("bad client: %+v", client)
	}
}

func TestOKXSign(t *testing.T) {
	timestamp := "2020-12-08T09:08:57.715Z"
	method := "GET"
	path := "/api/v5/account/balance"
	secret := "test-secret"
	got := okxSign(timestamp, method, path, "", secret)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + method + path))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Fatalf("sign=%s want %s", got, want)
	}
}

func TestParseBalances(t *testing.T) {
	data := []byte(`{"code":"0","msg":"","data":[{"details":[{"ccy":"USDT","availBal":"21.5","availEq":"20","cashBal":"19"},{"ccy":"BTC","availEq":"0.1"}]}]}`)
	got, err := parseBalances(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Asset != "USDT" || got[0].Free != 21.5 {
		t.Fatalf("bad USDT balance: %+v", got[0])
	}
	if got[1].Asset != "BTC" || got[1].Free != 0.1 {
		t.Fatalf("bad BTC balance: %+v", got[1])
	}
}

func TestParseInstrumentFilters(t *testing.T) {
	data := []byte(`{"code":"0","msg":"","data":[{"instId":"ETH-USDT","tickSz":"0.01","lotSz":"0.0001","minSz":"0.001","minNotional":"5"}]}`)
	got, err := parseInstrumentFilters(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	f := got[0]
	if f.Symbol != "ETHUSDT" || f.InstID != "ETH-USDT" || f.TickSize != 0.01 || f.StepSize != 0.0001 || f.MinSize != 0.001 || f.MinNotional != 5 {
		t.Fatalf("bad filter: %+v", f)
	}
}

func TestSymbolConversion(t *testing.T) {
	if got := OKXInstID("ETHUSDT"); got != "ETH-USDT" {
		t.Fatalf("OKXInstID=%s", got)
	}
	if got := InternalSymbol("ETH-USDT"); got != "ETHUSDT" {
		t.Fatalf("InternalSymbol=%s", got)
	}
}

func TestParseOrderResultSuccess(t *testing.T) {
	data := []byte(`{"code":"0","msg":"","data":[{"ordId":"123","clOrdId":"btcagenteth1","sCode":"0","sMsg":""}]}`)
	got, err := parseOrderResult(data, "ETH-USDT")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Submitted || got.OrderID != "123" || got.ClientOrderID != "btcagenteth1" || got.InstID != "ETH-USDT" {
		t.Fatalf("bad order result: %+v", got)
	}
}

func TestParseOrderResultReject(t *testing.T) {
	data := []byte(`{"code":"0","msg":"","data":[{"ordId":"","clOrdId":"btcagenteth1","sCode":"51000","sMsg":"bad order"}]}`)
	got, err := parseOrderResult(data, "ETH-USDT")
	if err == nil {
		t.Fatal("expected error")
	}
	if got.Submitted || got.Code != "51000" {
		t.Fatalf("bad rejection: %+v", got)
	}
}

func TestRedact(t *testing.T) {
	got := redact([]byte("bad key secret passphrase"), "key", "secret", "passphrase")
	for _, s := range []string{"key", "secret", "passphrase"} {
		if strings.Contains(got, s) {
			t.Fatalf("secret leaked in %q", got)
		}
	}
}

func TestParseOKXOrderStatus(t *testing.T) {
	data := []byte(`{"code":"0","msg":"","data":[
		{"instId":"ETH-USDT","ordId":"1","clOrdId":"c1","state":"live","side":"buy","ordType":"limit","px":"2000.5","sz":"0.01","accFillSz":"0","avgPx":"","fee":"0","uTime":"1700000000000"},
		{"instId":"ETH-USDT","ordId":"2","clOrdId":"c2","state":"partially_filled","side":"buy","ordType":"limit","px":"2000","sz":"0.02","accFillSz":"0.01","avgPx":"1999","fee":"-0.001","feeCcy":"USDT","uTime":"1700000000001"},
		{"instId":"ETH-USDT","ordId":"3","clOrdId":"c3","state":"filled","side":"buy","ordType":"limit","px":"2000","sz":"0.03","accFillSz":"0.03","avgPx":"1998","fee":"-0.002","uTime":"1700000000002"},
		{"instId":"ETH-USDT","ordId":"4","clOrdId":"c4","state":"canceled","side":"buy","ordType":"limit","px":"2000","sz":"0.04","accFillSz":"0","avgPx":"","fee":"0","uTime":"1700000000003"},
		{"instId":"ETH-USDT","ordId":"5","clOrdId":"c5","state":"rejected","side":"buy","ordType":"limit","px":"2000","sz":"0.05","accFillSz":"0","avgPx":"","fee":"0","uTime":"1700000000004"},
		{"instId":"ETH-USDT","ordId":"6","clOrdId":"c6","state":"mystery","side":"buy","ordType":"limit","px":"2000","sz":"0.06","accFillSz":"0","avgPx":"","fee":"0","uTime":"1700000000005"}
	]}`)
	got, err := parseOKXOrderStatus(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{StatusSubmitted, StatusPartialFill, StatusFilled, StatusCancelled, StatusRejected, StatusUnknownNeedsManualCheck}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d", len(got), len(want))
	}
	for i, status := range want {
		if got[i].Status != status {
			t.Fatalf("status[%d]=%s want %s", i, got[i].Status, status)
		}
	}
	if got[1].FilledQuantity != 0.01 || got[1].AccumulatedFillSz != 0.01 || got[1].AvgPrice != 1999 || got[1].Fee != -0.001 || got[1].FeeCurrency != "USDT" || got[1].UpdatedAt != 1700000000 {
		t.Fatalf("bad parsed fills: %+v", got[1])
	}
}

func TestOrderStatusSignsFullQueryPath(t *testing.T) {
	const secret = "test-secret"
	var sawPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.RequestURI()
		if sawPath != "/api/v5/trade/order?instId=ETH-USDT&ordId=ord-123" {
			t.Fatalf("path=%s", sawPath)
		}
		ts := r.Header.Get("OK-ACCESS-TIMESTAMP")
		wantSign := okxSign(ts, http.MethodGet, sawPath, "", secret)
		if r.Header.Get("OK-ACCESS-SIGN") != wantSign {
			t.Fatalf("signature did not include full query path")
		}
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"ETH-USDT","ordId":"ord-123","clOrdId":"client-123","state":"live","side":"buy","ordType":"limit","px":"2000","sz":"0.01","accFillSz":"0","avgPx":"","fee":"0","uTime":"1700000000000"}]}`))
	}))
	defer server.Close()

	client := &OKXClient{baseURL: server.URL, key: "key", secret: secret, passphrase: "pass", http: server.Client()}
	status, err := client.OrderStatus(context.Background(), "ETH-USDT", "ord-123", "")
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusSubmitted || status.OrderID != "ord-123" || sawPath == "" {
		t.Fatalf("bad status: %+v", status)
	}
}

func TestOrderStatusCanLookupByClientOrderID(t *testing.T) {
	const secret = "test-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath := r.URL.RequestURI()
		if sawPath != "/api/v5/trade/order?instId=ETH-USDT&clOrdId=client-123" {
			t.Fatalf("path=%s", sawPath)
		}
		ts := r.Header.Get("OK-ACCESS-TIMESTAMP")
		wantSign := okxSign(ts, http.MethodGet, sawPath, "", secret)
		if r.Header.Get("OK-ACCESS-SIGN") != wantSign {
			t.Fatalf("signature did not include clOrdId query path")
		}
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"ETH-USDT","ordId":"ord-123","clOrdId":"client-123","state":"filled","side":"buy","ordType":"limit","px":"2000","sz":"0.01","accFillSz":"0.01","avgPx":"1999","fee":"-0.001","feeCcy":"USDT","uTime":"1700000000000"}]}`))
	}))
	defer server.Close()

	client := &OKXClient{baseURL: server.URL, key: "key", secret: secret, passphrase: "pass", http: server.Client()}
	status, err := client.OrderStatus(context.Background(), "ETH-USDT", "", "client-123")
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != StatusFilled || status.ClientOrderID != "client-123" || status.OrderID != "ord-123" {
		t.Fatalf("bad status: %+v", status)
	}
}

func TestPendingOrdersSignsFullQueryPath(t *testing.T) {
	const secret = "test-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath := r.URL.RequestURI()
		if sawPath != "/api/v5/trade/orders-pending?instType=SPOT&instId=ETH-USDT" {
			t.Fatalf("path=%s", sawPath)
		}
		ts := r.Header.Get("OK-ACCESS-TIMESTAMP")
		wantSign := okxSign(ts, http.MethodGet, sawPath, "", secret)
		if r.Header.Get("OK-ACCESS-SIGN") != wantSign {
			t.Fatalf("signature did not include pending-orders query path")
		}
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"instId":"ETH-USDT","ordId":"ord-123","clOrdId":"client-123","state":"live","side":"buy","ordType":"limit","px":"2000","sz":"0.01","accFillSz":"0","avgPx":"","fee":"0","uTime":"1700000000000"}]}`))
	}))
	defer server.Close()

	client := &OKXClient{baseURL: server.URL, key: "key", secret: secret, passphrase: "pass", http: server.Client()}
	statuses, err := client.PendingOrders(context.Background(), "ETH-USDT")
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].Status != StatusSubmitted {
		t.Fatalf("bad pending statuses: %+v", statuses)
	}
}

func TestParseCancelOrderResultSuccess(t *testing.T) {
	data := []byte(`{"code":"0","msg":"","data":[{"ordId":"123","clOrdId":"client-1","sCode":"0","sMsg":""}]}`)
	got, err := parseCancelOrderResult(data, "ETH-USDT")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Canceled || got.OrderID != "123" || got.ClientOrderID != "client-1" {
		t.Fatalf("bad cancel result: %+v", got)
	}
}

func TestCancelOrderSignsBody(t *testing.T) {
	const secret = "test-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/api/v5/trade/cancel-order" || r.Method != http.MethodPost {
			t.Fatalf("bad request %s %s", r.Method, r.URL.RequestURI())
		}
		body := `{"clOrdId":"client-123","instId":"ETH-USDT"}`
		buf := make([]byte, len(body))
		_, _ = r.Body.Read(buf)
		if string(buf) != body {
			t.Fatalf("body=%s", string(buf))
		}
		ts := r.Header.Get("OK-ACCESS-TIMESTAMP")
		wantSign := okxSign(ts, http.MethodPost, "/api/v5/trade/cancel-order", body, secret)
		if r.Header.Get("OK-ACCESS-SIGN") != wantSign {
			t.Fatalf("signature did not include cancel body")
		}
		_, _ = w.Write([]byte(`{"code":"0","msg":"","data":[{"ordId":"","clOrdId":"client-123","sCode":"0","sMsg":""}]}`))
	}))
	defer server.Close()
	client := &OKXClient{baseURL: server.URL, key: "key", secret: secret, passphrase: "pass", http: server.Client()}
	got, err := client.CancelOrder(context.Background(), CancelOrderRequest{InstID: "ETH-USDT", ClientOrderID: "client-123"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Canceled || got.ClientOrderID != "client-123" {
		t.Fatalf("bad cancel: %+v", got)
	}
}
