package main

import (
	"btc-agent/internal/config"
	"btc-agent/internal/opsplan"
	"btc-agent/internal/storage"
	"context"
	"fmt"
	"strconv"
	"time"
)

func run(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return usage()
	}
	cmd := args[1]
	cfgPath := "config.yaml"
	for i := 2; i < len(args)-1; i++ {
		if args[i] == "--config" {
			cfgPath = args[i+1]
		}
	}
	if cmd == "eval-ai" {
		return runAIEvaluation()
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	db, err := storage.Open(cfg.Storage.Path)
	if err != nil {
		return err
	}
	defer db.Close()

	switch cmd {
	case "fetch":
		return fetch(ctx, cfg, db)
	case "analyze":
		_, err := analyze(ctx, cfg, db)
		return err
	case "plan":
		_, err := plan(ctx, cfg, db)
		return err
	case "paper-manager":
		return runPaperManager(cfg, db)
	case "operations-plan":
		return runOperationsPlan(cfg, db)
	case "market-watch":
		report, err := runMarketWatch(ctx, cfg, db, true)
		if err == nil {
			fmt.Println(opsplan.Markdown(report))
		}
		return err
	case "ops-events":
		return runOpsEvents(cfg, db)
	case "microstructure-fetch":
		_, err := runMicrostructureFetch(ctx, cfg, db)
		return err
	case "accumulation-readiness":
		return runAccumulationReadiness(ctx, cfg, db)
	case "btc-gate-diagnostic":
		return runBTCGateDiagnostic(ctx, cfg, db)
	case "run-daily":
		return runDaily(ctx, cfg, db)
	case "control-plane-snapshot":
		return runControlPlaneSnapshot(cfg, db)
	case "control-plane-validate-proposal":
		return runControlPlaneValidateProposal(cfg, args)
	case "control-plane-submit-proposal":
		return runControlPlaneSubmitProposal(cfg, db, args)
	case "control-plane-proposal-result":
		return runControlPlaneProposalResult(db, args)
	case "control-plane-recent-proposals":
		return runControlPlaneRecentProposals(db)
	case "control-plane-request-halt":
		return runControlPlaneRequestHalt(db, args)
	case "status":
		status, err := formatStatus(cfg, db)
		if err != nil {
			return err
		}
		fmt.Println(status)
		return nil
	case "backtest":
		return runBacktest(cfg, db)
	case "backtest-live-manager":
		researchExpiryDays, err := intArgValue(args, "--research-expiry-days")
		if err != nil {
			return err
		}
		researchHoldPriceAboveDiscountPct, err := floatArgValue(args, "--research-hold-if-price-above-discount-pct")
		if err != nil {
			return err
		}
		return runBacktestLiveManager(cfg, db, hasFlag(args, "--research-armed"), argValue(args, "--research-profile"), researchExpiryDays, hasFlag(args, "--research-hold-through-watch"), researchHoldPriceAboveDiscountPct, hasFlag(args, "--production-armed-probe"))
	case "learn":
		return runLearning(cfg, db)
	case "real-data-survey":
		return runRealDataSurvey(cfg, db)
	case "universe-research":
		return runUniverseResearch(ctx, cfg, db)
	case "export-training":
		return runExportTraining(cfg, db)
	case "run-ai-watch":
		return runAIWatch(ctx, cfg, db)
	case "hermes-cycle":
		return runHermesCycle(ctx, cfg, db)
	case "live-proof":
		return runLiveProof(ctx, cfg, db)
	case "live-readiness":
		return runLiveReadiness(ctx, cfg, db)
	case "hermes-canary-readiness":
		return runCanaryReadiness(cfg, db)
	case "live-auto-audit":
		return runLiveAutoAudit(ctx, cfg, db)
	case "live-doctor":
		_, err := runLiveDoctor(ctx, cfg, db)
		return err
	case "research-doctor":
		_, err := runResearchDoctor(ctx, cfg)
		return err
	case "research-brief":
		_, err := runResearchBrief(ctx, cfg, true)
		return err
	case "research-expert":
		return runExpertResearch(ctx, cfg, db, hasFlag(args, "--dry-run"), !hasFlag(args, "--dry-run"))
	case "execute-live-proof-order":
		return runExecuteLiveProofOrder(ctx, cfg, db, argValue(args, "--confirm"))
	case "auto-live-order":
		return runAutoLiveOrder(ctx, cfg, db, hasFlag(args, "--dry-run"))
	case "cancel-all-live-orders":
		return runCancelAllLiveOrders(ctx, cfg, db, hasFlag(args, "--dry-run"))
	case "simulate-live-manager":
		return runSimulateLiveManager(cfg)
	case "operator-halt":
		return runOperatorHalt(ctx, cfg, db)
	case "operator-resume":
		return runOperatorResume(ctx, cfg, db)
	case "operator-status":
		return runOperatorStatus(db)
	case "reconcile-live-orders":
		return runReconcileLiveOrders(ctx, cfg, db)
	case "live-positions":
		return runLivePositions(cfg, db)
	case "telegram-commands":
		return runTelegramCommands(ctx, cfg, db)
	case "scheduler-heartbeat-check":
		minutes, err := intArgValue(args, "--max-age-minutes")
		if err != nil {
			return err
		}
		return runSchedulerHeartbeatCheck(ctx, cfg, time.Duration(minutes)*time.Minute)
	case "live-supervisor":
		_, err := runLiveSupervisorCycle(ctx, cfg, db, &liveSupervisorState{}, hasFlag(args, "--dry-run"))
		return err
	case "maintenance":
		return runMaintenance(cfg, db)
	case "scheduler":
		return runScheduler(ctx, cfg, db, hasFlag(args, "--run-now"), hasFlag(args, "--dry-run"))
	default:
		return usage()
	}
}

func usage() error {
	return fmt.Errorf("usage: btc-agent <fetch|analyze|plan|paper-manager|operations-plan|market-watch|ops-events|microstructure-fetch|accumulation-readiness|btc-gate-diagnostic|run-daily|run-ai-watch|backtest|backtest-live-manager|learn|real-data-survey|universe-research|export-training|eval-ai|live-proof|live-readiness|hermes-canary-readiness|live-auto-audit|live-doctor|research-doctor|research-brief|research-expert|execute-live-proof-order|auto-live-order|live-supervisor|cancel-all-live-orders|simulate-live-manager|operator-halt|operator-resume|operator-status|reconcile-live-orders|live-positions|telegram-commands|scheduler-heartbeat-check|maintenance|control-plane-snapshot|control-plane-validate-proposal|control-plane-submit-proposal|control-plane-proposal-result|control-plane-recent-proposals|control-plane-request-halt|status|scheduler> --config config.yaml [--run-now|--dry-run|--max-age-minutes <minutes>|--research-armed|--production-armed-probe|--research-profile <name>|--research-expiry-days <days>|--research-hold-through-watch|--research-hold-if-price-above-discount-pct <pct>|--proposal-file <path>|--decision-id <id>|--caller <name>|--reason-code <code>|--summary <text>]")
}

func argValue(args []string, key string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}

func intArgValue(args []string, key string) (int, error) {
	value := argValue(args, key)
	if value == "" {
		return 0, nil
	}
	out, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	if out < 0 {
		return 0, fmt.Errorf("%s cannot be negative", key)
	}
	return out, nil
}

func floatArgValue(args []string, key string) (float64, error) {
	value := argValue(args, key)
	if value == "" {
		return 0, nil
	}
	out, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	if out < 0 {
		return 0, fmt.Errorf("%s cannot be negative", key)
	}
	return out, nil
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}
