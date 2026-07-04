package aiagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"btc-agent/internal/agent1"
	"btc-agent/internal/agent2"
)

type JSONCaller interface {
	ChatJSON(ctx context.Context, prompt string, out any) error
}

type Snapshot struct {
	Analysis agent1.MarketAnalysis `json:"analysis"`
	Plan     agent2.Plan           `json:"plan"`
	Status   string                `json:"status"`
}

type Report struct {
	DeterministicDecision string       `json:"deterministic_decision"`
	Summary               string       `json:"summary"`
	Blockers              []string     `json:"blockers"`
	WatchTriggers         []string     `json:"watch_triggers"`
	RiskWarnings          []string     `json:"risk_warnings"`
	TelegramText          string       `json:"telegram_text"`
	OverrideEngine        bool         `json:"override_engine"`
	Safety                SafetyResult `json:"safety"`
}

func Generate(ctx context.Context, caller JSONCaller, snap Snapshot) (Report, error) {
	if caller == nil {
		return Fallback(snap, SafetyResult{Pass: true}), nil
	}
	var report Report
	if err := caller.ChatJSON(ctx, Prompt(snap), &report); err != nil {
		return Fallback(snap, SafetyResult{Pass: true}), err
	}
	if report.DeterministicDecision == "" {
		report.DeterministicDecision = string(snap.Analysis.ActionPermission)
	}
	safety := CheckSafety(report.TelegramText+" "+report.Summary+" "+strings.Join(report.RiskWarnings, " "), report.OverrideEngine)
	report.Safety = safety
	if !safety.Pass || report.DeterministicDecision != string(snap.Analysis.ActionPermission) {
		return Fallback(snap, safety), nil
	}
	return report, nil
}

func Prompt(snap Snapshot) string {
	payload, _ := json.MarshalIndent(promptSnapshot(snap), "", "  ")
	return fmt.Sprintf(`Return exactly one valid JSON object. No markdown.
You are AI reporter for a deterministic crypto trading system.
The deterministic engine is the authority. Copy deterministic_decision exactly as %q.
Explain only. Do not place orders. Do not recommend real trading.
No futures. No leverage. No market orders. Report/watch only.
JSON schema:
{"deterministic_decision":"%s","summary":"short Vietnamese summary","blockers":["..."],"watch_triggers":["..."],"risk_warnings":["..."],"telegram_text":"Telegram-ready Vietnamese text, max 1200 chars","override_engine":false}
Snapshot:
%s`, snap.Analysis.ActionPermission, snap.Analysis.ActionPermission, string(payload))
}

func promptSnapshot(snap Snapshot) any {
	candidates := []map[string]any{}
	limit := len(snap.Plan.Watchlist.Candidates)
	if limit > 3 {
		limit = 3
	}
	for _, c := range snap.Plan.Watchlist.Candidates[:limit] {
		candidates = append(candidates, map[string]any{
			"symbol":            c.Symbol,
			"readiness_score":   c.ReadinessScore,
			"tier":              c.Tier,
			"actionable":        c.Actionable,
			"noise_flags":       c.NoiseFlags,
			"checklist_summary": agent2.ChecklistSummary(c.EntryChecklist),
			"missing":           c.Missing,
			"next_trigger":      c.NextTrigger,
		})
	}
	return map[string]any{
		"deterministic_decision": snap.Analysis.ActionPermission,
		"btc": map[string]any{
			"price":              snap.Analysis.BTCPrice,
			"regime":             snap.Analysis.MarketRegime,
			"trend_score":        snap.Analysis.TrendScore,
			"risk_level":         snap.Analysis.RiskLevel,
			"falling_knife_risk": snap.Analysis.FallingKnifeRisk,
			"fomo_risk":          snap.Analysis.FomoRisk,
			"flow_bias":          snap.Analysis.Flow.Bias,
			"flow_score":         snap.Analysis.Flow.Score,
		},
		"plan_state":           snap.Plan.State,
		"plan_summary":         snap.Plan.Summary,
		"watchlist_candidates": candidates,
	}
}

func Fallback(snap Snapshot, safety SafetyResult) Report {
	blockers := []string{}
	if snap.Analysis.ActionPermission != agent1.Allowed {
		blockers = append(blockers, "BTC permission chưa ALLOWED")
	}
	if snap.Analysis.RiskLevel == agent1.High {
		blockers = append(blockers, "BTC risk HIGH")
	}
	if snap.Analysis.FallingKnifeRisk == agent1.High {
		blockers = append(blockers, "falling knife risk HIGH")
	}
	if snap.Analysis.FomoRisk == agent1.High {
		blockers = append(blockers, "FOMO risk HIGH")
	}
	for _, c := range snap.Plan.Watchlist.Candidates {
		blockers = append(blockers, c.NoiseFlags...)
		if len(blockers) >= 8 {
			break
		}
	}
	text := fmt.Sprintf("BTC AGENT WATCH\nDeterministic decision: %s\nRegime: %s | Risk: %s | Flow: %s %.2f\nMode: report/watch only. No real orders.\n", snap.Analysis.ActionPermission, snap.Analysis.MarketRegime, snap.Analysis.RiskLevel, snap.Analysis.Flow.Bias, snap.Analysis.Flow.Score)
	if len(blockers) > 0 {
		text += "Blockers: " + strings.Join(unique(blockers), "; ") + "\n"
	}
	if len(snap.Plan.Watchlist.Candidates) > 0 {
		best := snap.Plan.Watchlist.Candidates[0]
		text += fmt.Sprintf("Closest: %s readiness %.2f tier=%s next=%s\n", best.Symbol, best.ReadinessScore, best.Tier, best.NextTrigger)
	}
	finalSafety := CheckSafety(text, false)
	if len(safety.Reasons) > 0 && finalSafety.Pass {
		finalSafety.Reasons = append([]string{"AI_OUTPUT_BLOCKED_FALLBACK_USED"}, safety.Reasons...)
	}
	return Report{DeterministicDecision: string(snap.Analysis.ActionPermission), Summary: snap.Analysis.Summary, Blockers: unique(blockers), TelegramText: text, OverrideEngine: false, Safety: finalSafety}
}
