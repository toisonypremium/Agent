package learning

import (
	"fmt"
	"sort"
	"time"

	"btc-agent/internal/backtest"
	"btc-agent/internal/survey"
)

const (
	AreaFlowParams           = "FLOW_PARAMS"
	AreaBTCPermission        = "BTC_PERMISSION"
	AreaWatchlist            = "WATCHLIST"
	AreaChecklist            = "CHECKLIST"
	AreaLayering             = "LAYERING"
	AreaExit                 = "EXIT"
	AreaStrategyIntelligence = "STRATEGY_INTELLIGENCE"
	AreaDataQuality          = "DATA_QUALITY"

	ConfidenceLow    = "LOW"
	ConfidenceMedium = "MEDIUM"
	ConfidenceHigh   = "HIGH"

	SeverityInfo       = "INFO"
	SeverityWatch      = "WATCH"
	SeverityActionable = "ACTIONABLE"
)

type RecommendationResult struct {
	GeneratedAt     time.Time        `json:"generated_at"`
	Summary         string           `json:"summary"`
	SurveySummary   string           `json:"survey_summary,omitempty"`
	EvidenceQuality string           `json:"evidence_quality,omitempty"`
	SurveyActions   []SurveyAction   `json:"survey_actions,omitempty"`
	Recommendations []Recommendation `json:"recommendations"`
}

type SurveyAction struct {
	Area           string     `json:"area"`
	Severity       string     `json:"severity"`
	Confidence     string     `json:"confidence"`
	Title          string     `json:"title"`
	Recommendation string     `json:"recommendation"`
	ManualAction   string     `json:"manual_action"`
	Evidence       []Evidence `json:"evidence,omitempty"`
}

type Recommendation struct {
	Area           string     `json:"area"`
	Title          string     `json:"title"`
	Recommendation string     `json:"recommendation"`
	ManualAction   string     `json:"manual_action"`
	Confidence     string     `json:"confidence"`
	Severity       string     `json:"severity"`
	Evidence       []Evidence `json:"evidence,omitempty"`
}

type Evidence struct {
	Metric string `json:"metric"`
	Value  string `json:"value"`
	Note   string `json:"note,omitempty"`
}

func BuildRecommendations(result backtest.Result) RecommendationResult {
	out := RecommendationResult{GeneratedAt: time.Now()}
	out.Recommendations = append(out.Recommendations, flowRecommendations(result)...)
	out.Recommendations = append(out.Recommendations, btcPermissionRecommendations(result)...)
	out.Recommendations = append(out.Recommendations, watchlistRecommendations(result)...)
	out.Recommendations = append(out.Recommendations, checklistRecommendations(result)...)
	out.Recommendations = append(out.Recommendations, opportunityRecommendations(result)...)
	out.Recommendations = append(out.Recommendations, filterValueRecommendations(result)...)
	out.Recommendations = append(out.Recommendations, strategyIntelligenceRecommendations(result)...)
	out.Recommendations = append(out.Recommendations, layerRecommendations(result)...)
	out.Recommendations = append(out.Recommendations, exitRecommendations(result)...)
	finalizeRecommendations(&out, result)
	return out
}

func BuildRecommendationsWithSurvey(result backtest.Result, s survey.RealDataSurvey) RecommendationResult {
	out := BuildRecommendations(result)
	out.SurveySummary = s.Summary
	out.EvidenceQuality = s.DataCoverage.Confidence
	out.SurveyActions = convertSurveyActions(s.LearningActions)
	for _, action := range out.SurveyActions {
		out.Recommendations = append(out.Recommendations, Recommendation{
			Area:           "SURVEY_" + action.Area,
			Title:          action.Title,
			Recommendation: action.Recommendation,
			ManualAction:   action.ManualAction,
			Confidence:     normalizeConfidence(action.Confidence),
			Severity:       normalizeSurveySeverity(action.Severity),
			Evidence:       action.Evidence,
		})
	}
	sortRecommendations(out.Recommendations)
	out.Summary = summarizeRecommendations(out.Recommendations) + "; survey_evidence=" + emptyDash(out.EvidenceQuality)
	return out
}

func finalizeRecommendations(out *RecommendationResult, result backtest.Result) {
	if len(out.Recommendations) == 0 {
		out.Recommendations = append(out.Recommendations, Recommendation{
			Area:           AreaDataQuality,
			Title:          "Need more audit evidence",
			Recommendation: "Keep collecting local candles and rerun backtest/learn before changing rules.",
			ManualAction:   "Run fetch and backtest over a larger sample; do not tune production rules from weak evidence.",
			Confidence:     ConfidenceLow,
			Severity:       SeverityInfo,
			Evidence:       []Evidence{{Metric: "windows_tested", Value: fmt.Sprint(result.WindowsTested)}},
		})
	}
	sortRecommendations(out.Recommendations)
	out.Summary = summarizeRecommendations(out.Recommendations)
}

func convertSurveyActions(actions []survey.SurveyAction) []SurveyAction {
	out := make([]SurveyAction, 0, len(actions))
	for _, action := range actions {
		out = append(out, SurveyAction{Area: action.Area, Severity: action.Severity, Confidence: action.Confidence, Title: action.Title, Recommendation: action.Recommendation, ManualAction: action.ManualAction, Evidence: convertSurveyEvidence(action.Evidence)})
	}
	return out
}

func convertSurveyEvidence(items []survey.SurveyEvidence) []Evidence {
	out := make([]Evidence, 0, len(items))
	for _, item := range items {
		out = append(out, Evidence{Metric: item.Metric, Value: item.Value, Note: item.Note})
	}
	return out
}

func normalizeSurveySeverity(severity string) string {
	switch severity {
	case survey.SeverityActionableReview:
		return SeverityActionable
	case survey.SeverityWatch, survey.SeverityBlockedBySafety:
		return SeverityWatch
	default:
		return SeverityInfo
	}
}

func normalizeConfidence(confidence string) string {
	switch confidence {
	case ConfidenceHigh, ConfidenceMedium, ConfidenceLow:
		return confidence
	default:
		return ConfidenceLow
	}
}

func flowRecommendations(result backtest.Result) []Recommendation {
	if !result.FlowParamQualityAudit.Enabled || len(result.FlowParamQualityAudit.Rows) == 0 {
		return nil
	}
	rows := append([]backtest.FlowParamQualityAuditRow(nil), result.FlowParamQualityAudit.Rows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Score > rows[j].Score })
	var current backtest.FlowParamQualityAuditRow
	for _, row := range rows {
		if row.Name == "current" {
			current = row
			break
		}
	}
	out := []Recommendation{}
	for _, row := range rows {
		if row.Verdict != backtest.FlowParamQualityCandidateTune {
			continue
		}
		out = append(out, Recommendation{
			Area:           AreaFlowParams,
			Title:          "Review flow parameter candidate " + row.Name,
			Recommendation: "Candidate flow params outperformed current audit baseline. Review manually before any config/rule change.",
			ManualAction:   "Compare candidate params against current flow.DefaultParams(), rerun backtest on more data, and change code only after manual review.",
			Confidence:     confidenceByCount(row.BullishCount),
			Severity:       SeverityActionable,
			Evidence: []Evidence{
				{Metric: "candidate", Value: row.Name},
				{Metric: "candidate_bullish", Value: fmt.Sprint(row.BullishCount)},
				{Metric: "current_bullish", Value: fmt.Sprint(current.BullishCount)},
				{Metric: "added_bullish", Value: fmt.Sprint(row.AddedBullishCount)},
				{Metric: "false_positive_rate", Value: pct(row.FalsePositiveRate)},
				{Metric: "score", Value: fmt.Sprintf("%.4f", row.Score)},
			},
		})
		break
	}
	return out
}

func btcPermissionRecommendations(result backtest.Result) []Recommendation {
	if !result.BTCPermissionAudit.Enabled || len(result.BTCPermissionAudit.Blockers) == 0 {
		return nil
	}
	top := result.BTCPermissionAudit.Blockers[0]
	if top.Rate < 0.35 || top.Count < 5 {
		return nil
	}
	return []Recommendation{{
		Area:           AreaBTCPermission,
		Title:          "Investigate dominant BTC permission blocker",
		Recommendation: "One BTC gate dominates blocked windows. Review this gate in isolation instead of loosening multiple safety filters.",
		ManualAction:   "Inspect historical rows for the blocker, compare forward returns/drawdowns, and keep safety gates active until evidence is strong.",
		Confidence:     confidenceByCount(top.Count),
		Severity:       SeverityWatch,
		Evidence:       []Evidence{{Metric: "blocker", Value: top.Blocker}, {Metric: "count", Value: fmt.Sprint(top.Count)}, {Metric: "rate", Value: pct(top.Rate)}},
	}}
}

func watchlistRecommendations(result backtest.Result) []Recommendation {
	if !result.WatchlistTriggerAudit.Enabled {
		return nil
	}
	for _, row := range result.WatchlistTriggerAudit.Rows {
		if row.Verdict != "CANDIDATE" {
			continue
		}
		return []Recommendation{{
			Area:           AreaWatchlist,
			Title:          "Prioritize watchlist trigger " + row.Trigger,
			Recommendation: "This actionable watchlist trigger showed favorable forward audit stats. Use it as a higher-priority manual watch context.",
			ManualAction:   "When status/report shows this trigger, review candidate first; do not bypass checklist or submit orders from this recommendation.",
			Confidence:     confidenceByCount(row.Count),
			Severity:       SeverityWatch,
			Evidence:       []Evidence{{Metric: "symbol", Value: row.Symbol}, {Metric: "trigger", Value: row.Trigger}, {Metric: "count", Value: fmt.Sprint(row.Count)}, {Metric: "avg_return_14d", Value: pct(row.AvgReturn[14])}, {Metric: "win_rate_14d", Value: pct(row.WinRate[14])}, {Metric: "worst_drawdown_14d", Value: pct(row.WorstDrawdown[14])}},
		}}
	}
	return nil
}

func checklistRecommendations(result backtest.Result) []Recommendation {
	if !result.ChecklistPassCountAudit.Enabled {
		return nil
	}
	for _, row := range result.ChecklistPassCountAudit.Rows {
		if row.Verdict != backtest.ChecklistVerdictNearActionableWatch {
			continue
		}
		blocker := row.TopHardBlocker
		if blocker == "" {
			blocker = row.TopSoftWait
		}
		return []Recommendation{{
			Area:           AreaChecklist,
			Title:          "Review near-actionable checklist bottleneck for " + row.Symbol,
			Recommendation: "Many samples are near actionable with repeated checklist waits. Review the repeated blocker manually.",
			ManualAction:   "Inspect candidate history for the blocker and decide whether reporting/threshold wording should improve; do not auto-loosen gates.",
			Confidence:     confidenceByCount(row.Samples),
			Severity:       SeverityWatch,
			Evidence:       []Evidence{{Metric: "symbol", Value: row.Symbol}, {Metric: "near_actionable", Value: fmt.Sprint(row.NearActionableCount)}, {Metric: "samples", Value: fmt.Sprint(row.Samples)}, {Metric: "top_blocker", Value: blocker}, {Metric: "hard_fail_rate", Value: pct(row.HardFailRate)}},
		}}
	}
	return nil
}

func opportunityRecommendations(result backtest.Result) []Recommendation {
	if !result.Agent2OpportunityAudit.Enabled {
		return nil
	}
	for _, row := range result.Agent2OpportunityAudit.Rows {
		if row.Samples < 10 {
			continue
		}
		severity := SeverityWatch
		if row.ResearchOnlyVerdict == backtest.OpportunityVerdictTuneReview {
			severity = SeverityActionable
		}
		return []Recommendation{{
			Area:           AreaChecklist,
			Title:          "Review Agent2 opportunity bottleneck for " + row.Symbol,
			Recommendation: row.RecommendedAction,
			ManualAction:   "Use backtest/learn evidence for manual review only; no live config was changed and WATCH/SCOUT/ARMED must not create orders.",
			Confidence:     confidenceByCount(row.Samples),
			Severity:       severity,
			Evidence: []Evidence{
				{Metric: "symbol", Value: row.Symbol},
				{Metric: "samples", Value: fmt.Sprint(row.Samples)},
				{Metric: "active_limit", Value: fmt.Sprint(row.ActiveLimitCount)},
				{Metric: "near_miss", Value: fmt.Sprint(row.NearMissCount)},
				{Metric: "top_missing", Value: row.TopMissingGate},
				{Metric: "verdict", Value: row.ResearchOnlyVerdict},
			},
		}}
	}
	return nil
}

func filterValueRecommendations(result backtest.Result) []Recommendation {
	if !result.FilterValueAudit.Enabled {
		return nil
	}
	for _, row := range result.FilterValueAudit.Rows {
		if row.Verdict != backtest.FilterValueTuneReview || row.Samples < 20 {
			continue
		}
		return []Recommendation{{
			Area:           AreaChecklist,
			Title:          "Review possible false-negative filter " + row.Filter,
			Recommendation: "Filter value audit found blocked samples with acceptable forward returns. Review this filter manually before any rule/config change.",
			ManualAction:   "Compare blocked vs passed rows across more candles; no live config changed; do not bypass ACTIVE_LIMIT; WATCH/SCOUT/ARMED must not create normal live orders.",
			Confidence:     confidenceByCount(row.Samples),
			Severity:       SeverityActionable,
			Evidence: []Evidence{
				{Metric: "filter", Value: row.Filter},
				{Metric: "samples", Value: fmt.Sprint(row.Samples)},
				{Metric: "blocked", Value: fmt.Sprint(row.Blocked)},
				{Metric: "false_negative_rate", Value: pct(row.FalseNegativeRate)},
				{Metric: "verdict", Value: row.Verdict},
			},
		}}
	}
	return nil
}

func strategyIntelligenceRecommendations(result backtest.Result) []Recommendation {
	if !result.Agent2OpportunityAudit.Enabled && !result.BTCPermissionAudit.Enabled && !result.ExitAudit.Enabled {
		return nil
	}
	evidence := []Evidence{}
	if result.BTCPermissionAudit.Enabled && len(result.BTCPermissionAudit.Blockers) > 0 {
		top := result.BTCPermissionAudit.Blockers[0]
		evidence = append(evidence, Evidence{Metric: "btc_top_blocker", Value: top.Blocker, Note: fmt.Sprintf("count=%d rate=%s", top.Count, pct(top.Rate))})
	}
	if result.Agent2OpportunityAudit.Enabled {
		for _, row := range result.Agent2OpportunityAudit.Rows {
			if row.Samples <= 0 {
				continue
			}
			evidence = append(evidence, Evidence{Metric: "agent2_top_missing", Value: row.TopMissingGate, Note: row.Symbol})
			break
		}
	}
	if result.ExitAudit.Enabled {
		for _, row := range result.ExitAudit.Rows {
			if row.OrdersPlaced <= 0 {
				continue
			}
			evidence = append(evidence, Evidence{Metric: "exit_research", Value: row.Verdict, Note: row.Symbol})
			break
		}
	}
	if len(evidence) == 0 {
		return nil
	}
	return []Recommendation{{
		Area:           AreaStrategyIntelligence,
		Title:          "Use strategy intelligence as diagnostic context only",
		Recommendation: "Prioritize BTC gate gaps, Agent2 closest-unlock gates, and exit research in the next manual review. Do not treat this as trade authority.",
		ManualAction:   "Manual review required; no live config changed; no order authority changed; WATCH/SCOUT/ARMED must not create orders; do not place take-profit orders automatically.",
		Confidence:     ConfidenceMedium,
		Severity:       SeverityWatch,
		Evidence:       evidence,
	}}
}

func layerRecommendations(result backtest.Result) []Recommendation {
	if !result.LayerAudit.Enabled {
		return nil
	}
	for _, row := range result.LayerAudit.Rows {
		if row.Verdict != "CANDIDATE" {
			continue
		}
		return []Recommendation{{
			Area:           AreaLayering,
			Title:          "Review layer settings for " + row.Symbol,
			Recommendation: "Layer audit found a candidate invalidation buffer and layer depth for manual review.",
			ManualAction:   "Compare candidate to current production assumptions, rerun simulation, and update rules only through review/PR.",
			Confidence:     confidenceByCount(row.OrdersPlaced),
			Severity:       SeverityActionable,
			Evidence:       []Evidence{{Metric: "symbol", Value: row.Symbol}, {Metric: "invalidation_buffer", Value: fmt.Sprintf("%.3f", row.InvalidationBuffer)}, {Metric: "layer_depth_multiplier", Value: fmt.Sprintf("%.2f", row.LayerDepthMultiplier)}, {Metric: "orders_filled", Value: fmt.Sprint(row.OrdersFilled)}, {Metric: "invalidations", Value: fmt.Sprint(row.Invalidations)}, {Metric: "final_pnl", Value: fmt.Sprintf("%.2f", row.FinalPnL)}, {Metric: "max_drawdown", Value: pct(row.MaxDrawdown)}},
		}}
	}
	return nil
}

func exitRecommendations(result backtest.Result) []Recommendation {
	if !result.ExitAudit.Enabled {
		return nil
	}
	for _, row := range result.ExitAudit.Rows {
		if row.Verdict != "CANDIDATE" {
			continue
		}
		return []Recommendation{{
			Area:           AreaExit,
			Title:          "Review exit settings for " + row.Symbol,
			Recommendation: "Exit audit found a candidate take-profit/time-stop combination for manual review.",
			ManualAction:   "Review candidate against invalidation behavior and position-management policy; research-only/manual review required; no live config changed; do not place take-profit orders automatically.",
			Confidence:     confidenceByCount(row.OrdersPlaced),
			Severity:       SeverityActionable,
			Evidence:       []Evidence{{Metric: "symbol", Value: row.Symbol}, {Metric: "take_profit", Value: pct(row.TakeProfitPct)}, {Metric: "time_stop_days", Value: fmt.Sprint(row.TimeStopDays)}, {Metric: "take_profits", Value: fmt.Sprint(row.TakeProfits)}, {Metric: "invalidations", Value: fmt.Sprint(row.Invalidations)}, {Metric: "final_pnl", Value: fmt.Sprintf("%.2f", row.FinalPnL)}, {Metric: "max_drawdown", Value: pct(row.MaxDrawdown)}},
		}}
	}
	return nil
}

func confidenceByCount(count int) string {
	switch {
	case count >= 30:
		return ConfidenceHigh
	case count >= 10:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}

func pct(v float64) string {
	return fmt.Sprintf("%.1f%%", v*100)
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func sortRecommendations(rows []Recommendation) {
	sev := map[string]int{SeverityActionable: 0, SeverityWatch: 1, SeverityInfo: 2}
	conf := map[string]int{ConfidenceHigh: 0, ConfidenceMedium: 1, ConfidenceLow: 2}
	sort.Slice(rows, func(i, j int) bool {
		if sev[rows[i].Severity] != sev[rows[j].Severity] {
			return sev[rows[i].Severity] < sev[rows[j].Severity]
		}
		if conf[rows[i].Confidence] != conf[rows[j].Confidence] {
			return conf[rows[i].Confidence] < conf[rows[j].Confidence]
		}
		return rows[i].Area < rows[j].Area
	})
}

func summarizeRecommendations(rows []Recommendation) string {
	actionable := 0
	watch := 0
	for _, row := range rows {
		switch row.Severity {
		case SeverityActionable:
			actionable++
		case SeverityWatch:
			watch++
		}
	}
	return fmt.Sprintf("Learning recommendations total=%d actionable=%d watch=%d manual_review_required=true", len(rows), actionable, watch)
}
