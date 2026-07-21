package freeapi

import (
	"btc-agent/internal/config"
	"btc-agent/internal/macroflow"
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
				TotalMarketCap        map[string]float64 `json:"total_market_cap"`
				TotalVolume           map[string]float64 `json:"total_volume"`
				MarketCapPercentage   map[string]float64 `json:"market_cap_percentage"`
				MarketCapChange24hUSD float64            `json:"market_cap_change_percentage_24h_usd"`
				VolumeChange24hUSD    float64            `json:"volume_change_percentage_24h_usd"`
			} `json:"data"`
		}
		e := c.get(ctx, "https://api.coingecko.com/api/v3/global", &x)
		r.GlobalMarketCapUSD = x.Data.TotalMarketCap["usd"]
		r.GlobalVolumeUSD = x.Data.TotalVolume["usd"]
		r.BTCDominancePct = x.Data.MarketCapPercentage["btc"]
		r.GlobalCapChange24h = x.Data.MarketCapChange24hUSD
		r.GlobalVolChange24h = x.Data.VolumeChange24hUSD
		add("coingecko", "https://api.coingecko.com/api/v3/global", true, e)
		if e == nil {
			var markets []struct {
				Symbol             string  `json:"symbol"`
				MarketCap          float64 `json:"market_cap"`
				MarketCapChange24h float64 `json:"market_cap_change_percentage_24h"`
				Change24h          float64 `json:"price_change_percentage_24h_in_currency"`
				Change7d           float64 `json:"price_change_percentage_7d_in_currency"`
			}
			marketsURL := "https://api.coingecko.com/api/v3/coins/markets?vs_currency=usd&order=market_cap_desc&per_page=100&page=1&sparkline=false&price_change_percentage=24h%2C7d"
			me := c.get(ctx, marketsURL, &markets)
			assets := make([]macroflow.AssetSnapshot, 0, len(markets))
			for _, m := range markets {
				s := strings.ToLower(strings.TrimSpace(m.Symbol))
				assets = append(assets, macroflow.AssetSnapshot{Symbol: s, MarketCapUSD: m.MarketCap, MarketCapChange24hPct: m.MarketCapChange24h, Change24hPct: m.Change24h, Change7dPct: m.Change7d, Stable: isStableSymbol(s)})
			}
			var supplyRows []struct {
				Date                string             `json:"date"`
				TotalCirculatingUSD map[string]float64 `json:"totalCirculatingUSD"`
			}
			const supplyURL = "https://stablecoins.llama.fi/stablecoincharts/all?stablecoin=1"
			se := c.get(ctx, supplyURL, &supplyRows)
			supply := make([]macroflow.SupplyPoint, 0, len(supplyRows))
			if se == nil {
				for _, row := range supplyRows {
					unix, parseErr := strconv.ParseInt(row.Date, 10, 64)
					usd := row.TotalCirculatingUSD["peggedUSD"]
					if parseErr == nil && unix > 0 && usd > 0 {
						supply = append(supply, macroflow.SupplyPoint{Unix: unix, USD: usd})
					}
				}
			}
			in, be := macroflow.BuildInput(macroflow.GlobalSnapshot{MarketCapUSD: r.GlobalMarketCapUSD, MarketCapChange24hPct: r.GlobalCapChange24h, VolumeChange24hPct: r.GlobalVolChange24h, UpdatedAtUnix: r.GeneratedAt.Unix()}, assets, supply, time.Now().Unix(), int64(max)*60)
			if be == nil {
				r.MacroFlow = macroflow.Evaluate(in)
				r.USDTDominancePct = in.USDTDominancePct
			}
			add("coingecko_markets", marketsURL, true, me)
			add("defillama_usdt_supply", supplyURL, true, se)
		}
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
func isStableSymbol(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "usdt", "usdc", "dai", "usde", "usds", "usd1", "usdg", "pyusd", "usdy", "usyc", "busd", "tusd":
		return true
	default:
		return false
	}
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
