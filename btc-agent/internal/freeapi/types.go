package freeapi

import "time"

type SourceStatus struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Enabled    bool   `json:"enabled"`
	Fresh      bool   `json:"fresh"`
	AgeMinutes int    `json:"age_minutes"`
	Error      string `json:"error,omitempty"`
}
type Report struct {
	GeneratedAt        time.Time      `json:"generated_at"`
	GlobalMarketCapUSD float64        `json:"global_market_cap_usd,omitempty"`
	GlobalVolumeUSD    float64        `json:"global_volume_usd,omitempty"`
	BTCDominancePct    float64        `json:"btc_dominance_pct,omitempty"`
	FearGreedValue     int            `json:"fear_greed_value,omitempty"`
	FearGreedLabel     string         `json:"fear_greed_label,omitempty"`
	EURUSD             float64        `json:"eurusd,omitempty"`
	DerivativesSymbol  string         `json:"derivatives_symbol,omitempty"`
	FundingRate        float64        `json:"funding_rate,omitempty"`
	OpenInterest       float64        `json:"open_interest,omitempty"`
	OpenInterestUSD    float64        `json:"open_interest_usd,omitempty"`
	DeFiTVLUSD         float64        `json:"defi_tvl_usd,omitempty"`
	News               []NewsItem     `json:"news,omitempty"`
	Sources            []SourceStatus `json:"sources"`
	Missing            []string       `json:"missing,omitempty"`
}
type NewsItem struct {
	Source      string    `json:"source"`
	Title       string    `json:"title"`
	URL         string    `json:"url,omitempty"`
	PublishedAt time.Time `json:"published_at,omitempty"`
	Category    string    `json:"category,omitempty"`
	Risk        string    `json:"risk,omitempty"`
}
