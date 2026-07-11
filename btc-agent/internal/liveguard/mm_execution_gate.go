package liveguard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"btc-agent/internal/config"
	"btc-agent/internal/liquidity"
)

const minMMExecutionBidShare = 0.40

type OrderBookProvider interface {
	OrderBook(ctx context.Context, instID string) (liquidity.OrderBookSnapshot, error)
}

type MMExecutionGateResult struct {
	Enabled              bool     `json:"enabled"`
	Pass                 bool     `json:"pass"`
	Symbol               string   `json:"symbol"`
	InstID               string   `json:"inst_id"`
	SpreadBps            float64  `json:"spread_bps,omitempty"`
	EstimatedSlippageBps float64  `json:"estimated_slippage_bps,omitempty"`
	BidDepth1PctUSDT     float64  `json:"bid_depth_1pct_usdt,omitempty"`
	AskDepth1PctUSDT     float64  `json:"ask_depth_1pct_usdt,omitempty"`
	BidDepthToOrderRatio float64  `json:"bid_depth_to_order_ratio,omitempty"`
	BidShare             float64  `json:"bid_share,omitempty"`
	BestBid              float64  `json:"best_bid,omitempty"`
	BestAsk              float64  `json:"best_ask,omitempty"`
	Reasons              []string `json:"reasons,omitempty"`
}

func EvaluateMMExecutionGate(ctx context.Context, cfg config.Config, desired ManagedDesiredOrder, provider OrderBookProvider) MMExecutionGateResult {
	result := MMExecutionGateResult{Enabled: cfg.Live.LiquidityGateEnabled, Pass: true, Symbol: desired.Symbol, InstID: desired.InstID}
	if !result.Enabled {
		return result
	}
	if provider == nil {
		if cfg.Live.RequireOrderBookLiquidity {
			result.Pass = false
			result.Reasons = append(result.Reasons, "MM execution gate: order book unavailable")
		} else {
			result.Reasons = append(result.Reasons, "MM execution gate: order book optional unavailable")
		}
		return result
	}
	samples := normalizedMMGateSamples(cfg)
	books := make([]liquidity.OrderBookSnapshot, 0, samples)
	for i := 0; i < samples; i++ {
		book, err := provider.OrderBook(ctx, desired.InstID)
		if err != nil {
			if cfg.Live.RequireOrderBookLiquidity {
				result.Pass = false
				result.Reasons = append(result.Reasons, "MM execution gate: order book error")
			} else {
				result.Reasons = append(result.Reasons, "MM execution gate: order book optional error")
			}
			return result
		}
		books = append(books, book)
		if i+1 < samples && cfg.Live.MMGateSampleDelayMs > 0 {
			select {
			case <-ctx.Done():
				result.Pass = false
				result.Reasons = append(result.Reasons, "MM execution gate: context canceled")
				return result
			case <-time.After(time.Duration(cfg.Live.MMGateSampleDelayMs) * time.Millisecond):
			}
		}
	}
	return EvaluateMMExecutionGateSamples(cfg, desired, books)
}

func EvaluateMMExecutionGateWithBook(cfg config.Config, desired ManagedDesiredOrder, book liquidity.OrderBookSnapshot) MMExecutionGateResult {
	result := MMExecutionGateResult{Enabled: cfg.Live.LiquidityGateEnabled, Pass: true, Symbol: desired.Symbol, InstID: desired.InstID, BestBid: book.BestBid, BestAsk: book.BestAsk, BidDepth1PctUSDT: book.BidDepth1PctUSDT, AskDepth1PctUSDT: book.AskDepth1PctUSDT}
	if !result.Enabled {
		return result
	}
	maxSpread := cfg.Live.MaxSpreadBps
	if maxSpread <= 0 {
		maxSpread = 15
	}
	maxSlip := cfg.Live.MaxSlippageBps
	if maxSlip <= 0 {
		maxSlip = 30
	}
	depthRatio := cfg.Live.MinBidDepthToOrderRatio
	if depthRatio <= 0 {
		depthRatio = 20
	}
	if desired.Side != "BUY" {
		result.addBlock("MM execution gate: side must be BUY")
	}
	if strings.ToLower(desired.Type) != "limit" {
		result.addBlock("MM execution gate: type must be limit")
	}
	if cfg.Live.RequirePostOnly && !desired.PostOnly {
		result.addBlock("MM execution gate: post-only required")
	}
	if desired.Notional <= 0 {
		result.addBlock("MM execution gate: order notional invalid")
	}
	if book.BestBid <= 0 || book.BestAsk <= 0 || book.BestAsk < book.BestBid {
		result.addBlock("MM execution gate: invalid order book")
		return result
	}
	mid := (book.BestBid + book.BestAsk) / 2
	result.SpreadBps = (book.BestAsk - book.BestBid) / mid * 10000
	if result.SpreadBps > maxSpread {
		result.addBlock(fmt.Sprintf("MM execution gate: spread %.2fbps > %.2fbps", result.SpreadBps, maxSpread))
	}
	if desired.PostOnly && strings.EqualFold(desired.Side, "BUY") && desired.Price >= book.BestAsk {
		result.addBlock("MM execution gate: post-only BUY price would cross best ask")
	}
	if desired.Notional > 0 && book.BidDepth1PctUSDT > 0 {
		result.BidDepthToOrderRatio = book.BidDepth1PctUSDT / desired.Notional
		result.EstimatedSlippageBps = desired.Notional / book.BidDepth1PctUSDT * 10000
	}
	if desired.Notional > 0 && book.BidDepth1PctUSDT < desired.Notional*depthRatio {
		result.addBlock(fmt.Sprintf("MM execution gate: bid depth %.2f < %.1fx order", book.BidDepth1PctUSDT, depthRatio))
	}
	if result.EstimatedSlippageBps > maxSlip {
		result.addBlock(fmt.Sprintf("MM execution gate: slippage %.2fbps > %.2fbps", result.EstimatedSlippageBps, maxSlip))
	}
	if book.BidDepth1PctUSDT+book.AskDepth1PctUSDT > 0 {
		result.BidShare = book.BidDepth1PctUSDT / (book.BidDepth1PctUSDT + book.AskDepth1PctUSDT)
		if result.BidShare < minMMExecutionBidShare {
			result.addBlock(fmt.Sprintf("MM execution gate: bid share %.1f%% < 40.0%% sell-pressure proxy", result.BidShare*100))
		}
	}
	if len(result.Reasons) == 0 {
		result.Reasons = append(result.Reasons, fmt.Sprintf("MM execution gate OK: spread %.2fbps depth %.1fx bid_share %.1f%%", result.SpreadBps, result.BidDepthToOrderRatio, result.BidShare*100))
	}
	return result
}

func EvaluateMMExecutionGateSamples(cfg config.Config, desired ManagedDesiredOrder, books []liquidity.OrderBookSnapshot) MMExecutionGateResult {
	result := MMExecutionGateResult{Enabled: cfg.Live.LiquidityGateEnabled, Pass: true, Symbol: desired.Symbol, InstID: desired.InstID}
	if !result.Enabled {
		return result
	}
	if len(books) == 0 {
		result.Pass = false
		result.Reasons = append(result.Reasons, "MM execution gate: order book unavailable")
		return result
	}
	for i, book := range books {
		sample := EvaluateMMExecutionGateWithBook(cfg, desired, book)
		if i == 0 {
			result = sample
		}
		if !sample.Pass {
			result.Pass = false
			for _, reason := range sample.Reasons {
				result.Reasons = append(result.Reasons, fmt.Sprintf("sample %d: %s", i+1, reason))
			}
		}
		if sample.SpreadBps > result.SpreadBps {
			result.SpreadBps = sample.SpreadBps
		}
		if result.EstimatedSlippageBps == 0 || (sample.EstimatedSlippageBps > 0 && sample.EstimatedSlippageBps > result.EstimatedSlippageBps) {
			result.EstimatedSlippageBps = sample.EstimatedSlippageBps
		}
		if result.BidDepth1PctUSDT == 0 || (sample.BidDepth1PctUSDT > 0 && sample.BidDepth1PctUSDT < result.BidDepth1PctUSDT) {
			result.BidDepth1PctUSDT = sample.BidDepth1PctUSDT
		}
		if result.BidShare == 0 || (sample.BidShare > 0 && sample.BidShare < result.BidShare) {
			result.BidShare = sample.BidShare
		}
	}
	if len(books) > 1 && result.Pass {
		result.Reasons = []string{fmt.Sprintf("MM execution gate OK: %d stable samples spread %.2fbps depth %.1fx bid_share %.1f%%", len(books), result.SpreadBps, result.BidDepthToOrderRatio, result.BidShare*100)}
	}
	return result
}

func normalizedMMGateSamples(cfg config.Config) int {
	if cfg.Live.MMGateSamples <= 0 {
		return 1
	}
	if cfg.Live.MMGateSamples > 3 {
		return 3
	}
	return cfg.Live.MMGateSamples
}

func (r *MMExecutionGateResult) addBlock(reason string) {
	r.Pass = false
	r.Reasons = append(r.Reasons, reason)
}

func orderBookProviderFromPlacer(placer OrderPlacer) OrderBookProvider {
	provider, _ := placer.(OrderBookProvider)
	return provider
}
