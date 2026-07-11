package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"btc-agent/internal/microstructure"
)

type BinanceFuturesClient struct {
	base string
	http *http.Client
}

func NewBinanceFutures(base string) *BinanceFuturesClient {
	return &BinanceFuturesClient{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 20 * time.Second}}
}

func (c *BinanceFuturesClient) FuturesObservation(ctx context.Context, symbol string) (microstructure.FuturesObservation, error) {
	out := microstructure.FuturesObservation{}
	q := url.Values{"symbol": {strings.ToUpper(symbol)}}
	var oi struct {
		OpenInterest string `json:"openInterest"`
	}
	if err := c.get(ctx, "/fapi/v1/openInterest?"+q.Encode(), &oi); err != nil {
		return out, err
	}
	out.OpenInterest = parseFloatDefault(oi.OpenInterest)
	var premium struct {
		MarkPrice       string `json:"markPrice"`
		IndexPrice      string `json:"indexPrice"`
		LastFundingRate string `json:"lastFundingRate"`
	}
	if err := c.get(ctx, "/fapi/v1/premiumIndex?"+q.Encode(), &premium); err != nil {
		return out, err
	}
	mark := parseFloatDefault(premium.MarkPrice)
	index := parseFloatDefault(premium.IndexPrice)
	out.FundingRate = parseFloatDefault(premium.LastFundingRate)
	if mark > 0 && index > 0 {
		out.BasisPct = (mark - index) / index * 100
	}
	return out, nil
}

func (c *BinanceFuturesClient) get(ctx context.Context, path string, out any) error {
	if c.base == "" {
		return fmt.Errorf("binance futures base URL required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("binance futures http %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
