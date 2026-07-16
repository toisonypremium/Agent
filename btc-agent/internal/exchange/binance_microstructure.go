package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"btc-agent/internal/microstructure"
)

func (c *BinanceClient) KlineFlow(ctx context.Context, symbol, interval string, limit int) (microstructure.SpotFlow, time.Time, error) {
	if limit <= 0 {
		limit = 3
	}
	q := url.Values{"symbol": {strings.ToUpper(symbol)}, "interval": {interval}, "limit": {strconv.Itoa(limit)}}
	var raw [][]json.RawMessage
	if err := c.get(ctx, "/api/v3/klines?"+q.Encode(), &raw); err != nil {
		return microstructure.SpotFlow{}, time.Time{}, err
	}
	// Use the newest closed candle only. The newest API row may still be open;
	// selecting the previous row makes each persisted observation a non-overlapping bucket.
	if len(raw) < 2 {
		return microstructure.SpotFlow{}, time.Time{}, fmt.Errorf("insufficient closed kline flow for %s %s", symbol, interval)
	}
	row := raw[len(raw)-2]
	if len(row) < 11 {
		return microstructure.SpotFlow{}, time.Time{}, fmt.Errorf("invalid kline flow for %s %s", symbol, interval)
	}
	var closeMS int64
	_ = json.Unmarshal(row[6], &closeMS)
	flow := microstructure.SpotFlow{VolumeBase: rawStringFloat(row[5]), QuoteVolumeUSDT: rawStringFloat(row[7]), TakerBuyBase: rawStringFloat(row[9]), TakerBuyQuoteUSDT: rawStringFloat(row[10])}
	if flow.QuoteVolumeUSDT <= 0 {
		return flow, time.UnixMilli(closeMS), fmt.Errorf("no kline flow for %s %s", symbol, interval)
	}
	flow.TakerSellBase = maxExchangeFloat(0, flow.VolumeBase-flow.TakerBuyBase)
	flow.TakerSellQuoteUSDT = maxExchangeFloat(0, flow.QuoteVolumeUSDT-flow.TakerBuyQuoteUSDT)
	flow.TakerBuyRatio = flow.TakerBuyQuoteUSDT / flow.QuoteVolumeUSDT
	flow.CVDQuoteUSDT = flow.TakerBuyQuoteUSDT - flow.TakerSellQuoteUSDT
	return flow, time.UnixMilli(closeMS), nil
}
func (c *BinanceClient) Depth(ctx context.Context, symbol string, limit int) (microstructure.OrderBook, error) {
	if limit <= 0 {
		limit = 100
	}
	q := url.Values{"symbol": {strings.ToUpper(symbol)}, "limit": {strconv.Itoa(limit)}}
	var raw struct {
		Bids [][]string `json:"bids"`
		Asks [][]string `json:"asks"`
	}
	if err := c.get(ctx, "/api/v3/depth?"+q.Encode(), &raw); err != nil {
		return microstructure.OrderBook{}, err
	}
	if len(raw.Bids) == 0 || len(raw.Asks) == 0 {
		return microstructure.OrderBook{}, fmt.Errorf("no orderbook for %s", symbol)
	}
	book := microstructure.OrderBook{BestBid: parseFloatDefault(raw.Bids[0][0]), BestAsk: parseFloatDefault(raw.Asks[0][0])}
	for _, bid := range raw.Bids {
		if len(bid) < 2 {
			continue
		}
		price := parseFloatDefault(bid[0])
		size := parseFloatDefault(bid[1])
		book.BidDepthUSDT += price * size
	}
	for _, ask := range raw.Asks {
		if len(ask) < 2 {
			continue
		}
		price := parseFloatDefault(ask[0])
		size := parseFloatDefault(ask[1])
		book.AskDepthUSDT += price * size
	}
	if book.BestBid > 0 && book.BestAsk > 0 {
		mid := (book.BestBid + book.BestAsk) / 2
		book.SpreadBps = (book.BestAsk - book.BestBid) / mid * 10000
	}
	if book.BidDepthUSDT+book.AskDepthUSDT > 0 {
		book.Imbalance = (book.BidDepthUSDT - book.AskDepthUSDT) / (book.BidDepthUSDT + book.AskDepthUSDT)
	}
	return book, nil
}

func rawStringFloat(raw json.RawMessage) float64 {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return parseFloatDefault(s)
	}
	var f float64
	_ = json.Unmarshal(raw, &f)
	return f
}

func parseFloatDefault(value string) float64 {
	f, _ := strconv.ParseFloat(value, 64)
	return f
}

func maxExchangeFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
