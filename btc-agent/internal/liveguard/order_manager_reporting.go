package liveguard

import (
	"sort"
	"strings"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
)

func BuildManagedCoinSummaries(cfg config.Config, plan agent2.Plan, openOrders []live.OrderStatus, result ManagedCycleResult) []ManagedCoinSummary {
	symbols := []string{}
	seen := map[string]bool{}
	addSymbol := func(symbol string) {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" || seen[symbol] {
			return
		}
		seen[symbol] = true
		symbols = append(symbols, symbol)
	}
	for _, symbol := range cfg.Data.Symbols.Assets {
		addSymbol(symbol)
	}
	stateBySymbol := map[string]agent2.State{}
	reasonBySymbol := map[string]string{}
	hardBySymbol := map[string][]string{}
	softBySymbol := map[string][]string{}
	nextBySymbol := map[string]string{}
	attributionBySymbol := map[string]agent2.FilterAttribution{}
	for _, asset := range plan.Assets {
		symbol := strings.ToUpper(asset.Symbol)
		addSymbol(symbol)
		stateBySymbol[symbol] = asset.State
		reasonBySymbol[symbol] = asset.Reason
		hardBySymbol[symbol] = appendUniqueStrings(hardBySymbol[symbol], asset.HardBlockers...)
		softBySymbol[symbol] = appendUniqueStrings(softBySymbol[symbol], asset.SoftBlockers...)
		nextBySymbol[symbol] = asset.NextTrigger
		attributionBySymbol[symbol] = agent2.BuildFilterAttribution(asset)
	}
	watchBySymbol := map[string]agent2.WatchCandidate{}
	for _, candidate := range plan.Watchlist.Candidates {
		symbol := strings.ToUpper(candidate.Symbol)
		addSymbol(symbol)
		watchBySymbol[symbol] = candidate
	}
	for _, order := range openOrders {
		addSymbol(orderSymbol(order))
	}
	for _, d := range result.Desired {
		addSymbol(d.Symbol)
	}
	for _, decision := range allManagedDecisions(result) {
		addSymbol(decisionSymbol(decision))
	}

	summaries := map[string]*ManagedCoinSummary{}
	for _, symbol := range symbols {
		state, ok := stateBySymbol[symbol]
		if !ok {
			state = defaultCoinState(plan.State)
		}
		summaries[symbol] = &ManagedCoinSummary{Symbol: symbol, State: state}
	}
	ensure := func(symbol string) *ManagedCoinSummary {
		symbol = strings.ToUpper(strings.TrimSpace(symbol))
		if symbol == "" {
			symbol = "UNKNOWN"
		}
		if summaries[symbol] == nil {
			addSymbol(symbol)
			summaries[symbol] = &ManagedCoinSummary{Symbol: symbol, State: defaultCoinState(plan.State)}
		}
		return summaries[symbol]
	}
	for _, d := range result.Desired {
		ensure(d.Symbol).DesiredLayers++
	}
	for _, order := range openOrders {
		s := ensure(orderSymbol(order))
		s.OpenOrders++
		notional := order.Notional
		if notional <= 0 && order.Price > 0 && order.Quantity > 0 {
			notional = order.Price * order.Quantity
		}
		s.PendingNotional += notional
	}
	addDecision := func(decision ManagedOrderDecision, counter func(*ManagedCoinSummary)) {
		s := ensure(decisionSymbol(decision))
		counter(s)
		s.Actions = append(s.Actions, decision)
		if decision.Reason != "" && !stringInSlice(s.Reasons, decision.Reason) {
			s.Reasons = append(s.Reasons, decision.Reason)
			if decision.Action == "block" || decision.Error != "" {
				s.HardBlockers = appendUniqueStrings(s.HardBlockers, decision.Reason)
			} else {
				s.SoftBlockers = appendUniqueStrings(s.SoftBlockers, decision.Reason)
			}
		}
	}
	for _, d := range result.Kept {
		addDecision(d, func(s *ManagedCoinSummary) { s.Kept++ })
	}
	for _, d := range result.Canceled {
		addDecision(d, func(s *ManagedCoinSummary) {
			s.Canceled++
			s.PendingNotional -= decisionOrderNotional(d)
		})
	}
	for _, d := range result.Replaced {
		addDecision(d, func(s *ManagedCoinSummary) {
			s.Replaced++
			s.PendingNotional -= decisionOrderNotional(d)
		})
	}
	for _, d := range result.Placed {
		addDecision(d, func(s *ManagedCoinSummary) {
			s.Placed++
			s.PendingNotional += d.Desired.Notional
		})
	}
	for _, d := range result.Blocked {
		addDecision(d, func(s *ManagedCoinSummary) { s.Blocked++ })
	}
	if len(result.Reasons) > 0 {
		for _, symbol := range symbols {
			s := summaries[symbol]
			if s == nil {
				continue
			}
			for _, reason := range result.Reasons {
				if reason != "" && !stringInSlice(s.Reasons, reason) {
					s.Reasons = append(s.Reasons, reason)
				}
			}
		}
	}
	for _, symbol := range symbols {
		s := summaries[symbol]
		if s == nil {
			continue
		}
		s.HardBlockers = appendUniqueStrings(s.HardBlockers, hardBySymbol[symbol]...)
		s.SoftBlockers = appendUniqueStrings(s.SoftBlockers, softBySymbol[symbol]...)
		if nextBySymbol[symbol] != "" {
			s.NextTrigger = nextBySymbol[symbol]
		}
		if candidate, ok := watchBySymbol[symbol]; ok {
			s.ReadinessScore = candidate.ReadinessScore
			if s.NextTrigger == "" {
				s.NextTrigger = candidate.NextTrigger
			}
			if s.DesiredLayers == 0 && s.Placed == 0 && s.Kept == 0 {
				s.SoftBlockers = appendUniqueStrings(s.SoftBlockers, candidate.Missing...)
			}
		}
		if s.DesiredLayers == 0 && s.Placed == 0 && s.Kept == 0 {
			s.WhyNoOrder = appendUniqueStrings(s.WhyNoOrder, s.HardBlockers...)
			s.WhyNoOrder = appendUniqueStrings(s.WhyNoOrder, s.SoftBlockers...)
			s.WhyNoOrder = appendUniqueStrings(s.WhyNoOrder, s.Reasons...)
			if reason := reasonBySymbol[symbol]; reason != "" {
				s.WhyNoOrder = appendUniqueStrings(s.WhyNoOrder, reason)
			}
			if len(s.WhyNoOrder) == 0 {
				s.WhyNoOrder = append(s.WhyNoOrder, "chưa có ACTIVE_LIMIT layer hợp lệ cho coin này")
			}
		}
		if attr, ok := attributionBySymbol[symbol]; ok {
			s.FilterAttribution = attr
			s.TopFilterBlocker = attr.TopBlocker
			s.TopFilterBlockerKey = attr.TopBlockerKey
		}
		if s.DesiredLayers > 0 && s.Placed == 0 && s.Kept == 0 && s.Blocked > 0 {
			s.WhyNoOrder = appendUniqueStrings(s.WhyNoOrder, s.HardBlockers...)
			s.WhyNoOrder = appendUniqueStrings(s.WhyNoOrder, s.Reasons...)
		}
		s.WhyNoOrder = agent2.CompactReasons(s.WhyNoOrder, 6)
		s.Reasons = agent2.CompactReasons(s.Reasons, 8)
	}
	order := map[string]int{}
	for i, symbol := range cfg.Data.Symbols.Assets {
		order[strings.ToUpper(symbol)] = i
	}
	sort.Slice(symbols, func(i, j int) bool {
		li, lok := order[symbols[i]]
		rj, rok := order[symbols[j]]
		if lok && rok {
			return li < rj
		}
		if lok != rok {
			return lok
		}
		return symbols[i] < symbols[j]
	})
	out := []ManagedCoinSummary{}
	for _, symbol := range symbols {
		if summaries[symbol] != nil {
			out = append(out, *summaries[symbol])
		}
	}
	return out
}

func allManagedDecisions(result ManagedCycleResult) []ManagedOrderDecision {
	out := []ManagedOrderDecision{}
	out = append(out, result.Kept...)
	out = append(out, result.Canceled...)
	out = append(out, result.Replaced...)
	out = append(out, result.Placed...)
	out = append(out, result.Blocked...)
	return out
}

func decisionSymbol(decision ManagedOrderDecision) string {
	if decision.Symbol != "" {
		return decision.Symbol
	}
	if decision.Desired.Symbol != "" {
		return decision.Desired.Symbol
	}
	return orderSymbol(decision.Order)
}

func decisionOrderNotional(decision ManagedOrderDecision) float64 {
	notional := decision.Order.Notional
	if notional <= 0 && decision.Order.Price > 0 && decision.Order.Quantity > 0 {
		notional = decision.Order.Price * decision.Order.Quantity
	}
	return notional
}

func defaultCoinState(planState agent2.State) agent2.State {
	if planState == agent2.StateWatch || planState == agent2.StateArmed || planState == agent2.StateNoTrade {
		return planState
	}
	return agent2.StateNoTrade
}

func stringInSlice(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func appendUniqueStrings(items []string, values ...string) []string {
	for _, value := range values {
		if value == "" || stringInSlice(items, value) {
			continue
		}
		items = append(items, value)
	}
	return items
}
