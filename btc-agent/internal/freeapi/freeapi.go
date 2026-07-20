package freeapi

import (
	"btc-agent/internal/config"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type rss struct {
	Channel struct {
		Title string `xml:"title"`
		Items []struct {
			Title string `xml:"title"`
			Link  string `xml:"link"`
		} `xml:"item"`
	} `xml:"channel"`
}
type Client struct {
	cfg  config.Config
	http *http.Client
}

func New(c config.Config) *Client {
	t := time.Duration(c.FreeAPI.TimeoutSeconds) * time.Second
	if t <= 0 {
		t = 10 * time.Second
	}
	return &Client{c, &http.Client{Timeout: t}}
}
func (c *Client) get(ctx context.Context, url string, v any) error {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if e != nil {
		return e
	}
	r, e := c.http.Do(req)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode/100 != 2 {
		return fmt.Errorf("http %d", r.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func (c *Client) Run(ctx context.Context) Report {
	r := Report{GeneratedAt: time.Now().UTC()}
	max := c.cfg.FreeAPI.MaxStaleMinutes
	if max <= 0 {
		max = 360
	}
	add := func(n, u string, en bool, e error) {
		r.Sources = append(r.Sources, SourceStatus{n, u, en, e == nil, 0, errString(e)})
		if e != nil {
			r.Missing = append(r.Missing, n)
		}
	}
	if !c.cfg.FreeAPI.Enabled {
		return r
	}
	if c.cfg.FreeAPI.CoinGecko.Enabled {
		var x struct {
			Data struct {
				TotalMarketCap      map[string]float64 `json:"total_market_cap"`
				TotalVolume         map[string]float64 `json:"total_volume"`
				MarketCapPercentage map[string]float64 `json:"market_cap_percentage"`
			} `json:"data"`
		}
		e := c.get(ctx, "https://api.coingecko.com/api/v3/global", &x)
		r.GlobalMarketCapUSD = x.Data.TotalMarketCap["usd"]
		r.GlobalVolumeUSD = x.Data.TotalVolume["usd"]
		r.BTCDominancePct = x.Data.MarketCapPercentage["btc"]
		add("coingecko", "https://api.coingecko.com/api/v3/global", true, e)
	}
	if c.cfg.FreeAPI.FearGreed.Enabled {
		var x struct {
			Data []struct {
				Value               string `json:"value"`
				ValueClassification string `json:"value_classification"`
			} `json:"data"`
		}
		e := c.get(ctx, "https://api.alternative.me/fng/?limit=1", &x)
		if len(x.Data) > 0 {
			fmt.Sscanf(x.Data[0].Value, "%d", &r.FearGreedValue)
			r.FearGreedLabel = x.Data[0].ValueClassification
		}
		add("fear_greed", "https://api.alternative.me/fng/?limit=1", true, e)
	}
	if c.cfg.FreeAPI.FX.Enabled {
		var x struct {
			Rates map[string]float64 `json:"rates"`
		}
		e := c.get(ctx, "https://api.frankfurter.app/latest?from=EUR&to=USD", &x)
		r.EURUSD = x.Rates["USD"]
		add("frankfurter", "https://api.frankfurter.app/latest?from=EUR&to=USD", true, e)
	}
	if c.cfg.FreeAPI.Derivatives.Enabled {
		baseURL := strings.TrimRight(strings.TrimSpace(c.cfg.FreeAPI.Derivatives.BaseURL), "/")
		if baseURL == "" {
			baseURL = "https://fapi.binance.com"
		}
		symbol := strings.ToUpper(strings.TrimSpace(c.cfg.FreeAPI.Derivatives.Symbol))
		if symbol == "" {
			symbol = "BTCUSDT"
		}
		r.DerivativesSymbol = symbol
		premiumURL := baseURL + "/fapi/v1/premiumIndex?symbol=" + url.QueryEscape(symbol)
		var premium struct {
			MarkPrice       string `json:"markPrice"`
			LastFundingRate string `json:"lastFundingRate"`
		}
		premiumErr := c.get(ctx, premiumURL, &premium)
		markPrice, markErr := strconv.ParseFloat(premium.MarkPrice, 64)
		fundingRate, fundingErr := strconv.ParseFloat(premium.LastFundingRate, 64)
		if premiumErr == nil && (markErr != nil || fundingErr != nil) {
			premiumErr = fmt.Errorf("invalid premium index response")
		}
		if premiumErr == nil {
			r.FundingRate = fundingRate
		}
		add("binance_funding", premiumURL, true, premiumErr)

		openInterestURL := baseURL + "/fapi/v1/openInterest?symbol=" + url.QueryEscape(symbol)
		var oi struct {
			OpenInterest string `json:"openInterest"`
		}
		oiErr := c.get(ctx, openInterestURL, &oi)
		openInterest, parseErr := strconv.ParseFloat(oi.OpenInterest, 64)
		if oiErr == nil && parseErr != nil {
			oiErr = fmt.Errorf("invalid open interest response")
		}
		if oiErr == nil {
			r.OpenInterest = openInterest
			r.OpenInterestUSD = openInterest * markPrice
		}
		add("binance_open_interest", openInterestURL, true, oiErr)
	}
	if c.cfg.FreeAPI.OnChain.Enabled {
		const chainsURL = "https://api.llama.fi/v2/chains"
		var chains []struct {
			TVL float64 `json:"tvl"`
		}
		e := c.get(ctx, chainsURL, &chains)
		if e == nil {
			for _, chain := range chains {
				if chain.TVL > 0 {
					r.DeFiTVLUSD += chain.TVL
				}
			}
		}
		add("defillama_chains", chainsURL, true, e)
	}
	if c.cfg.FreeAPI.News.Enabled {
		limit := c.cfg.FreeAPI.News.MaxItems
		if limit <= 0 {
			limit = 20
		}
		for _, u := range c.cfg.Research.RSS.Feeds {
			req, e := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if e != nil {
				continue
			}
			resp, e := c.http.Do(req)
			if e != nil {
				continue
			}
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			resp.Body.Close()
			var f rss
			if xml.Unmarshal(b, &f) == nil {
				for _, i := range f.Channel.Items {
					if len(r.News) >= limit {
						break
					}
					r.News = append(r.News, NewsItem{Source: f.Channel.Title, Title: strings.TrimSpace(i.Title), URL: strings.TrimSpace(i.Link), Category: "news"})
				}
			}
		}
		add("rss_news", "configured RSS feeds", true, nil)
	}
	return r
}
func errString(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}
func Save(r Report, dir string) error {
	if dir == "" {
		dir = "reports"
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(dir, "freeapi_latest.json"), b, 0600)
}
func Load(dir string) (Report, error) {
	b, e := os.ReadFile(filepath.Join(dir, "freeapi_latest.json"))
	var r Report
	if e == nil {
		e = json.Unmarshal(b, &r)
	}
	return r, e
}
