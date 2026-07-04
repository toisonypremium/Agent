package exchange

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestBinanceKlinesBuildsFullQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/v3/klines" || q.Get("symbol") != "BTCUSDT" || q.Get("interval") != "1d" || q.Get("limit") != "100" || q.Get("startTime") != "" {
			t.Fatalf("bad query path=%s raw=%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(klineJSON(1700000000000)))
	}))
	defer server.Close()

	client := NewBinance(server.URL)
	got, err := client.Klines(context.Background(), "btcusdt", "1d", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Symbol != "BTCUSDT" || got[0].Interval != "1d" || got[0].OpenTime.UnixMilli() != 1700000000000 {
		t.Fatalf("bad candles: %+v", got)
	}
}

func TestBinanceKlinesSinceAddsStartTime(t *testing.T) {
	start := time.Unix(1700000000, 0).Add(time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("startTime") != strconv.FormatInt(start.UnixMilli(), 10) {
			t.Fatalf("startTime=%q want %d raw=%s", q.Get("startTime"), start.UnixMilli(), r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(klineJSON(start.UnixMilli())))
	}))
	defer server.Close()

	client := NewBinance(server.URL)
	got, err := client.KlinesSince(context.Background(), "ETHUSDT", "4h", start, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].OpenTime.Equal(start) {
		t.Fatalf("bad incremental candles: %+v", got)
	}
}

func TestBinanceKlinesSinceAllowsEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client := NewBinance(server.URL)
	got, err := client.KlinesSince(context.Background(), "ETHUSDT", "4h", time.Unix(1700000000, 0), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("len=%d want 0", len(got))
	}
}

func TestBinanceGetRetriesTransientFailure(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "temporary", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(klineJSON(1700000000000)))
	}))
	defer server.Close()

	client := NewBinance(server.URL)
	got, err := client.Klines(context.Background(), "BTCUSDT", "1d", 1)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || len(got) != 1 {
		t.Fatalf("attempts=%d len=%d", attempts, len(got))
	}
}

func klineJSON(openMS int64) string {
	closeMS := openMS + int64(24*time.Hour/time.Millisecond) - 1
	return fmt.Sprintf(`[[%d,"1.0","2.0","0.5","1.5","10",%d]]`, openMS, closeMS)
}
