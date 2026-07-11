package agent2

import (
	"sort"
	"strings"
)

type FilterAttribution struct {
	Symbol        string                  `json:"symbol"`
	State         State                   `json:"state"`
	SetupScore    float64                 `json:"setup_score,omitempty"`
	Passed        int                     `json:"passed"`
	FailedHard    int                     `json:"failed_hard"`
	FailedSoft    int                     `json:"failed_soft"`
	TopBlocker    string                  `json:"top_blocker,omitempty"`
	TopBlockerKey string                  `json:"top_blocker_key,omitempty"`
	GateRows      []FilterAttributionGate `json:"gate_rows,omitempty"`
}

type FilterAttributionGate struct {
	Name      string            `json:"name"`
	Pass      bool              `json:"pass"`
	Severity  SetupGateSeverity `json:"severity"`
	Score     float64           `json:"score"`
	ReasonKey string            `json:"reason_key,omitempty"`
	Reason    string            `json:"reason,omitempty"`
	Next      string            `json:"next,omitempty"`
}

func BuildFilterAttribution(asset AssetPlan) FilterAttribution {
	out := FilterAttribution{Symbol: asset.Symbol, State: asset.State, SetupScore: asset.SetupScore}
	for _, gate := range asset.SetupGates {
		row := FilterAttributionGate{Name: gate.Name, Pass: gate.Pass, Severity: gate.Severity, Score: gate.Score, ReasonKey: NormalizeReasonKey(firstNonEmptyAttr(gate.Name, gate.Reason)), Reason: gate.Reason, Next: gate.Next}
		out.GateRows = append(out.GateRows, row)
		if gate.Pass {
			out.Passed++
			continue
		}
		if gate.Severity == SetupGateHard {
			out.FailedHard++
		} else {
			out.FailedSoft++
		}
		if out.TopBlocker == "" || ReasonPriority(row.ReasonKey) < ReasonPriority(out.TopBlockerKey) {
			out.TopBlocker = firstNonEmptyAttr(gate.Reason, gate.Name)
			out.TopBlockerKey = row.ReasonKey
		}
	}
	if out.TopBlocker == "" {
		for _, reason := range asset.Reasons {
			if reason.Message == "" {
				continue
			}
			key := NormalizeReasonKey(string(reason.Code))
			if out.TopBlocker == "" || ReasonPriority(key) < ReasonPriority(out.TopBlockerKey) {
				out.TopBlocker = reason.Message
				out.TopBlockerKey = key
			}
		}
	}
	return out
}

func NormalizeReasonKey(reason string) string {
	s := strings.ToLower(strings.TrimSpace(reason))
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.Join(strings.Fields(s), " ")
	switch {
	case s == "", s == "none":
		return "UNKNOWN"
	case strings.Contains(s, "doctor") || strings.Contains(s, "reconcile") || strings.Contains(s, "risk governor") || strings.Contains(s, "data health") || strings.Contains(s, "operator halt"):
		return "SAFETY"
	case strings.Contains(s, "btc permission") || strings.Contains(s, "active limit") || strings.Contains(s, "btc watch") || strings.Contains(s, "btc armed"):
		return EntryCheckBTCPermission
	case strings.Contains(s, "panic") || strings.Contains(s, "falling knife"):
		return EntryCheckFallingKnife
	case strings.Contains(s, "fomo"):
		return EntryCheckFOMO
	case strings.Contains(s, "reward/risk") || strings.Contains(s, "reward risk") || strings.Contains(s, "rr "):
		return EntryCheckRewardRisk
	case strings.Contains(s, "discount") || strings.Contains(s, "support zone") || strings.Contains(s, "zone"):
		return EntryCheckDiscountZone
	case strings.Contains(s, "asset flow") || strings.Contains(s, "reclaim") || strings.Contains(s, "absorption"):
		return EntryCheckAssetFlowEntry
	case strings.Contains(s, "mm case") || strings.Contains(s, "mm accumulation") || strings.Contains(s, "mm execution"):
		return EntryCheckMMAccumulation
	case strings.Contains(s, "rotation rank"):
		return EntryCheckRotationRank
	case strings.Contains(s, "rotation"):
		return EntryCheckRotationScore
	case strings.Contains(s, "relative strength") || strings.Contains(s, "underperform"):
		return EntryCheckRelativeStrength
	case strings.Contains(s, "liquidity") || strings.Contains(s, "spread") || strings.Contains(s, "slippage") || strings.Contains(s, "bid depth") || strings.Contains(s, "order book"):
		return EntryCheckLiquidityQuality
	case strings.Contains(s, "data"):
		return EntryCheckData
	default:
		return strings.ToUpper(strings.ReplaceAll(s, " ", "_"))
	}
}

func CompactReasons(reasons []string, limit int) []string {
	type item struct {
		key      string
		reason   string
		priority int
		index    int
	}
	seen := map[string]item{}
	for i, reason := range reasons {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			continue
		}
		key := NormalizeReasonKey(reason)
		it := item{key: key, reason: compactReasonText(key, reason), priority: ReasonPriority(key), index: i}
		if prev, ok := seen[key]; !ok || it.priority < prev.priority || (it.priority == prev.priority && len(it.reason) < len(prev.reason)) {
			seen[key] = it
		}
	}
	items := make([]item, 0, len(seen))
	for _, it := range seen {
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].priority != items[j].priority {
			return items[i].priority < items[j].priority
		}
		return items[i].index < items[j].index
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.reason)
	}
	return out
}

func ReasonPriority(key string) int {
	switch key {
	case "SAFETY", EntryCheckData:
		return 0
	case EntryCheckBTCPermission:
		return 1
	case EntryCheckFallingKnife, EntryCheckFOMO:
		return 2
	case EntryCheckRewardRisk, EntryCheckDiscountZone:
		return 3
	case EntryCheckAssetFlowEntry, EntryCheckMMAccumulation:
		return 4
	case EntryCheckRotationScore, EntryCheckRotationRank, EntryCheckRelativeStrength, EntryCheckLiquidityQuality:
		return 5
	default:
		return 9
	}
}

func compactReasonText(key, reason string) string {
	switch key {
	case EntryCheckBTCPermission:
		return "BTC permission chưa cho phép ACTIVE_LIMIT"
	case EntryCheckRewardRisk:
		return reason
	case EntryCheckDiscountZone:
		return reason
	case EntryCheckMMAccumulation:
		if strings.Contains(strings.ToLower(reason), "mm execution") {
			return reason
		}
		return "MM/flow chưa có footprint đủ rõ"
	case EntryCheckAssetFlowEntry:
		return "asset flow chưa xác nhận reclaim/absorption"
	case EntryCheckRotationScore, EntryCheckRotationRank:
		return "rotation score/rank chưa đạt"
	case EntryCheckLiquidityQuality:
		return reason
	case EntryCheckFallingKnife:
		return reason
	case EntryCheckFOMO:
		return reason
	default:
		return reason
	}
}

func firstNonEmptyAttr(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
