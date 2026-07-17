package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"btc-agent/internal/market"
)

type BinanceClient struct {
	base string
	http *http.Client
}

func NewBinance(base string) *BinanceClient {
	return &BinanceClient{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *BinanceClient) Klines(ctx context.Context, symbol, interval string, limit int) ([]market.Candle, error) {
	return c.KlinesRange(ctx, symbol, interval, limit, time.Time{}, time.Time{})
}

// KlinesRange fetches public candles in a bounded time window. It has no
// account credentials and no order authority.
func (c *BinanceClient) KlinesRange(ctx context.Context, symbol, interval string, limit int, start, end time.Time) ([]market.Candle, error) {
	q := url.Values{"symbol": {strings.ToUpper(symbol)}, "interval": {interval}, "limit": {strconv.Itoa(limit)}}
	if !start.IsZero() {
		q.Set("startTime", strconv.FormatInt(start.UnixMilli(), 10))
	}
	if !end.IsZero() {
		q.Set("endTime", strconv.FormatInt(end.UnixMilli(), 10))
	}
	var raw [][]json.RawMessage
	if err := c.get(ctx, "/api/v3/klines?"+q.Encode(), &raw); err != nil {
		return nil, err
	}
	out := make([]market.Candle, 0, len(raw))
	for _, row := range raw {
		if len(row) < 7 {
			continue
		}
		cdl, ok := parseKline(symbol, interval, row)
		if ok {
			out = append(out, cdl)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no candles for %s %s", symbol, interval)
	}
	return out, nil
}

func (c *BinanceClient) get(ctx context.Context, path string, out any) error {
	var last error
	for i := 0; i < 3; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
		if err != nil {
			return err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			last = err
			time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
			continue
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				last = fmt.Errorf("binance http %d", resp.StatusCode)
				return
			}
			last = json.NewDecoder(resp.Body).Decode(out)
		}()
		if last == nil {
			return nil
		}
		time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
	}
	return last
}

func parseKline(symbol, interval string, row []json.RawMessage) (market.Candle, bool) {
	var openMS, closeMS int64
	if json.Unmarshal(row[0], &openMS) != nil || json.Unmarshal(row[6], &closeMS) != nil {
		return market.Candle{}, false
	}
	idx := []int{1, 2, 3, 4, 5}
	vals := make([]float64, 5)
	for i, at := range idx {
		var s string
		if json.Unmarshal(row[at], &s) != nil {
			return market.Candle{}, false
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return market.Candle{}, false
		}
		vals[i] = v
	}
	return market.Candle{Symbol: strings.ToUpper(symbol), Interval: interval, OpenTime: time.UnixMilli(openMS), CloseTime: time.UnixMilli(closeMS), Open: vals[0], High: vals[1], Low: vals[2], Close: vals[3], Volume: vals[4]}, true
}
