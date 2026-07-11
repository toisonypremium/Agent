package microstructure

import "fmt"

func BuildSignals(s Snapshot) Snapshot {
	flow := s.SpotFlow
	if flow.VolumeBase > 0 && flow.TakerBuyBase >= 0 {
		flow.TakerSellBase = maxFloat(0, flow.VolumeBase-flow.TakerBuyBase)
	}
	if flow.QuoteVolumeUSDT > 0 && flow.TakerBuyQuoteUSDT >= 0 {
		flow.TakerSellQuoteUSDT = maxFloat(0, flow.QuoteVolumeUSDT-flow.TakerBuyQuoteUSDT)
		flow.TakerBuyRatio = flow.TakerBuyQuoteUSDT / flow.QuoteVolumeUSDT
		flow.CVDQuoteUSDT = flow.TakerBuyQuoteUSDT - flow.TakerSellQuoteUSDT
	}
	s.SpotFlow = flow
	book := s.OrderBook
	if book.BestBid > 0 && book.BestAsk > 0 {
		mid := (book.BestBid + book.BestAsk) / 2
		if mid > 0 {
			book.SpreadBps = (book.BestAsk - book.BestBid) / mid * 10000
		}
	}
	if book.BidDepthUSDT+book.AskDepthUSDT > 0 {
		book.Imbalance = (book.BidDepthUSDT - book.AskDepthUSDT) / (book.BidDepthUSDT + book.AskDepthUSDT)
	}
	s.OrderBook = book
	s.Signals = signals(s)
	return s
}

func signals(s Snapshot) Signals {
	out := Signals{}
	switch {
	case s.SpotFlow.TakerBuyRatio >= 0.58:
		out.BuyPressure = "BUY_DOMINANT"
		out.Reasons = append(out.Reasons, fmt.Sprintf("taker buy ratio %.1f%%", s.SpotFlow.TakerBuyRatio*100))
	case s.SpotFlow.TakerBuyRatio <= 0.42 && s.SpotFlow.TakerBuyRatio > 0:
		out.BuyPressure = "SELL_DOMINANT"
		out.Risky = true
		out.Reasons = append(out.Reasons, fmt.Sprintf("taker buy ratio weak %.1f%%", s.SpotFlow.TakerBuyRatio*100))
	default:
		out.BuyPressure = "NEUTRAL"
	}
	switch {
	case s.SpotFlow.CVDQuoteUSDT > 0:
		out.CVDTrend = "POSITIVE"
	case s.SpotFlow.CVDQuoteUSDT < 0:
		out.CVDTrend = "NEGATIVE"
		out.Risky = true
	default:
		out.CVDTrend = "FLAT"
	}
	switch {
	case s.OrderBook.Imbalance >= 0.15:
		out.OrderBookBias = "BID_SUPPORT"
		out.Reasons = append(out.Reasons, fmt.Sprintf("bid imbalance %.2f", s.OrderBook.Imbalance))
	case s.OrderBook.Imbalance <= -0.15:
		out.OrderBookBias = "ASK_PRESSURE"
		out.Risky = true
		out.Reasons = append(out.Reasons, fmt.Sprintf("ask imbalance %.2f", s.OrderBook.Imbalance))
	default:
		out.OrderBookBias = "BALANCED"
	}
	switch {
	case s.Futures.FundingRate > 0.001:
		out.FundingBias = "CROWDED_LONG"
		out.Risky = true
	case s.Futures.FundingRate < -0.001:
		out.FundingBias = "SHORT_PRESSURE"
	default:
		out.FundingBias = "NEUTRAL"
	}
	switch {
	case s.Futures.BasisPct > 1:
		out.BasisBias = "PERP_PREMIUM"
		out.Risky = true
	case s.Futures.BasisPct < -1:
		out.BasisBias = "PERP_DISCOUNT"
	default:
		out.BasisBias = "NEUTRAL"
	}
	out.AbsorptionHint = out.CVDTrend == "POSITIVE" && out.OrderBookBias != "ASK_PRESSURE" && out.BuyPressure != "SELL_DOMINANT"
	out.Supportive = out.AbsorptionHint && !out.Risky
	return out
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
