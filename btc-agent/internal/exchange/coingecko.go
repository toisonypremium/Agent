package exchange

import "context"

type MarketOverview struct{ Note string }

func FetchCoinGeckoOverview(ctx context.Context) (MarketOverview, error) {
	return MarketOverview{Note: "optional overview skipped in phase 1"}, nil
}
