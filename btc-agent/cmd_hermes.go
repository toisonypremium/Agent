package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/freeapi"
	"btc-agent/internal/hermesagent"
	"btc-agent/internal/hermesmemory"
	"btc-agent/internal/hermesoperator"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/llm"
	"btc-agent/internal/microstructure"
	"btc-agent/internal/reportio"
	"btc-agent/internal/storage"
)

const hermesReportDir = "reports"
const hermesStateFile = "hermes_state.json"

func runHermesCycle(ctx context.Context, cfg config.Config, db *storage.DB) error {
	return runHermesCycleWithTrigger(ctx, cfg, db, hermesagent.HermesTrigger{Source: "scheduled", Reason: "interval", AllowNotify: true})
}

func runHermesCycleWithTrigger(ctx context.Context, cfg config.Config, db *storage.DB, trigger hermesagent.HermesTrigger) error {
	if err := ensureFreshHermesInputs(ctx, cfg, db, trigger); err != nil {
		log.Printf("[Hermes] freshness warning: %v", err)
	}
	snap := buildHermesSnapshotWithTrigger(cfg, trigger)
	if cfg.FreeAPI.Enabled {
		apiCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		report := freeapi.New(cfg).Run(apiCtx)
		cancel()
		if err := freeapi.Save(report, "reports"); err != nil {
			log.Printf("[Hermes] free API report save warning: %v", err)
		}
		snap.ResearchSummary += fmt.Sprintf(" | freeapi global_cap=%.0f btc_dom=%.2f fear_greed=%d/%s eurusd=%.5f funding=%.8f open_interest_usd=%.0f defi_tvl_usd=%.0f news=%d missing=%s", report.GlobalMarketCapUSD, report.BTCDominancePct, report.FearGreedValue, report.FearGreedLabel, report.EURUSD, report.FundingRate, report.OpenInterestUSD, report.DeFiTVLUSD, len(report.News), strings.Join(report.Missing, ","))
	}
	if plan, err := db.LatestPlan(); err == nil {
		enrichHermesAssetsFromPlan(cfg, db, &snap, plan)
	}
	// Recall similar situations before narrative generation. Memory is evidence only;
	// it cannot grant execution authority or override deterministic gates.
	if db != nil {
		mem, memErr := hermesmemory.Recall(db, hermesmemory.Situation{Regime: snap.BTCRegime, Phase: snap.BTCPhase, Permission: snap.BTCPermission, PlanState: snap.PlanState, DoctorStatus: snap.DoctorStatus, Trend: snap.BTCTrend, MMVerdict: snap.BTCMMVerdict, MMQuality: snap.BTCMMDataQuality, AuditAgeMinutes: snap.AuditAgeMinutes, ForcedSimulationPassed: snap.ForcedSimPassed, Authority: snap.MarketAuthority}, 5)
		if memErr != nil {
			log.Printf("[Hermes] memory recall warning: %v", memErr)
		} else {
			b, _ := json.Marshal(mem)
			snap.ResearchSummary += " | cognitive_memory=" + string(b)
		}
	}
	caller := hermesCallerFromConfig(cfg)
	narrativeStarted := time.Now()
	report, err := hermesagent.Generate(ctx, caller, snap)
	logHermesLLMCall("narrative", trigger, narrativeStarted, err)
	if err != nil {
		log.Printf("[Hermes] LLM warning: %v", err)
	}
	if trigger.UserText != "" && !strings.Contains(strings.ToLower(report.TelegramText), strings.ToLower(trigger.UserText)) {
		report.WorthyAlert = true
	}
	if err := saveJSONFile(hermesReportDir, "hermes_report_latest.json", report); err != nil {
		return fmt.Errorf("hermes report save: %w", err)
	}
	if db != nil {
		situation := hermesmemory.Situation{GeneratedAt: snap.GeneratedAt, Regime: snap.BTCRegime, Phase: snap.BTCPhase, Permission: snap.BTCPermission, PlanState: snap.PlanState, DoctorStatus: snap.DoctorStatus, Trend: snap.BTCTrend, MMVerdict: snap.BTCMMVerdict, MMQuality: snap.BTCMMDataQuality, AuditAgeMinutes: snap.AuditAgeMinutes, ForcedSimulationPassed: snap.ForcedSimPassed, Authority: snap.MarketAuthority}
		episode := hermesmemory.BuildEpisode(situation, report.GateSummary, []string{snap.AuditVerdict, snap.DoctorStatus, snap.MarketAuthority}, []string{report.AssetSummary, report.ExitSummary}, snap.DoctorBlockers)
		if err := hermesmemory.Save(db, episode); err != nil {
			log.Printf("[Hermes] memory save warning: %v", err)
		}
		_ = hermesmemory.SaveProvenance(db, hermesmemory.Provenance{ProvenanceID: "prov:" + episode.EpisodeID, EpisodeID: episode.EpisodeID, DerivationMethod: "deterministic_snapshot_plus_readonly_narrative", DerivationLayer: "derived", CreatedAt: episode.CreatedAt, MetadataJSON: `{"authority":"deterministic_engine_only","source":"live_auto_audit+plan+microstructure"}`})
		mem, memErr := hermesmemory.Recall(db, situation, 5)
		if memErr != nil {
			log.Printf("[Hermes] reasoning recall warning: %v", memErr)
		} else {
			for _, similar := range mem.Similar {
				relation := "supports"
				if !strings.EqualFold(similar.Episode.Situation.Permission, situation.Permission) {
					relation = "contradicts"
				}
				if similar.Episode.EpisodeID != episode.EpisodeID {
					_ = hermesmemory.SaveReasoningEdge(db, hermesmemory.ReasoningEdge{FromEpisodeID: similar.Episode.EpisodeID, ToEpisodeID: episode.EpisodeID, Relation: relation, Confidence: similar.Similarity, DecayWeight: 1, ValidFrom: episode.CreatedAt, Rationale: "deterministic situation similarity"})
				}
			}
			reasoning := hermesmemory.BuildReasoning(episode, mem, []string{"WAIT/NO_ACTION", "WATCH until deterministic gate improves"})
			if err := saveJSONFile(hermesReportDir, "hermes_reasoning_latest.json", reasoning); err != nil {
				log.Printf("[Hermes] reasoning report warning: %v", err)
			}
			// Predictions are research-only and neutral until a calibrated forecasting
			// model exists. They create measurable outcomes without inventing edge.
			for _, asset := range snap.Assets {
				candles, candleErr := db.LoadCandles(asset.Symbol, "1d", 1)
				if candleErr != nil || len(candles) == 0 || candles[0].Close <= 0 {
					continue
				}
				for _, horizon := range []string{"1d", "3d", "7d"} {
					pred, due, predErr := hermesmemory.NewPrediction(episode.EpisodeID, asset.Symbol, horizon, asset.State, candles[0].Close, 0, reasoning.Confidence, episode.CreatedAt)
					if predErr == nil {
						_ = hermesmemory.SavePrediction(db, pred, due)
					}
				}
			}
			if due, dueErr := hermesmemory.PendingPredictions(db, time.Now().UTC(), 100); dueErr == nil {
				for _, pred := range due {
					candles, candleErr := db.LoadCandles(pred.Symbol, "1d", 1)
					if candleErr != nil || len(candles) == 0 || pred.BasePrice <= 0 {
						continue
					}
					// Never score against a candle that predates the prediction horizon.
					if pred.DueAt.IsZero() || candles[0].CloseTime.Before(pred.DueAt) {
						continue
					}
					actual := candles[0].Close/pred.BasePrice - 1
					_ = hermesmemory.PersistScore(db, hermesmemory.ScorePrediction(pred, actual, time.Now().UTC()))
				}
			}
			if calibration, calErr := hermesmemory.LoadCalibration(db, 1000); calErr == nil {
				_ = saveJSONFile(hermesReportDir, "hermes_calibration_latest.json", calibration)
			}
			if backfillErr := hermesmemory.BackfillEpisodeProvenance(db); backfillErr != nil {
				log.Printf("[Hermes] provenance backfill warning: %v", backfillErr)
			}
			if brainAudit, auditErr := hermesmemory.AuditBrain(db, episode.EpisodeID); auditErr == nil {
				_ = saveJSONFile(hermesReportDir, "hermes_brain_audit_latest.json", brainAudit)
			} else {
				log.Printf("[Hermes] brain audit warning: %v", auditErr)
			}
			if hypothesisAudit, hypothesisErr := hermesmemory.AuditHypotheses(db); hypothesisErr == nil {
				_ = saveJSONFile(hermesReportDir, "hermes_hypothesis_audit_latest.json", hypothesisAudit)
			} else {
				log.Printf("[Hermes] hypothesis audit warning: %v", hypothesisErr)
			}
			if researchInvariantErr := hermesmemory.EnsureResearchInvariants(db); researchInvariantErr != nil {
				log.Printf("[Hermes] research invariant warning: %v", researchInvariantErr)
			}
			if researchAudit, researchErr := hermesmemory.AuditResearch(db); researchErr == nil {
				_ = saveJSONFile(hermesReportDir, "hermes_research_audit_latest.json", researchAudit)
			} else {
				log.Printf("[Hermes] research audit warning: %v", researchErr)
			}
			// Register a read-only daily dataset snapshot from canonical SQLite candles.
			// This creates research provenance only; it never reaches planning or execution.
			for _, symbol := range []string{"BTCUSDT", "ETHUSDT", "SOLUSDT"} {
				candles, candleErr := db.LoadCandles(symbol, "1d", 600)
				if candleErr != nil || len(candles) == 0 {
					continue
				}
				// Exclude the still-forming daily bar. An open bar changes every cycle
				// and would create false dataset versions and unstable research hashes.
				closed := candles[:0]
				now := time.Now().UTC()
				for _, candle := range candles {
					if !candle.CloseTime.IsZero() && candle.CloseTime.After(now) {
						continue
					}
					closed = append(closed, candle)
				}
				if len(closed) == 0 {
					continue
				}
				if dataset, datasetErr := hermesmemory.BuildDatasetFromCandles(symbol, "1d", "canonical_sqlite_candles_closed", closed, 1); datasetErr == nil {
					if saveErr := hermesmemory.SaveDataset(db, dataset); saveErr != nil {
						log.Printf("[Hermes] dataset snapshot warning: %v", saveErr)
					}
				}
			}
			// Re-run after persistence; otherwise this cycle's report describes the
			// pre-snapshot database state.
			if researchAudit, researchErr := hermesmemory.AuditResearch(db); researchErr == nil {
				_ = saveJSONFile(hermesReportDir, "hermes_research_audit_latest.json", researchAudit)
			} else {
				log.Printf("[Hermes] research audit post-snapshot warning: %v", researchErr)
			}
			if planErr := hermesmemory.EnsureBaselineResearchPlan(db, episode.EpisodeID); planErr != nil {
				log.Printf("[Hermes] baseline research plan warning: %v", planErr)
			}
			if researchAudit, researchErr := hermesmemory.AuditResearch(db); researchErr == nil {
				_ = saveJSONFile(hermesReportDir, "hermes_research_audit_latest.json", researchAudit)
			} else {
				log.Printf("[Hermes] research audit final warning: %v", researchErr)
			}
		}
	}
	if err := runHermesShadowDecision(ctx, cfg, snap, caller, trigger); err != nil {
		log.Printf("[Hermes] shadow decision warning: %v", err)
	}
	md := buildHermesMarkdown(snap, report, trigger)
	if err := os.MkdirAll(hermesReportDir, 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(hermesReportDir, "hermes_report_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	fmt.Println(md)
	state := loadHermesState()
	shouldSend, fp := hermesShouldNotify(state, snap, report, trigger)
	if shouldSend {
		if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
			sendScheduledTelegram(ctx, cfg, "hermes-cycle", report.TelegramText)
			state.LastSentFingerprint = fp
			state.LastSentAt = time.Now().UTC()
			state.LastAuditVerdict = snap.AuditVerdict
			state.LastDoctorStatus = snap.DoctorStatus
			state.LastExitFingerprint = exitFingerprint(snap)
			_ = saveHermesState(state)
		}
	}
	return nil
}

// runHermesDecisionCycle refreshes only the autonomous execution decision.
// Narrative generation is deliberately excluded from the 15-minute execution path.
func runHermesDecisionCycle(ctx context.Context, cfg config.Config, db *storage.DB, trigger hermesagent.HermesTrigger) error {
	if err := ensureFreshHermesInputs(ctx, cfg, db, trigger); err != nil {
		log.Printf("[Hermes] decision freshness warning: %v", err)
	}
	snap := buildHermesSnapshotWithTrigger(cfg, trigger)
	if plan, err := db.LatestPlan(); err == nil {
		enrichHermesAssetsFromPlan(cfg, db, &snap, plan)
	}
	return runHermesShadowDecision(ctx, cfg, snap, hermesCallerFromConfig(cfg), trigger)
}

func logHermesLLMCall(purpose string, trigger hermesagent.HermesTrigger, started time.Time, err error) {
	status := "ok"
	if err != nil {
		status = "error"
	}
	log.Printf("[LLM_USAGE] purpose=%s trigger=%s/%s status=%s latency_ms=%d", purpose, trigger.Source, trigger.Reason, status, time.Since(started).Milliseconds())
}

func enrichHermesAssetsFromPlan(cfg config.Config, db *storage.DB, snap *hermesagent.HermesSnapshot, plan agent2.Plan) {
	bySymbol := map[string]agent2.AssetPlan{}
	performance := storage.HermesLossProtection{BySymbol: map[string]storage.HermesAssetPerformance{}}
	if db != nil {
		if p, err := db.HermesLossProtectionSnapshot(time.Unix(0, 0)); err == nil {
			performance = p
		}
	}
	for _, asset := range plan.Assets {
		bySymbol[strings.ToUpper(asset.Symbol)] = asset
	}
	for i := range snap.Assets {
		asset, ok := bySymbol[strings.ToUpper(snap.Assets[i].Symbol)]
		if !ok {
			continue
		}
		target := asset.RewardRiskDetail.Target
		if target <= 0 && len(asset.Layers) > 0 {
			target = asset.Layers[0].Target
		}
		snap.Assets[i].EntryZoneLow = asset.DiscountZone.Low
		snap.Assets[i].EntryZoneHigh = asset.DiscountZone.High
		snap.Assets[i].Invalidation = asset.Invalidation
		snap.Assets[i].Target = target
		snap.Assets[i].MMCase = string(asset.MMCase)
		snap.Assets[i].MMScore = asset.MMScore
		snap.Assets[i].MMMissing = asset.MMMissing
		snap.Assets[i].FlowBias = string(asset.AssetFlowBias)
		snap.Assets[i].FlowScore = asset.AssetFlowScore
		snap.Assets[i].LiquidityGrade = asset.LiquidityQuality.Grade
		snap.Assets[i].LiquidityScore = asset.LiquidityQuality.Score
		snap.Assets[i].LiquidityPass = asset.LiquidityQuality.Pass
		snap.Assets[i].RotationRank = asset.RotationRank
		snap.Assets[i].RotationScore = asset.RotationScore
		snap.Assets[i].NextTrigger = asset.NextTrigger
		probeEligible := asset.State == agent2.StateScout && asset.DiscountZone.Valid() && asset.Invalidation > 0 && asset.RewardRisk >= cfg.Risk.ExceptionalRRBypassFallingKnife
		if asset.LiquidityQuality.Enabled && !asset.LiquidityQuality.Pass {
			probeEligible = false
		}
		snap.Assets[i].ProbeEligible = probeEligible
		if probeEligible {
			snap.Assets[i].ProbePolicy = "PROBE_LIMIT only: soft MM/flow/relative-strength/rotation weakness reduces confidence and size; never OPEN/SCALE while falling knife is high"
		} else {
			snap.Assets[i].ProbePolicy = "HOLD until deterministic probe envelope is valid"
		}
		perf := performance.BySymbol[strings.ToUpper(asset.Symbol)]
		snap.Assets[i].QuantReasoning = hermesoperator.ComputeQuantReasoning(hermesoperator.QuantReasoningInput{Symbol: asset.Symbol, ProbeEligible: probeEligible, SetupScore: asset.SetupScore, MMScore: asset.MMScore, FlowScore: asset.AssetFlowScore, RotationScore: asset.RotationScore, LiquidityScore: asset.LiquidityQuality.Score, RewardRisk: asset.RewardRisk, EntryPrice: asset.RewardRiskDetail.Entry, Invalidation: asset.Invalidation, Target: target, MarketRegime: snap.BTCRegime, AccumulationPhase: snap.BTCPhase, DataQuality: snap.BTCMMDataQuality, HistoricalTrades: perf.ClosedFills, HistoricalWins: perf.WinningFills, HistoricalExpectancy: perf.Expectancy, TotalCapital: cfg.Portfolio.TotalCapital, MaxProbeCapitalPct: cfg.HermesOperator.MaxProbeNotionalPct, MaxProbeNotional: config.EffectiveHermesProbeNotional(cfg)})
	}
}

// hermesShadowDecision is the persisted validated decision audit. In autonomous mode its
// validated actions are re-evaluated by production safety immediately before execution.
type hermesShadowDecision struct {
	GeneratedAt time.Time                        `json:"generated_at"`
	Mode        string                           `json:"mode"`
	Validation  hermesoperator.ValidationResult  `json:"validation"`
	Safety      []liveguard.HermesActionDecision `json:"safety"`
}

func runHermesShadowDecision(ctx context.Context, cfg config.Config, snap hermesagent.HermesSnapshot, caller hermesagent.JSONCaller, trigger hermesagent.HermesTrigger) error {
	if caller == nil || !cfg.HermesOperator.Enabled {
		return nil
	}
	allowed := map[string]bool{}
	for _, symbol := range cfg.Data.Symbols.Assets {
		allowed[strings.ToUpper(symbol)] = true
	}
	policy := hermesoperator.ValidationPolicy{
		Now: time.Now().UTC(), MaxDecisionTTL: time.Duration(cfg.HermesOperator.DecisionTTLSeconds) * time.Second,
		MinConfidence: cfg.HermesOperator.MinConfidence, MaxActions: cfg.HermesOperator.MaxActionsPerCycle,
		MaxProbeNotionalUSDT:  config.EffectiveHermesProbeNotional(cfg),
		MaxActionNotionalUSDT: config.EffectiveHermesActionNotional(cfg),
		AllowedSymbols:        allowed,
	}
	operatorSnapshot := hermesoperator.Snapshot{
		GeneratedAt: snap.GeneratedAt.Format(time.RFC3339), Mode: cfg.HermesOperator.NormalizedMode(),
		Market: map[string]any{"phase": snap.BTCPhase, "permission": snap.BTCPermission, "regime": snap.BTCRegime, "trend": snap.BTCTrend, "mm_verdict": snap.BTCMMVerdict, "mm_confidence": snap.BTCMMConfidence, "mm_core_signals": snap.BTCMMCoreSignals, "mm_data_quality": snap.BTCMMDataQuality},
		Assets: snap.Assets, Positions: snap.Positions, Limits: map[string]any{"probe_pct": cfg.HermesOperator.MaxProbeNotionalPct, "action_pct": cfg.HermesOperator.MaxActionNotionalPct, "portfolio_pct": cfg.HermesOperator.MaxPortfolioExposurePct}, Safety: map[string]any{"audit": snap.AuditVerdict, "doctor": snap.DoctorStatus, "halted": snap.OperatorHalted},
	}
	assetAllowance := map[string]float64{}
	for _, symbol := range cfg.Data.Symbols.Assets {
		assetAllowance[strings.ToUpper(symbol)] = config.EffectiveLiveNotionalPerAsset(cfg)
	}
	decisionStarted := time.Now()
	validation, err := hermesoperator.Generate(ctx, caller, operatorSnapshot, policy)
	logHermesLLMCall("operator_decision", trigger, decisionStarted, err)
	if err != nil {
		return err
	}
	// A malformed LLM exposure action must not become a silent HOLD when the
	// deterministic quant brain has an explicit probe recommendation. Rebuild
	// only quant-eligible PROBE_LIMIT actions, then run the same schema and
	// safety validation again. Clean HOLD/WATCH decisions are preserved.
	if len(validation.Reasons) > 0 && len(validation.Actions) == 0 {
		if fallback, ok := deterministicHermesProbeFallback(snap, validation.Decision, policy); ok {
			validation = hermesoperator.Validate(fallback, policy)
			if len(validation.Reasons) == 0 && len(validation.Actions) > 0 {
				validation.Decision.MarketThesis = strings.TrimSpace(validation.Decision.MarketThesis + " | deterministic quant probe fallback after malformed LLM exposure payload")
			}
		}
	}
	safety := liveguard.EvaluateHermesActions(validation.Actions, liveguard.HermesSafetyContext{
		OperatorHalted:             snap.OperatorHalted,
		DataHealthy:                snap.DoctorStatus == "DOCTOR_OK" || snap.DoctorStatus == "OK",
		ReconcileClean:             snap.AuditVerdict != "DOCTOR_BLOCK",
		OKXReady:                   snap.DoctorStatus == "DOCTOR_OK" || snap.DoctorStatus == "OK",
		PanicSelling:               strings.EqualFold(snap.BTCRegime, "PANIC_SELLING"),
		PortfolioNotionalRemaining: config.EffectiveHermesPortfolioExposure(cfg),
		AssetNotionalRemaining:     assetAllowance,
		Autonomous:                 cfg.HermesOperator.NormalizedMode() == "autonomous", TotalCapital: cfg.Portfolio.TotalCapital,
		AccumulationPhase: snap.BTCPhase, MarketRegime: snap.BTCRegime, TrendScore: snap.BTCTrend,
		MMConfidence: snap.BTCMMConfidence, DataQuality: snap.BTCMMDataQuality, PerOrderCap: config.EffectiveLiveNotionalPerOrder(cfg),
	})
	return saveJSONFile(hermesReportDir, "hermes_shadow_decision_latest.json", hermesShadowDecision{GeneratedAt: time.Now().UTC(), Mode: cfg.HermesOperator.NormalizedMode(), Validation: validation, Safety: safety})
}

func deterministicHermesProbeFallback(snap hermesagent.HermesSnapshot, decision hermesoperator.Decision, policy hermesoperator.ValidationPolicy) (hermesoperator.Decision, bool) {
	if len(decision.Actions) > 0 && len(decision.Actions) > policy.MaxActions && policy.MaxActions > 0 {
		return decision, false
	}
	for _, asset := range snap.Assets {
		var q struct {
			Eligible          bool    `json:"eligible"`
			Recommendation    string  `json:"recommendation"`
			Confidence        float64 `json:"confidence"`
			SuggestedNotional float64 `json:"suggested_notional_usdt"`
			Entry             float64 `json:"entry_price"`
			Invalidation      float64 `json:"invalidation"`
			Target            float64 `json:"target"`
		}
		b, _ := json.Marshal(asset.QuantReasoning)
		if json.Unmarshal(b, &q) != nil || !q.Eligible || q.Recommendation != string(hermesoperator.IntentProbeLimit) {
			continue
		}
		if q.Confidence < policy.MinConfidence || q.SuggestedNotional <= 0 || q.Entry <= 0 || q.Invalidation <= 0 || q.Target <= q.Entry {
			continue
		}
		notional := q.SuggestedNotional
		if policy.MaxProbeNotionalUSDT > 0 && notional > policy.MaxProbeNotionalUSDT {
			notional = policy.MaxProbeNotionalUSDT
		}
		if policy.MaxActionNotionalUSDT > 0 && notional > policy.MaxActionNotionalUSDT {
			notional = policy.MaxActionNotionalUSDT
		}
		if notional <= 0 {
			continue
		}
		decision.Actions = []hermesoperator.Action{{Symbol: asset.Symbol, Intent: hermesoperator.IntentProbeLimit, Confidence: q.Confidence, EntryPrice: q.Entry, Invalidation: q.Invalidation, Target: q.Target, RequestedNotionalUSDT: notional, MaxLayers: 1, ReasonCodes: []string{"QUANT_BRAIN_FALLBACK", "EXCEPTIONAL_RR", "PROBE_ONLY"}}}
		return decision, true
	}
	return decision, false
}

func hermesCallerFromConfig(cfg config.Config) hermesagent.JSONCaller {
	if !cfg.AI.Enabled {
		return nil
	}
	client, err := llm.NewFromEnv(cfg.AI.BaseURLEnv, cfg.AI.APIKeyEnv, cfg.AI.Model, cfg.AI.MaxTokens, cfg.AI.Temperature)
	if err != nil {
		log.Printf("[Hermes] LLM client warning: %v", err)
		return nil
	}
	return client
}

func ensureFreshHermesInputs(ctx context.Context, cfg config.Config, db *storage.DB, trigger hermesagent.HermesTrigger) error {
	maxAge := cfg.AI.HermesFreshAuditMaxMinutes
	if maxAge <= 0 {
		maxAge = 30
	}
	stale, err := hermesAuditStale(maxAge)
	if err != nil {
		return err
	}
	if !stale {
		return nil
	}
	if trigger.Source != "telegram" && trigger.Source != "scheduled" && trigger.Source != "audit" {
		return nil
	}
	if err := runLiveAutoAudit(ctx, cfg, db); err != nil {
		return fmt.Errorf("refresh live-auto-audit: %w", err)
	}
	return nil
}

func hermesAuditStale(maxAgeMinutes int) (bool, error) {
	b, err := os.ReadFile(filepath.Join(hermesReportDir, "live_auto_audit_latest.json"))
	if err != nil {
		return true, nil
	}
	var report struct {
		GeneratedAt time.Time `json:"generated_at"`
	}
	if err := json.Unmarshal(b, &report); err != nil {
		return true, err
	}
	if report.GeneratedAt.IsZero() {
		return true, nil
	}
	age := time.Since(report.GeneratedAt)
	return age > time.Duration(maxAgeMinutes)*time.Minute, nil
}

func buildHermesSnapshot(cfg config.Config) hermesagent.HermesSnapshot {
	return buildHermesSnapshotWithTrigger(cfg, hermesagent.HermesTrigger{})
}

func buildHermesSnapshotWithTrigger(cfg config.Config, trigger hermesagent.HermesTrigger) hermesagent.HermesSnapshot {
	snap := hermesagent.HermesSnapshot{
		GeneratedAt:   time.Now().UTC(),
		TriggerSource: trigger.Source,
		TriggerReason: trigger.Reason,
		UserQuestion:  trigger.UserText,
	}

	if b, err := os.ReadFile(filepath.Join(hermesReportDir, "live_auto_audit_latest.json")); err == nil {
		var audit struct {
			GeneratedAt           time.Time `json:"generated_at"`
			Verdict               string    `json:"verdict"`
			CurrentMarketAuth     string    `json:"current_market_authority"`
			CurrentDryRunApproved bool      `json:"current_dry_run_approved"`
			Reasons               []string  `json:"reasons"`
			ForcedSimulation      struct {
				Passed bool `json:"passed"`
			} `json:"forced_simulation"`
			Doctor struct {
				Status   string   `json:"status"`
				Blockers []string `json:"blockers"`
			} `json:"doctor"`
			Analysis struct {
				ActionPermission string  `json:"action_permission"`
				MarketRegime     string  `json:"market_regime"`
				TrendScore       float64 `json:"trend_score"`
				BTCAccumulation  struct {
					Phase string `json:"phase"`
				} `json:"btc_accumulation"`
			} `json:"analysis"`
			Plan struct {
				State string `json:"state"`
			} `json:"plan"`
		}
		if json.Unmarshal(b, &audit) == nil {
			snap.AuditVerdict = audit.Verdict
			snap.MarketAuthority = audit.CurrentMarketAuth
			snap.CurrentDryRunApproved = audit.CurrentDryRunApproved
			snap.ForcedSimPassed = audit.ForcedSimulation.Passed
			snap.AuditReasons = audit.Reasons
			snap.BTCPermission = audit.Analysis.ActionPermission
			snap.BTCRegime = audit.Analysis.MarketRegime
			snap.BTCTrend = audit.Analysis.TrendScore
			snap.BTCPhase = audit.Analysis.BTCAccumulation.Phase
			snap.PlanState = audit.Plan.State
			snap.DoctorStatus = audit.Doctor.Status
			snap.DoctorBlockers = audit.Doctor.Blockers
			if !audit.GeneratedAt.IsZero() {
				snap.AuditAgeMinutes = int(math.Round(time.Since(audit.GeneratedAt).Minutes()))
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(hermesReportDir, "microstructure_latest.json")); err == nil {
		var ms microstructure.Summary
		if json.Unmarshal(b, &ms) == nil {
			btc := strings.ToUpper(strings.TrimSpace(cfg.Data.Symbols.BTC))
			if btc == "" {
				btc = "BTCUSDT"
			}
			if fp, ok := ms.MMFootprint[btc]; ok {
				snap.BTCMMVerdict = fp.Verdict
				snap.BTCMMConfidence = fp.FootprintScore
				snap.BTCMMCoreSignals = fp.CoreSignalCount
				snap.BTCMMDataQuality = fp.DataQuality
			}
		}
	}

	if b, err := os.ReadFile(filepath.Join(hermesReportDir, "live_supervisor_latest.json")); err == nil {
		var sup liveguard.SupervisorResult
		if json.Unmarshal(b, &sup) == nil {
			snap.OperatorHalted = sup.AutoHalted
			snap.ExitEnabled = len(sup.Exits) > 0
			snap.LastSupervisorAt = sup.GeneratedAt.Format(time.RFC3339)
			for _, ex := range sup.Exits {
				snap.Exits = append(snap.Exits, hermesagent.HermesExit{Symbol: ex.Symbol, Action: string(ex.Action), PnLPct: ex.PnLPct, Reason: ex.Reason})
			}
		}
	}

	if scenario, ok := loadScenarioReportFile(); ok {
		for _, coin := range scenario.Coins {
			why := ""
			if len(coin.WhyNoOrder) > 0 {
				why = strings.Join(coin.WhyNoOrder, "; ")
				if len(why) > 120 {
					why = why[:120] + "..."
				}
			}
			snap.Assets = append(snap.Assets, hermesagent.HermesAsset{
				Symbol:     coin.Symbol,
				State:      string(coin.State),
				Readiness:  coin.ReadinessScore * 100,
				RR:         coin.RewardRisk,
				OpenOrders: coin.OpenOrders,
				Why:        why,
			})
		}
	}

	if posReport, ok := loadLivePositionReportFile(); ok {
		for _, pos := range posReport.Positions {
			snap.Positions = append(snap.Positions, hermesagent.HermesPosition{Symbol: pos.Symbol, Quantity: pos.Quantity, AvgEntryPrice: pos.AvgEntryPrice, OpenedAt: pos.OpenedAt})
		}
	}

	if b, err := os.ReadFile(filepath.Join(hermesReportDir, "research_brief_latest.json")); err == nil {
		var brief struct {
			Summary string `json:"summary"`
		}
		if json.Unmarshal(b, &brief) == nil && brief.Summary != "" {
			snap.ResearchSummary = brief.Summary
			if len(snap.ResearchSummary) > 300 {
				snap.ResearchSummary = snap.ResearchSummary[:300] + "..."
			}
		}
	}

	if b, err := os.ReadFile(filepath.Join(hermesReportDir, "scheduler_heartbeat.json")); err == nil {
		var hb struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(b, &hb) == nil {
			snap.SchedulerRunning = hb.Status == "running"
		}
	}
	return snap
}

func hermesShouldNotify(state hermesagent.HermesState, snap hermesagent.HermesSnapshot, report hermesagent.HermesReport, trigger hermesagent.HermesTrigger) (bool, string) {
	if trigger.ForceReply {
		return true, fingerprintForHermes(snap, report, trigger)
	}
	fp := fingerprintForHermes(snap, report, trigger)
	if state.LastSentFingerprint == fp {
		if time.Since(state.LastSentAt) < 30*time.Minute {
			return false, fp
		}
	}
	if state.LastAuditVerdict != snap.AuditVerdict || state.LastDoctorStatus != snap.DoctorStatus || state.LastExitFingerprint != exitFingerprint(snap) {
		return true, fp
	}
	if report.WorthyAlert {
		return true, fp
	}
	return false, fp
}

func fingerprintForHermes(snap hermesagent.HermesSnapshot, report hermesagent.HermesReport, trigger hermesagent.HermesTrigger) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%v|%s|%s", trigger.Source, trigger.Reason, snap.AuditVerdict, snap.DoctorStatus, snap.BTCPhase, snap.ExitEnabled, exitFingerprint(snap), report.ExitSummary)
}

func exitFingerprint(snap hermesagent.HermesSnapshot) string {
	parts := make([]string, 0, len(snap.Exits))
	for _, ex := range snap.Exits {
		parts = append(parts, fmt.Sprintf("%s:%s:%.2f", ex.Symbol, ex.Action, ex.PnLPct))
	}
	return strings.Join(parts, "|")
}

func loadHermesReportFile() (hermesagent.HermesReport, bool) {
	b, err := os.ReadFile(filepath.Join(hermesReportDir, "hermes_report_latest.json"))
	if err != nil {
		return hermesagent.HermesReport{}, false
	}
	var out hermesagent.HermesReport
	if err := json.Unmarshal(b, &out); err != nil {
		return hermesagent.HermesReport{}, false
	}
	return out, true
}

func loadHermesState() hermesagent.HermesState {
	var state hermesagent.HermesState
	b, err := os.ReadFile(filepath.Join(hermesReportDir, hermesStateFile))
	if err == nil {
		_ = json.Unmarshal(b, &state)
	}
	return state
}

func saveHermesState(state hermesagent.HermesState) error {
	if err := os.MkdirAll(hermesReportDir, 0700); err != nil {
		return err
	}
	return reportio.WriteJSON(hermesReportDir, hermesStateFile, state)
}

func buildHermesMarkdown(snap hermesagent.HermesSnapshot, report hermesagent.HermesReport, trigger hermesagent.HermesTrigger) string {
	var b strings.Builder
	b.WriteString("HERMES BOT MANAGER\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n", report.GeneratedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("Trigger: %s/%s\n", trigger.Source, trigger.Reason))
	if trigger.UserText != "" {
		b.WriteString(fmt.Sprintf("User question: %s\n", trigger.UserText))
	}
	b.WriteString(fmt.Sprintf("Audit age: %d min\n\n", snap.AuditAgeMinutes))
	b.WriteString("📊 STRATEGY\n")
	b.WriteString(fmt.Sprintf("BTC Phase: %s | Permission: %s | Regime: %s | Trend: %.1f\n", snap.BTCPhase, snap.BTCPermission, snap.BTCRegime, snap.BTCTrend))
	b.WriteString(fmt.Sprintf("Audit: %s | Market authority: %s\n", snap.AuditVerdict, snap.MarketAuthority))
	b.WriteString(fmt.Sprintf("Doctor: %s\n", snap.DoctorStatus))
	if len(snap.DoctorBlockers) > 0 {
		b.WriteString(fmt.Sprintf("Blockers: %s\n", strings.Join(snap.DoctorBlockers, "; ")))
	}
	b.WriteString("\n")
	if len(snap.Assets) > 0 {
		b.WriteString("📈 ASSETS\n")
		for _, a := range snap.Assets {
			b.WriteString(fmt.Sprintf("- %s: %s readiness=%.0f%% RR=%.2f orders=%d\n", a.Symbol, a.State, a.Readiness, a.RR, a.OpenOrders))
		}
		b.WriteString("\n")
	}
	if len(snap.Exits) > 0 {
		b.WriteString("📉 EXIT SIGNALS\n")
		for _, ex := range snap.Exits {
			b.WriteString(fmt.Sprintf("- %s → %s PnL=%.2f%%: %s\n", ex.Symbol, ex.Action, ex.PnLPct*100, ex.Reason))
		}
		b.WriteString("Autonomous exit authority active for validated Hermes-owned positions.\n\n")
	} else {
		b.WriteString("📉 EXIT SIGNALS: NONE\n\n")
	}
	if len(snap.Positions) > 0 {
		b.WriteString("💼 POSITIONS\n")
		for _, p := range snap.Positions {
			b.WriteString(fmt.Sprintf("- %s qty=%.6f avg=%.4f\n", p.Symbol, p.Quantity, p.AvgEntryPrice))
		}
		b.WriteString("\n")
	}
	b.WriteString("🤖 HERMES ANALYSIS\n")
	b.WriteString(fmt.Sprintf("Gate: %s\n", report.GateSummary))
	b.WriteString(fmt.Sprintf("Assets: %s\n", report.AssetSummary))
	b.WriteString(fmt.Sprintf("Exits: %s\n", report.ExitSummary))
	if len(report.Anomalies) > 0 {
		b.WriteString(fmt.Sprintf("⚠ Anomalies: %s\n", strings.Join(report.Anomalies, "; ")))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("✅ %s\n", report.ActionLine))
	return b.String()
}
