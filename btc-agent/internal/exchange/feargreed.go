package exchange

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

type FearGreed struct {
	Value          int
	Classification string
}

func FetchFearGreed(ctx context.Context) (FearGreed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.alternative.me/fng/?limit=1", nil)
	if err != nil {
		return FearGreed{}, err
	}
	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return FearGreed{}, err
	}
	defer resp.Body.Close()
	var raw struct {
		Data []struct {
			Value          string `json:"value"`
			Classification string `json:"value_classification"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return FearGreed{}, err
	}
	if len(raw.Data) == 0 {
		return FearGreed{}, nil
	}
	v, _ := strconv.Atoi(raw.Data[0].Value)
	return FearGreed{Value: v, Classification: raw.Data[0].Classification}, nil
}
