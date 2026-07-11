package survey

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"btc-agent/internal/backtest"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/microstructure"
)

const (
	SeverityActionableReview = "ACTIONABLE_REVIEW"
	SeverityWatch            = "WATCH"
	SeverityNoChange         = "NO_CHANGE"
	SeverityBlockedBySafety  = "BLOCKED_BY_SAFETY"

	ConfidenceLow    = "LOW"
	ConfidenceMedium = "MEDIUM"
	ConfidenceHigh   = "HIGH"
)

type RealDataSurvey struct {
	GeneratedAt       time.Time      `json:"generated_at"`
	DataCoverage      SurveyCoverage `json:"data_coverage"`
	BTCGate           SurveySection  `json:"btc_gate"`
	Agent2Gate        SurveySection  `json:"agent2_gate"`
	ManagedLive       SurveySection  `json:"managed_live"`
	AccumulationPhase SurveySection  `json:"accumulation_phase"`
	Microstructure    SurveySection  `json:"microstructure"`
	LearningActions   []SurveyAction `json:"learning_actions,omitempty"`
	RiskNotes         []string       `json:"risk_notes,omitempty"`
	Summary           string         `json:"summary"`
}

type SurveyCoverage struct {
	WindowsTested int       `json:"windows_tested"`
	PeriodStart   time.Time `json:"period_start,omitempty"`
	PeriodEnd     time.Time `json:"period_end,omitempty"`
	DataSanity    string    `json:"data_sanity,omitempty"`
	Confidence    string    `json:"confidence"`
	Summary       string    `json:"summary"`
}

type SurveySection struct {
	Area       string           `json:"area"`
	Verdict    string           `json:"verdict"`
	Confidence string           `json:"confidence"`
	Summary    string           `json:"summary"`
	Evidence   []SurveyEvidence `json:"evidence,omitempty"`
}

type SurveyEvidence struct {
	Metric string `json:"metric"`
	Value  string `json:"value"`
	Note   string `json:"note,omitempty"`
}

type SurveyAction struct {
	Area           string           `json:"area"`
	Severity       string           `json:"severity"`
	Confidence     string           `json:"confidence"`
	Title          string           `json:"title"`
	Recommendation string           `json:"recommendation"`
	ManualAction   string           `json:"manual_action"`
	Evidence       []SurveyEvidence `json:"evidence,omitempty"`
}

func Build(result backtest.Result, history *liveguard.LiveManagerHistoryResult) RealDataSurvey {
	return BuildWithMicrostructure(result, history, microstructure.Summary{})
}

func BuildWithMicrostructure(result backtest.Result, history *liveguard.LiveManagerHistoryResult, micro microstructure.Summary) RealDataSurvey {
	out := RealDataSurvey{GeneratedAt: time.Now()}
	out.DataCoverage = buildCoverage(result)
	out.BTCGate = buildBTCGateSection(result)
	out.Agent2Gate = buildAgent2GateSection(result)
	out.ManagedLive = buildManagedLiveSection(history)
	out.AccumulationPhase = buildAccumulationPhaseSection(result)
	out.Microstructure = buildMicrostructureSection(micro)
	out.RiskNotes = []string{
		"Survey is diagnostic only and does not write config.",
		"Survey does not place, cancel, or modify live orders.",
		"WATCH, SCOUT, and ARMED remain observation states; only ACTIVE_LIMIT + ALLOWED + ACCUMULATION_CONFIRMED can create normal live desired orders.",
	}
	out.LearningActions = surveyActions(out)
	out.Summary = summarize(out)
	return out
}

func buildCoverage(result backtest.Result) SurveyCoverage {
	coverage := SurveyCoverage{WindowsTested: result.WindowsTested, PeriodStart: result.PeriodStart, PeriodEnd: result.PeriodEnd, DataSanity: result.DataSanity.Status, Confidence: confidenceByWindows(result.WindowsTested)}
	coverage.Summary = fmt.Sprintf("real-data windows=%d period=%s->%s data_sanity=%s confidence=%s", result.WindowsTested, dateOrNA(result.PeriodStart), dateOrNA(result.PeriodEnd), emptyDefault(result.DataSanity.Status, "not_checked"), coverage.Confidence)
	return coverage
}

func buildBTCGateSection(result backtest.Result) SurveySection {
	section := SurveySection{Area: "BTC_GATE", Verdict: SeverityNoChange, Confidence: confidenceByWindows(result.WindowsTested), Summary: "BTC gate audit unavailable or not enough evidence."}
	if !result.BTCPermissionAudit.Enabled || len(result.BTCPermissionAudit.Blockers) == 0 {
		return section
	}
	top := result.BTCPermissionAudit.Blockers[0]
	section.Summary = fmt.Sprintf("top BTC blocker=%s count=%d rate=%s", top.Blocker, top.Count, pct(top.Rate))
	section.Evidence = []SurveyEvidence{{Metric: "top_blocker", Value: top.Blocker}, {Metric: "count", Value: fmt.Sprint(top.Count)}, {Metric: "rate", Value: pct(top.Rate)}}
	section.Confidence = confidenceByCount(top.Count)
	if top.Count >= 10 && top.Rate >= 0.35 {
		section.Verdict = SeverityWatch
	}
	if top.Count >= 30 && top.Rate >= 0.60 {
		section.Verdict = SeverityActionableReview
	}
	return section
}

func buildAgent2GateSection(result backtest.Result) SurveySection {
	section := SurveySection{Area: "AGENT2_GATE", Verdict: SeverityNoChange, Confidence: confidenceByWindows(result.WindowsTested), Summary: "Agent2 opportunity audit unavailable or not enough evidence."}
	if !result.Agent2OpportunityAudit.Enabled || len(result.Agent2OpportunityAudit.Rows) == 0 {
		return section
	}
	row := topOpportunityRow(result.Agent2OpportunityAudit.Rows)
	section.Summary = fmt.Sprintf("top near-miss=%s samples=%d near=%d top_missing=%s verdict=%s", row.Symbol, row.Samples, row.NearMissCount, emptyDefault(row.TopMissingGate, "n/a"), row.ResearchOnlyVerdict)
	section.Evidence = []SurveyEvidence{
		{Metric: "symbol", Value: row.Symbol},
		{Metric: "samples", Value: fmt.Sprint(row.Samples)},
		{Metric: "active_limit", Value: fmt.Sprint(row.ActiveLimitCount)},
		{Metric: "near_miss", Value: fmt.Sprint(row.NearMissCount)},
		{Metric: "top_missing", Value: emptyDefault(row.TopMissingGate, "n/a")},
		{Metric: "research_verdict", Value: row.ResearchOnlyVerdict},
	}
	section.Confidence = confidenceByCount(row.Samples)
	if row.Samples >= 10 && row.NearMissCount > 0 {
		section.Verdict = SeverityWatch
	}
	if row.Samples >= 20 && row.ResearchOnlyVerdict == backtest.OpportunityVerdictTuneReview {
		section.Verdict = SeverityActionableReview
	}
	return section
}

func buildAccumulationPhaseSection(result backtest.Result) SurveySection {
	section := SurveySection{Area: "ACCUMULATION_PHASE", Verdict: SeverityNoChange, Confidence: confidenceByWindows(result.WindowsTested), Summary: "Accumulation phase audit unavailable or not enough evidence."}
	if !result.AccumulationPhaseAudit.Enabled || len(result.AccumulationPhaseAudit.Rows) == 0 {
		return section
	}
	var confirmed backtest.AccumulationPhaseAuditRow
	for _, row := range result.AccumulationPhaseAudit.Rows {
		if string(row.Phase) == "ACCUMULATION_CONFIRMED" {
			confirmed = row
			break
		}
	}
	if confirmed.Count == 0 {
		section.Summary = fmt.Sprintf("BTC accumulation phase audit found no confirmed samples; phases=%d", len(result.AccumulationPhaseAudit.Rows))
		section.Evidence = []SurveyEvidence{{Metric: "confirmed_count", Value: "0"}}
		section.Verdict = SeverityWatch
		return section
	}
	section.Summary = fmt.Sprintf("BTC confirmed accumulation samples=%d avg_score=%.1f false_positive=%s verdict=%s", confirmed.Count, confirmed.AvgScore, pct(confirmed.FalsePositiveRate), confirmed.Verdict)
	section.Evidence = []SurveyEvidence{{Metric: "confirmed_count", Value: fmt.Sprint(confirmed.Count)}, {Metric: "avg_score", Value: fmt.Sprintf("%.1f", confirmed.AvgScore)}, {Metric: "false_positive_rate", Value: pct(confirmed.FalsePositiveRate)}, {Metric: "verdict", Value: confirmed.Verdict}}
	section.Confidence = confidenceByCount(confirmed.Count)
	section.Verdict = SeverityWatch
	if confirmed.Count >= 20 && confirmed.Verdict == "CANDIDATE" {
		section.Verdict = SeverityActionableReview
	}
	return section
}

func buildMicrostructureSection(summary microstructure.Summary) SurveySection {
	section := SurveySection{Area: "MICROSTRUCTURE", Verdict: SeverityNoChange, Confidence: ConfidenceLow, Summary: "Microstructure unavailable or disabled."}
	if !summary.Enabled {
		return section
	}
	section.Summary = fmt.Sprintf("microstructure status=%s fresh=%d/%d blockers=%d warnings=%d", summary.Status, summary.FreshSymbols, summary.RequiredFresh, len(summary.Blockers), len(summary.Warnings))
	section.Evidence = []SurveyEvidence{{Metric: "status", Value: summary.Status}, {Metric: "fresh_symbols", Value: fmt.Sprint(summary.FreshSymbols)}, {Metric: "required_fresh", Value: fmt.Sprint(summary.RequiredFresh)}, {Metric: "blockers", Value: fmt.Sprint(len(summary.Blockers))}}
	if summary.BTC.Symbol != "" {
		section.Evidence = append(section.Evidence, SurveyEvidence{Metric: "btc_taker_buy_ratio", Value: fmt.Sprintf("%.1f%%", summary.BTC.SpotFlow.TakerBuyRatio*100)}, SurveyEvidence{Metric: "btc_cvd_quote_usdt", Value: fmt.Sprintf("%.2f", summary.BTC.SpotFlow.CVDQuoteUSDT)}, SurveyEvidence{Metric: "btc_funding_rate", Value: fmt.Sprintf("%.4f", summary.BTC.Futures.FundingRate)}, SurveyEvidence{Metric: "btc_basis_pct", Value: fmt.Sprintf("%.2f%%", summary.BTC.Futures.BasisPct)})
	}
	section.Confidence = confidenceByCount(summary.FreshSymbols)
	if summary.Status == microstructure.StatusBlock || len(summary.Blockers) > 0 {
		section.Verdict = SeverityBlockedBySafety
	} else if summary.Status == microstructure.StatusWarn || len(summary.Warnings) > 0 {
		section.Verdict = SeverityWatch
	}
	return section
}

func buildManagedLiveSection(history *liveguard.LiveManagerHistoryResult) SurveySection {
	section := SurveySection{Area: "MANAGED_LIVE", Verdict: SeverityNoChange, Confidence: ConfidenceLow, Summary: "live manager history simulation missing; run backtest-live-manager for quality evidence."}
	if history == nil {
		return section
	}
	stats := history.Total
	section.Summary = fmt.Sprintf("managed history placed=%d filled=%d canceled=%d replaced=%d blocked=%d fill_rate=%s cancel_rate=%s quality=%s %.1f", stats.Placed, stats.Filled, stats.Canceled, stats.Replaced, stats.Blocked, pct(stats.FillRate), pct(stats.CancelRate), emptyDefault(stats.QualityGrade, "NO_SAMPLE"), stats.QualityScore)
	section.Evidence = []SurveyEvidence{
		{Metric: "windows", Value: fmt.Sprint(history.WindowsTested)},
		{Metric: "placed", Value: fmt.Sprint(stats.Placed)},
		{Metric: "filled", Value: fmt.Sprint(stats.Filled)},
		{Metric: "canceled", Value: fmt.Sprint(stats.Canceled)},
		{Metric: "fill_rate", Value: pct(stats.FillRate)},
		{Metric: "cancel_rate", Value: pct(stats.CancelRate)},
		{Metric: "quality_grade", Value: emptyDefault(stats.QualityGrade, "NO_SAMPLE")},
		{Metric: "quality_score", Value: fmt.Sprintf("%.1f", stats.QualityScore)},
	}
	section.Confidence = confidenceByCount(stats.Placed)
	if stats.Placed == 0 {
		section.Verdict = SeverityNoChange
		return section
	}
	if strings.EqualFold(stats.QualityGrade, "D") || stats.CancelRate > 0.50 {
		section.Verdict = SeverityWatch
		return section
	}
	if stats.Placed >= 10 && stats.FillRate >= 0.45 && stats.CancelRate <= 0.25 {
		section.Verdict = SeverityActionableReview
	}
	return section
}

func surveyActions(s RealDataSurvey) []SurveyAction {
	actions := []SurveyAction{}
	if s.DataCoverage.Confidence == ConfidenceLow {
		actions = append(actions, SurveyAction{Area: "DATA_QUALITY", Severity: SeverityNoChange, Confidence: ConfidenceLow, Title: "Collect more local history", Recommendation: "Real-data sample is still weak; keep fetching candles and rerun survey before changing rules.", ManualAction: "Run fetch/backtest/backtest-live-manager again after more history. Do not tune production config from low sample.", Evidence: []SurveyEvidence{{Metric: "windows_tested", Value: fmt.Sprint(s.DataCoverage.WindowsTested)}}})
	}
	if s.BTCGate.Verdict == SeverityActionableReview || s.BTCGate.Verdict == SeverityWatch {
		actions = append(actions, SurveyAction{Area: s.BTCGate.Area, Severity: s.BTCGate.Verdict, Confidence: s.BTCGate.Confidence, Title: "Review BTC gate bottleneck", Recommendation: "BTC permission is the dominant blocker in real-data audit. Review the blocker in isolation instead of loosening multiple gates.", ManualAction: "Inspect BTC permission audit and forward returns; any threshold change must be a separate reviewed code/config change, not an automatic learning action.", Evidence: s.BTCGate.Evidence})
	}
	if s.Agent2Gate.Verdict == SeverityActionableReview || s.Agent2Gate.Verdict == SeverityWatch {
		actions = append(actions, SurveyAction{Area: s.Agent2Gate.Area, Severity: s.Agent2Gate.Verdict, Confidence: s.Agent2Gate.Confidence, Title: "Review Agent2 near-miss bottleneck", Recommendation: "Agent2 opportunity audit found repeated near-miss gates. Use this to prioritize manual review of the top missing gate.", ManualAction: "Review backtest rows for the missing gate; do not bypass ACTIVE_LIMIT or allow WATCH/SCOUT/ARMED to create normal live orders.", Evidence: s.Agent2Gate.Evidence})
	}
	if s.ManagedLive.Verdict == SeverityActionableReview || s.ManagedLive.Verdict == SeverityWatch {
		actions = append(actions, SurveyAction{Area: s.ManagedLive.Area, Severity: s.ManagedLive.Verdict, Confidence: s.ManagedLive.Confidence, Title: "Review managed order quality", Recommendation: "Historical managed-engine quality should guide sizing review only inside existing live guards.", ManualAction: "Use quality grade to review sizing/caps manually. Do not change live order authority; keep ACTIVE_LIMIT + ALLOWED required.", Evidence: s.ManagedLive.Evidence})
	}
	if s.AccumulationPhase.Verdict == SeverityActionableReview || s.AccumulationPhase.Verdict == SeverityWatch {
		actions = append(actions, SurveyAction{Area: s.AccumulationPhase.Area, Severity: s.AccumulationPhase.Verdict, Confidence: s.AccumulationPhase.Confidence, Title: "Review BTC accumulation phase evidence", Recommendation: "Use accumulation false-positive and forward-return evidence before allowing any gate threshold change.", ManualAction: "Review confirmed vs falling-knife rows manually; do not bypass ACTIVE_LIMIT + ALLOWED or auto-tune config.", Evidence: s.AccumulationPhase.Evidence})
	}
	actions = append(actions, SurveyAction{Area: "LIVE_SAFETY", Severity: SeverityBlockedBySafety, Confidence: ConfidenceHigh, Title: "Keep survey report-only", Recommendation: "Survey/learning output must not write config, place orders, or bypass live gates.", ManualAction: "Apply any rule change only through reviewed implementation and full verification.", Evidence: []SurveyEvidence{{Metric: "required_gate", Value: "ACTIVE_LIMIT + ALLOWED"}}})
	sort.SliceStable(actions, func(i, j int) bool {
		return severityRank(actions[i].Severity) < severityRank(actions[j].Severity)
	})
	return actions
}

func summarize(s RealDataSurvey) string {
	return fmt.Sprintf("real-data survey: windows=%d confidence=%s btc=%s accumulation=%s micro=%s agent2=%s managed=%s actions=%d report_only=true", s.DataCoverage.WindowsTested, s.DataCoverage.Confidence, s.BTCGate.Verdict, s.AccumulationPhase.Verdict, s.Microstructure.Verdict, s.Agent2Gate.Verdict, s.ManagedLive.Verdict, len(s.LearningActions))
}

func topOpportunityRow(rows []backtest.Agent2OpportunityAuditRow) backtest.Agent2OpportunityAuditRow {
	if len(rows) == 0 {
		return backtest.Agent2OpportunityAuditRow{}
	}
	best := rows[0]
	for _, row := range rows[1:] {
		if row.NearMissCount > best.NearMissCount || (row.NearMissCount == best.NearMissCount && row.Samples > best.Samples) {
			best = row
		}
	}
	return best
}

func confidenceByWindows(windows int) string {
	return confidenceByCount(windows)
}

func confidenceByCount(count int) string {
	switch {
	case count >= 120:
		return ConfidenceHigh
	case count >= 30:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}

func severityRank(severity string) int {
	switch severity {
	case SeverityActionableReview:
		return 0
	case SeverityWatch:
		return 1
	case SeverityBlockedBySafety:
		return 2
	case SeverityNoChange:
		return 3
	default:
		return 4
	}
}

func pct(v float64) string {
	return fmt.Sprintf("%.1f%%", v*100)
}

func dateOrNA(t time.Time) string {
	if t.IsZero() {
		return "n/a"
	}
	return t.Format("2006-01-02")
}

func emptyDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
