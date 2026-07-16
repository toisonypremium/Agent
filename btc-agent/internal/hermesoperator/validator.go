package hermesoperator

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func Validate(d Decision, p ValidationPolicy) ValidationResult {
	if p.Now.IsZero() {
		p.Now = time.Now().UTC()
	}
	result := ValidationResult{Decision: d}
	if d.Version != 1 {
		result.Reasons = append(result.Reasons, "unsupported decision version")
	}
	if strings.TrimSpace(d.DecisionID) == "" {
		result.Reasons = append(result.Reasons, "decision_id required")
	}
	if d.GeneratedAt.IsZero() || d.ValidUntil.IsZero() {
		result.Reasons = append(result.Reasons, "decision timestamps required")
	}
	if !d.GeneratedAt.IsZero() && d.GeneratedAt.After(p.Now.Add(2*time.Minute)) {
		result.Reasons = append(result.Reasons, "decision generated_at is in the future")
	}
	if !d.ValidUntil.IsZero() && !p.Now.Before(d.ValidUntil) {
		result.Reasons = append(result.Reasons, "decision expired")
	}
	if !d.GeneratedAt.IsZero() && !d.ValidUntil.IsZero() && p.MaxDecisionTTL > 0 && d.ValidUntil.Sub(d.GeneratedAt) > p.MaxDecisionTTL {
		result.Reasons = append(result.Reasons, "decision TTL exceeds policy")
	}
	if len(d.Actions) > p.MaxActions && p.MaxActions > 0 {
		result.Reasons = append(result.Reasons, "too many actions")
	}

	globalValid := len(result.Reasons) == 0
	for idx, a := range d.Actions {
		a.Symbol = strings.ToUpper(strings.TrimSpace(a.Symbol))
		actionReasons := validateAction(idx, a, p)
		result.Reasons = append(result.Reasons, actionReasons...)
		if globalValid && len(actionReasons) == 0 {
			result.Actions = append(result.Actions, a)
		}
	}
	return result
}

func validateAction(idx int, a Action, p ValidationPolicy) []string {
	reasons := []string{}
	if a.Symbol == "" {
		return append(reasons, fmt.Sprintf("action %d: symbol required", idx))
	}
	if len(p.AllowedSymbols) > 0 && !p.AllowedSymbols[a.Symbol] {
		reasons = append(reasons, a.Symbol+": symbol not allowed")
	}
	if !a.Intent.IsKnown() {
		return append(reasons, a.Symbol+": unknown intent")
	}
	if math.IsNaN(a.Confidence) || math.IsInf(a.Confidence, 0) || a.Confidence < 0 || a.Confidence > 1 {
		reasons = append(reasons, a.Symbol+": invalid confidence")
	}
	if a.Intent.IncreasesExposure() {
		if a.Confidence < p.MinConfidence {
			reasons = append(reasons, a.Symbol+": confidence below floor")
		}
		if !positiveFinite(a.RequestedNotionalUSDT) {
			reasons = append(reasons, a.Symbol+": positive notional required")
		}
		if p.MaxActionNotionalUSDT > 0 && a.RequestedNotionalUSDT > p.MaxActionNotionalUSDT {
			reasons = append(reasons, a.Symbol+": notional exceeds action cap")
		}
		if a.Intent == IntentProbeLimit && p.MaxProbeNotionalUSDT > 0 && a.RequestedNotionalUSDT > p.MaxProbeNotionalUSDT {
			reasons = append(reasons, a.Symbol+": probe notional exceeds probe cap")
		}
		if !positiveFinite(a.EntryPrice) {
			reasons = append(reasons, a.Symbol+": entry price required")
		}
		if a.MaxLayers < 0 {
			reasons = append(reasons, a.Symbol+": invalid max layers")
		}
	}
	if a.Intent == IntentReduce || a.Intent == IntentExitLimit {
		if !positiveFinite(a.RequestedNotionalUSDT) {
			reasons = append(reasons, a.Symbol+": positive reduce notional required")
		}
		if p.MaxActionNotionalUSDT > 0 && a.RequestedNotionalUSDT > p.MaxActionNotionalUSDT {
			reasons = append(reasons, a.Symbol+": reduce notional exceeds action cap")
		}
		if !positiveFinite(a.EntryPrice) {
			reasons = append(reasons, a.Symbol+": reduce limit price required")
		}
	}
	return reasons
}

func positiveFinite(v float64) bool { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }
