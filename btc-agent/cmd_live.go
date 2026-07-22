package main

import (
	"btc-agent/internal/agent2"
	"btc-agent/internal/config"
	"btc-agent/internal/exchange/live"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/market"
	"btc-agent/internal/storage"
	"btc-agent/internal/telegramreport"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func requireAutoLiveRuntime(cfg config.Config) error {
	if os.Getenv("BTC_AGENT_ALLOW_AUTO_LIVE") != "true" {
		return fmt.Errorf("BTC_AGENT_ALLOW_AUTO_LIVE=true required for auto live execution")
	}
	if !cfg.Live.LiveAutoMode {
		return fmt.Errorf("live.live_auto_mode=false")
	}
	if !cfg.Live.Enabled {
		return fmt.Errorf("live.enabled=false")
	}
	if !cfg.Live.AutoExecute {
		return fmt.Errorf("live.auto_execute=false")
	}
	if !cfg.Live.SupervisorEnabled {
		return fmt.Errorf("live.supervisor_enabled=false")
	}
	if !cfg.Live.OrderManagementEnabled {
		return fmt.Errorf("live.order_management_enabled=false")
	}
	if cfg.Live.RequireManualConfirm {
		return fmt.Errorf("live.require_manual_confirm=true")
	}
	if cfg.Live.ProofOnly {
		return fmt.Errorf("live.proof_only=true")
	}
	if !cfg.Execution.RealTradingEnabled {
		return fmt.Errorf("execution.real_trading_enabled=false")
	}
	return nil
}

func runLiveProof(ctx context.Context, cfg config.Config, db *storage.DB) error {
	p, err := db.LatestPlan()
	if err != nil {
		return fmt.Errorf("load latest plan: %w", err)
	}
	var balanceReader liveguard.BalanceReader
	var filterReader liveguard.FilterReader
	if cfg.Live.Enabled && strings.ToLower(cfg.Live.Exchange) == "okx" {
		client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
		if err == nil {
			balanceReader = client
			filterReader = client
		}
	}
	proof := liveguard.BuildProofWithChecks(ctx, cfg, p, balanceReader, filterReader)
	if err := saveJSONFile("reports", "live_proof_latest.json", proof); err != nil {
		return err
	}
	md := fmt.Sprintf("LIVE TRADING READINESS PROOF\n\nStatus: %s\nSummary: %s\nNo real order was placed.\n", proof.Status, proof.Summary)
	if proof.Account.Enabled {
		md += fmt.Sprintf("Account check: auth_ok=%v balance_ok=%v base=%s free_usdt=%.2f min_required=%.2f\n", proof.Account.AuthOK, proof.Account.BalanceOK, proof.Account.BaseCurrency, proof.Account.FreeUSDT, proof.Account.MinRequiredUSDT)
		if proof.Account.Error != "" {
			md += "Account error: " + proof.Account.Error + "\n"
		}
	}
	if proof.Preflight.Enabled {
		md += fmt.Sprintf("Preflight: enabled=%v pass=%v inst_id=%s price=%.8f qty=%.8f notional=%.2f tick=%.8f step=%.8f min_size=%.8f min_notional=%.2f\n", proof.Preflight.Enabled, proof.Preflight.Pass, proof.Preflight.InstID, proof.Preflight.Price, proof.Preflight.Quantity, proof.Preflight.Notional, proof.Preflight.TickSize, proof.Preflight.StepSize, proof.Preflight.MinSize, proof.Preflight.MinNotional)
		if len(proof.Preflight.Reasons) > 0 {
			md += "Preflight reasons: " + fmt.Sprint(proof.Preflight.Reasons) + "\n"
		}
	}
	if proof.Candidate.Symbol != "" {
		md += fmt.Sprintf("Candidate: %s %s limit %.8f qty %.8f notional %.2f post_only=%v\n", proof.Candidate.Side, proof.Candidate.Symbol, proof.Candidate.Price, proof.Candidate.Quantity, proof.Candidate.Notional, proof.Candidate.PostOnly)
	}
	if len(proof.Reasons) > 0 {
		md += "Reasons: " + fmt.Sprint(proof.Reasons) + "\n"
	}
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "live_proof_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(ctx, cfg, "live-proof", telegramreport.LiveProofHumanText(proof))
	}
	fmt.Println(md)
	return nil
}

type liveReadinessReport struct {
	GeneratedAt                    time.Time                       `json:"generated_at"`
	Mode                           string                          `json:"mode"`
	LiveEnabled                    bool                            `json:"live_enabled"`
	RealTradingEnabled             bool                            `json:"real_trading_enabled"`
	AutoExecute                    bool                            `json:"auto_execute"`
	LiveAutoMode                   bool                            `json:"live_auto_mode"`
	LiveAutoMaxNotional            float64                         `json:"live_auto_max_notional_usdt"`
	OrderManagementEnabled         bool                            `json:"order_management_enabled"`
	MaxAutoLayersPerAsset          int                             `json:"max_auto_layers_per_asset"`
	MaxOpenLiveOrdersPerAsset      int                             `json:"max_open_live_orders_per_asset"`
	MaxOpenLiveOrdersTotal         int                             `json:"max_open_live_orders_total"`
	MaxLiveNotionalPerOrderUSDT    float64                         `json:"max_live_notional_per_order_usdt"`
	MaxLiveNotionalPerAssetUSDT    float64                         `json:"max_live_notional_per_asset_usdt"`
	MaxLiveNotionalTotalUSDT       float64                         `json:"max_live_notional_total_usdt"`
	CancelIfPlanNotActive          bool                            `json:"cancel_if_plan_not_active"`
	CancelIfPriceAboveDiscountZone float64                         `json:"cancel_if_price_above_discount_zone_pct"`
	ReplaceIfPriceDriftPct         float64                         `json:"replace_if_price_drift_pct"`
	CancelStaleAfterMinutes        int                             `json:"cancel_stale_after_minutes"`
	RequireManualConfirm           bool                            `json:"require_manual_confirm"`
	ProofOnly                      bool                            `json:"proof_only"`
	AutoLiveEnv                    bool                            `json:"auto_live_env"`
	OperatorHalted                 bool                            `json:"operator_halted"`
	CredentialEnvPresent           map[string]bool                 `json:"credential_env_present"`
	PlanState                      agent2.State                    `json:"plan_state"`
	Proof                          liveguard.Proof                 `json:"proof"`
	OpenLiveOrders                 []live.OrderStatus              `json:"open_live_orders"`
	LivePositions                  []live.LivePosition             `json:"live_positions"`
	DataHealth                     liveguard.DataHealthResult      `json:"data_health"`
	DataSanity                     liveguard.DataSanityResult      `json:"data_sanity"`
	ReconcileSafety                liveguard.ReconcileSafetyResult `json:"reconcile_safety"`
	RiskGovernor                   liveguard.RiskGovernorResult    `json:"risk_governor"`
	ManagedCoinSummaries           []liveguard.ManagedCoinSummary  `json:"managed_coin_summaries,omitempty"`
	AutoLiveBlockers               []string                        `json:"auto_live_blockers"`
	Summary                        string                          `json:"summary"`
}

func runLiveReadiness(ctx context.Context, cfg config.Config, db *storage.DB) error {
	p, err := db.LatestPlan()
	if err != nil {
		return fmt.Errorf("load latest plan: %w", err)
	}
	halted, err := db.IsHalted()
	if err != nil {
		return fmt.Errorf("load operator halt: %w", err)
	}
	open, err := db.OpenLiveOrders()
	if err != nil {
		return fmt.Errorf("load open live orders: %w", err)
	}
	positions, err := db.LivePositions()
	if err != nil {
		return fmt.Errorf("load live positions: %w", err)
	}
	analysis, err := db.LatestAnalysis()
	if err != nil {
		return fmt.Errorf("load latest analysis: %w", err)
	}
	assets, err := loadAssets(cfg, db)
	if err != nil {
		return fmt.Errorf("load assets for data health: %w", err)
	}
	dataHealth := liveguard.CheckDataHealth(cfg, analysis, p, assets, open, positions, time.Now())
	dataSanity := liveguard.DataSanityResult{}
	reconcileSafety := liveguard.ReconcileSafety(liveguard.ReconcileResult{Checked: len(open), Orders: open})
	riskGovernor := liveguard.EvaluateRiskGovernor(cfg, analysis, p, open, positions, dataHealth, dataSanity, reconcileSafety)
	var balanceReader liveguard.BalanceReader
	var filterReader liveguard.FilterReader
	if cfg.Live.Enabled && strings.ToLower(cfg.Live.Exchange) == "okx" {
		client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
		if err == nil {
			balanceReader = client
			filterReader = client
		}
	}
	proof := liveguard.BuildProofWithChecks(ctx, cfg, p, balanceReader, filterReader)
	managedSummaries := liveguard.BuildManagedCoinSummaries(cfg, p, open, liveguard.ManagedCycleResult{PlanState: p.State, Desired: []liveguard.ManagedDesiredOrder{}, DataHealth: dataHealth, ReconcileSafety: reconcileSafety, RiskGovernor: riskGovernor})
	report := liveReadinessReport{
		GeneratedAt:                    time.Now(),
		Mode:                           os.Getenv("BTC_AGENT_MODE"),
		LiveEnabled:                    cfg.Live.Enabled,
		RealTradingEnabled:             cfg.Execution.RealTradingEnabled,
		AutoExecute:                    cfg.Live.AutoExecute,
		LiveAutoMode:                   config.LiveAutoMode(cfg),
		LiveAutoMaxNotional:            config.LiveAutoMaxNotionalUSDT(cfg),
		OrderManagementEnabled:         cfg.Live.OrderManagementEnabled,
		MaxAutoLayersPerAsset:          cfg.Live.MaxAutoLayersPerAsset,
		MaxOpenLiveOrdersPerAsset:      cfg.Live.MaxOpenLiveOrdersPerAsset,
		MaxOpenLiveOrdersTotal:         cfg.Live.MaxOpenLiveOrdersTotal,
		MaxLiveNotionalPerOrderUSDT:    cfg.Live.MaxLiveNotionalPerOrderUSDT,
		MaxLiveNotionalPerAssetUSDT:    cfg.Live.MaxLiveNotionalPerAssetUSDT,
		MaxLiveNotionalTotalUSDT:       cfg.Live.MaxLiveNotionalTotalUSDT,
		CancelIfPlanNotActive:          cfg.Live.CancelIfPlanNotActive,
		CancelIfPriceAboveDiscountZone: cfg.Live.CancelIfPriceAboveDiscountZonePct,
		ReplaceIfPriceDriftPct:         cfg.Live.ReplaceIfPriceDriftPct,
		CancelStaleAfterMinutes:        cfg.Live.CancelStaleAfterMinutes,
		RequireManualConfirm:           cfg.Live.RequireManualConfirm,
		ProofOnly:                      cfg.Live.ProofOnly,
		AutoLiveEnv:                    os.Getenv("BTC_AGENT_ALLOW_AUTO_LIVE") == "true",
		OperatorHalted:                 halted,
		CredentialEnvPresent:           liveCredentialEnvPresent(cfg),
		PlanState:                      p.State,
		Proof:                          proof,
		OpenLiveOrders:                 open,
		LivePositions:                  positions,
		DataHealth:                     dataHealth,
		ReconcileSafety:                reconcileSafety,
		RiskGovernor:                   riskGovernor,
		ManagedCoinSummaries:           managedSummaries,
	}
	if err := requireAutoLiveRuntime(cfg); err != nil {
		report.AutoLiveBlockers = append(report.AutoLiveBlockers, err.Error())
	}
	if halted {
		report.AutoLiveBlockers = append(report.AutoLiveBlockers, "operator halt active")
	}
	if len(open) > 0 && !cfg.Live.OrderManagementEnabled {
		report.AutoLiveBlockers = append(report.AutoLiveBlockers, "open live order exists")
	}
	if dataHealth.Status == liveguard.DataHealthBlock {
		report.AutoLiveBlockers = append(report.AutoLiveBlockers, "data health block")
	}
	if riskGovernor.Status == liveguard.RiskGovernorBlock {
		report.AutoLiveBlockers = append(report.AutoLiveBlockers, "risk governor block")
	}
	report.AutoLiveBlockers = uniqueStringsMain(report.AutoLiveBlockers)
	report.Summary = liveReadinessSummary(report)
	if err := saveJSONFile("reports", "live_readiness_latest.json", report); err != nil {
		return err
	}
	md := liveReadinessMarkdown(report)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "live_readiness_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(ctx, cfg, "live-readiness", telegramreport.LiveReadinessHumanText(liveReadinessTelegramView(report)))
	}
	fmt.Println(md)
	return nil
}

func liveCredentialEnvPresent(cfg config.Config) map[string]bool {
	out := map[string]bool{}
	for _, env := range []string{cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv} {
		if env == "" {
			continue
		}
		out[env] = os.Getenv(env) != ""
	}
	return out
}

func inspectRuntimeEnvFiles() []liveguard.EnvFileStatus {
	paths := []string{}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, "btc-agent.env"))
	}
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		paths = append(paths, filepath.Join(cwd, ".env"))
	}
	seen := map[string]bool{}
	out := []liveguard.EnvFileStatus{}
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		status := liveguard.EnvFileStatus{Path: path}
		b, err := os.ReadFile(path)
		if err != nil {
			out = append(out, status)
			continue
		}
		status.Exists = true
		values := parseEnvRuntimeValues(string(b))
		status.Mode = values["BTC_AGENT_MODE"]
		status.AutoLiveAllow = values["BTC_AGENT_ALLOW_AUTO_LIVE"]
		status.OKXKeyPresent = values["OKX_API_KEY"] != ""
		status.OKXSecretPresent = values["OKX_API_SECRET"] != ""
		status.OKXPassphrasePresent = values["OKX_API_PASSPHRASE"] != ""
		out = append(out, status)
	}
	return out
}

func parseEnvRuntimeValues(text string) map[string]string {
	out := map[string]string{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(parts[0]), "export "))
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		switch key {
		case "BTC_AGENT_MODE", "BTC_AGENT_ALLOW_AUTO_LIVE", "OKX_API_KEY", "OKX_API_SECRET", "OKX_API_PASSPHRASE":
			out[key] = value
		}
	}
	return out
}

func envFileConflictWarnings(files []liveguard.EnvFileStatus) []string {
	warnings := []string{}
	existing := []liveguard.EnvFileStatus{}
	for _, file := range files {
		if file.Exists {
			existing = append(existing, file)
		}
	}
	if len(existing) < 2 {
		return warnings
	}
	firstMode := existing[0].Mode
	firstAllow := existing[0].AutoLiveAllow
	for _, file := range existing[1:] {
		if file.Mode != firstMode {
			warnings = append(warnings, fmt.Sprintf("env file BTC_AGENT_MODE differs: %s=%s vs %s=%s", existing[0].Path, emptyDefault(firstMode, "unset"), file.Path, emptyDefault(file.Mode, "unset")))
		}
		if file.AutoLiveAllow != firstAllow {
			warnings = append(warnings, fmt.Sprintf("env file BTC_AGENT_ALLOW_AUTO_LIVE differs: %s=%s vs %s=%s", existing[0].Path, emptyDefault(firstAllow, "unset"), file.Path, emptyDefault(file.AutoLiveAllow, "unset")))
		}
	}
	for _, file := range existing {
		if strings.HasSuffix(file.Path, string(filepath.Separator)+"btc-agent.env") {
			if mode := os.Getenv("BTC_AGENT_MODE"); mode != "" && file.Mode != "" && mode != file.Mode {
				warnings = append(warnings, fmt.Sprintf("loaded BTC_AGENT_MODE=%s differs from %s=%s", mode, file.Path, file.Mode))
			}
			if allow := os.Getenv("BTC_AGENT_ALLOW_AUTO_LIVE"); allow != "" && file.AutoLiveAllow != "" && allow != file.AutoLiveAllow {
				warnings = append(warnings, fmt.Sprintf("loaded BTC_AGENT_ALLOW_AUTO_LIVE=%s differs from %s=%s", allow, file.Path, file.AutoLiveAllow))
			}
		}
	}
	return warnings
}

func runLiveDoctor(ctx context.Context, cfg config.Config, db *storage.DB) (liveguard.RuntimeDoctorResult, error) {
	result := buildLiveDoctorResult(ctx, cfg, db)
	if err := writeLiveDoctorResult(result); err != nil {
		return result, err
	}
	fmt.Println(liveDoctorMarkdown(result))
	return result, nil
}

func writeLiveDoctorResult(result liveguard.RuntimeDoctorResult) error {
	result.RefreshSummary()
	if err := saveJSONFile("reports", "live_doctor_latest.json", result); err != nil {
		return err
	}
	md := liveDoctorMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join("reports", "live_doctor_latest.md"), []byte(md), 0600)
}

func buildLiveDoctorResult(ctx context.Context, cfg config.Config, db *storage.DB) liveguard.RuntimeDoctorResult {
	envFiles := inspectRuntimeEnvFiles()
	result := liveguard.RuntimeDoctorResult{GeneratedAt: time.Now(), CredentialEnvPresent: liveCredentialEnvPresent(cfg), EnvFiles: envFiles, AutoLiveEnv: os.Getenv("BTC_AGENT_ALLOW_AUTO_LIVE") == "true", TelegramTokenPresent: firstNonEmpty(cfg.Notify.TelegramToken, os.Getenv("TELEGRAM_TOKEN")) != "", TelegramChatPresent: firstNonEmpty(cfg.Notify.TelegramChatID, os.Getenv("TELEGRAM_CHAT_ID")) != ""}
	result.Warnings = append(result.Warnings, envFileConflictWarnings(envFiles)...)
	if !result.AutoLiveEnv && cfg.Live.Enabled && cfg.Live.AutoExecute {
		result.Blockers = append(result.Blockers, "BTC_AGENT_ALLOW_AUTO_LIVE=true required for auto live execution")
	}
	missingCreds := []string{}
	for _, env := range []string{cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv} {
		if env != "" && !result.CredentialEnvPresent[env] {
			missingCreds = append(missingCreds, env)
		}
	}
	if cfg.Live.Enabled && len(missingCreds) > 0 {
		result.Blockers = append(result.Blockers, "missing OKX credential env: "+strings.Join(missingCreds, ", "))
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && (!result.TelegramTokenPresent || !result.TelegramChatPresent) {
		result.Warnings = append(result.Warnings, "telegram token/chat missing; notifications will be skipped")
	}
	if halted, err := db.IsHalted(); err != nil {
		result.Blockers = append(result.Blockers, "read operator halt: "+err.Error())
	} else {
		result.OperatorHalted = halted
		if halted {
			result.Blockers = append(result.Blockers, "operator halt active")
		}
	}
	open, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		result.Blockers = append(result.Blockers, "load open live orders: "+err.Error())
	} else {
		result.OpenLiveOrders = len(open)
	}
	plan, planErr := db.LatestPlan()
	if planErr != nil {
		result.Warnings = append(result.Warnings, "latest plan unavailable: "+planErr.Error())
	} else {
		result.PlanState = plan.State
	}
	analysis, analysisErr := db.LatestAnalysis()
	if analysisErr != nil {
		result.Warnings = append(result.Warnings, "latest analysis unavailable: "+analysisErr.Error())
	}
	positions, positionsErr := db.LivePositions()
	if positionsErr != nil {
		result.Blockers = append(result.Blockers, "load live positions: "+positionsErr.Error())
	}
	if planErr == nil && analysisErr == nil && err == nil && positionsErr == nil {
		assets, assetsErr := loadAssets(cfg, db)
		if assetsErr != nil {
			result.Warnings = append(result.Warnings, "load assets for data health: "+assetsErr.Error())
		} else {
			now := time.Now()
			result.DataHealth = liveguard.CheckDataHealth(cfg, analysis, plan, assets, open, positions, now)
			btc, btcErr := loadBTC(cfg, db)
			if btcErr != nil {
				result.Warnings = append(result.Warnings, "load BTC for data sanity: "+btcErr.Error())
			} else {
				result.DataSanity = liveguard.CheckDataSanity(cfg, btc, assets, analysis, now)
			}
			result.ReconcileSafety = liveguard.ReconcileSafety(liveguard.ReconcileResult{Checked: len(open), Orders: open})
			result.RiskGovernor = liveguard.EvaluateRiskGovernor(cfg, analysis, plan, open, positions, result.DataHealth, result.DataSanity, result.ReconcileSafety)
			if result.DataHealth.Status == liveguard.DataHealthBlock {
				result.Blockers = append(result.Blockers, result.DataHealth.Blockers...)
			}
			if result.DataSanity.Status == liveguard.DataSanityBlock {
				result.Blockers = append(result.Blockers, result.DataSanity.Blockers...)
			}
			if result.ReconcileSafety.Status == liveguard.ReconcileBlock {
				result.Blockers = append(result.Blockers, result.ReconcileSafety.Blockers...)
			}
			if result.RiskGovernor.Status == liveguard.RiskGovernorBlock {
				result.Blockers = append(result.Blockers, result.RiskGovernor.Blockers...)
			}
		}
	}
	if cfg.Live.Enabled && len(missingCreds) == 0 {
		client, clientErr := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
		if clientErr != nil {
			result.Blockers = append(result.Blockers, "create OKX client: "+clientErr.Error())
		} else {
			result.OKXClientReady = true
			if planErr == nil {
				proof := liveguard.BuildProofWithChecks(ctx, cfg, plan, client, client)
				result.OKXReadOnlyChecked = proof.Account.Enabled || proof.Preflight.Enabled
				result.ProofStatus = proof.Status
				result.AccountAuthOK = proof.Account.AuthOK
				result.AccountBalanceOK = proof.Account.BalanceOK
				result.PreflightPass = proof.Preflight.Pass
				if proof.Status == liveguard.NotReadyBalance || proof.Status == liveguard.NotReadyFilters || proof.Status == liveguard.NotReadyConfig {
					result.Blockers = append(result.Blockers, proof.Reasons...)
				}
			}
		}
	}
	result.RefreshSummary()
	return result
}

func liveDoctorMarkdown(result liveguard.RuntimeDoctorResult) string {
	md := fmt.Sprintf("LIVE DOCTOR\n\nGenerated: %s\nStatus: %s\nSummary: %s\n\n", result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Status, result.Summary)
	md += fmt.Sprintf("Auto live env: %v\n", result.AutoLiveEnv)
	md += "OKX credential env present:\n"
	for _, env := range []string{"OKX_API_KEY", "OKX_API_SECRET", "OKX_API_PASSPHRASE"} {
		if _, ok := result.CredentialEnvPresent[env]; ok {
			md += fmt.Sprintf("- %s: %v\n", env, result.CredentialEnvPresent[env])
		}
	}
	md += fmt.Sprintf("Telegram env/config present: token=%v chat=%v\n", result.TelegramTokenPresent, result.TelegramChatPresent)
	if len(result.EnvFiles) > 0 {
		md += "Env files:\n"
		for _, file := range result.EnvFiles {
			md += fmt.Sprintf("- %s: exists=%v mode=%s allow_auto_live=%s okx_key=%v okx_secret=%v okx_passphrase=%v\n", file.Path, file.Exists, emptyDefault(file.Mode, "unset"), emptyDefault(file.AutoLiveAllow, "unset"), file.OKXKeyPresent, file.OKXSecretPresent, file.OKXPassphrasePresent)
		}
	}
	md += fmt.Sprintf("Operator halt: %v\n", result.OperatorHalted)
	md += fmt.Sprintf("Open live orders: %d\n", result.OpenLiveOrders)
	md += fmt.Sprintf("Plan state: %s\n", result.PlanState)
	md += fmt.Sprintf("OKX client ready: %v | read-only checked: %v\n", result.OKXClientReady, result.OKXReadOnlyChecked)
	if result.ProofStatus != "" {
		md += fmt.Sprintf("Proof status: %s | auth=%v balance=%v preflight=%v\n", result.ProofStatus, result.AccountAuthOK, result.AccountBalanceOK, result.PreflightPass)
	}
	if result.DataHealth.Status != "" {
		md += fmt.Sprintf("Data health: %s | %s\n", result.DataHealth.Status, result.DataHealth.Summary)
	}
	if result.DataSanity.Status != "" {
		md += fmt.Sprintf("Data sanity: %s | %s\n", result.DataSanity.Status, result.DataSanity.Summary)
	}
	if result.ReconcileSafety.Status != "" {
		md += fmt.Sprintf("Reconcile safety: %s | %s\n", result.ReconcileSafety.Status, result.ReconcileSafety.Summary)
	}
	if result.RiskGovernor.Status != "" {
		md += fmt.Sprintf("Risk governor: %s | %s\n", result.RiskGovernor.Status, result.RiskGovernor.Summary)
	}
	if len(result.Blockers) > 0 {
		md += "\nBlockers:\n"
		for _, blocker := range result.Blockers {
			md += "- " + blocker + "\n"
		}
	}
	if len(result.Warnings) > 0 {
		md += "\nWarnings:\n"
		for _, warning := range result.Warnings {
			md += "- " + warning + "\n"
		}
	}
	md += "\nSafety: spot limit BUY post-only only; no futures, no leverage, no market order.\n"
	return md
}

func liveReadinessTelegramView(r liveReadinessReport) telegramreport.LiveReadinessView {
	return telegramreport.LiveReadinessView{
		GeneratedAt:                   r.GeneratedAt,
		Mode:                          r.Mode,
		AutoLiveEnv:                   r.AutoLiveEnv,
		OperatorHalted:                r.OperatorHalted,
		CredentialEnvPresent:          r.CredentialEnvPresent,
		PlanState:                     r.PlanState,
		Proof:                         r.Proof,
		OpenLiveOrders:                len(r.OpenLiveOrders),
		LivePositions:                 len(r.LivePositions),
		DataHealth:                    r.DataHealth,
		ReconcileSafety:               r.ReconcileSafety,
		RiskGovernor:                  r.RiskGovernor,
		AutoLiveBlockers:              r.AutoLiveBlockers,
		LiveEnabled:                   r.LiveEnabled,
		RealTradingEnabled:            r.RealTradingEnabled,
		AutoExecute:                   r.AutoExecute,
		LiveAutoMode:                  r.LiveAutoMode,
		LiveAutoMaxNotional:           r.LiveAutoMaxNotional,
		RequireManualConfirm:          r.RequireManualConfirm,
		ProofOnly:                     r.ProofOnly,
		OrderManagementEnabled:        r.OrderManagementEnabled,
		MaxAutoLayersPerAsset:         r.MaxAutoLayersPerAsset,
		MaxOpenLiveOrdersPerAsset:     r.MaxOpenLiveOrdersPerAsset,
		MaxOpenLiveOrdersTotal:        r.MaxOpenLiveOrdersTotal,
		MaxLiveNotionalPerOrderUSDT:   r.MaxLiveNotionalPerOrderUSDT,
		MaxLiveNotionalPerAssetUSDT:   r.MaxLiveNotionalPerAssetUSDT,
		MaxLiveNotionalTotalUSDT:      r.MaxLiveNotionalTotalUSDT,
		CancelIfPlanNotActive:         r.CancelIfPlanNotActive,
		CancelIfPriceAboveDiscountPct: r.CancelIfPriceAboveDiscountZone,
		ReplaceIfPriceDriftPct:        r.ReplaceIfPriceDriftPct,
		CancelStaleAfterMinutes:       r.CancelStaleAfterMinutes,
		ManagedCoinSummaries:          r.ManagedCoinSummaries,
	}
}

func liveReadinessSummary(r liveReadinessReport) string {
	if len(r.AutoLiveBlockers) == 0 && r.Proof.Status == liveguard.ReadyForManualLiveProofOrder {
		return "LIVE_READY_FOR_AUTO"
	}
	if r.Proof.Status == liveguard.ReadyForManualLiveProofOrder {
		return fmt.Sprintf("LIVE_READY_FOR_MANUAL; auto_blockers=%d", len(r.AutoLiveBlockers))
	}
	return fmt.Sprintf("LIVE_NOT_READY; proof=%s auto_blockers=%d", r.Proof.Status, len(r.AutoLiveBlockers))
}

func liveReadinessMarkdown(r liveReadinessReport) string {
	md := fmt.Sprintf("LIVE READINESS REPORT\n\nGenerated: %s\nSummary: %s\n\n", r.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), r.Summary)
	md += fmt.Sprintf("Mode: %s | auto env: %v\n", emptyDefault(r.Mode, "unset"), r.AutoLiveEnv)
	md += fmt.Sprintf("Config: live=%v real=%v auto_execute=%v manual_confirm=%v proof_only=%v\n", r.LiveEnabled, r.RealTradingEnabled, r.AutoExecute, r.RequireManualConfirm, r.ProofOnly)
	md += fmt.Sprintf("Optional live-auto cap: enabled=%v max=%.2f USDT\n", r.LiveAutoMode, r.LiveAutoMaxNotional)
	md += fmt.Sprintf("Managed order engine: enabled=%v max_layers_per_asset=%d max_open_per_asset=%d max_open_total=%d\n", r.OrderManagementEnabled, r.MaxAutoLayersPerAsset, r.MaxOpenLiveOrdersPerAsset, r.MaxOpenLiveOrdersTotal)
	md += fmt.Sprintf("Managed notional caps: per_order=%.2f per_asset=%.2f total=%.2f USDT\n", r.MaxLiveNotionalPerOrderUSDT, r.MaxLiveNotionalPerAssetUSDT, r.MaxLiveNotionalTotalUSDT)
	md += fmt.Sprintf("Managed cancel/replace: cancel_plan_inactive=%v cancel_price_above_discount=%.2f%% replace_drift=%.2f%% stale_after=%dm\n", r.CancelIfPlanNotActive, r.CancelIfPriceAboveDiscountZone*100, r.ReplaceIfPriceDriftPct*100, r.CancelStaleAfterMinutes)
	if r.LiveAutoMode && r.OrderManagementEnabled {
		md += "Risk sizing: BTC permission controls budget multiplier; hard safety still blocks dangerous actions.\n"
		md += "Opportunity allocation: live capital uses OpportunityComposite plus history quality inside ACTIVE_LIMIT guard.\n"
		md += "Quality multiplier: A/B full, C reduced, NO_SAMPLE/missing probe, D blocked.\n"
	}
	md += fmt.Sprintf("Operator halt: %v\n", r.OperatorHalted)
	md += "Credential env present:\n"
	for _, env := range []string{"OKX_API_KEY", "OKX_API_SECRET", "OKX_API_PASSPHRASE"} {
		if _, ok := r.CredentialEnvPresent[env]; ok {
			md += fmt.Sprintf("- %s: %v\n", env, r.CredentialEnvPresent[env])
		}
	}
	md += fmt.Sprintf("Plan state: %s\n", r.PlanState)
	md += fmt.Sprintf("Proof: %s | %s\n", r.Proof.Status, r.Proof.Summary)
	md += fmt.Sprintf("Open live orders: %d\n", len(r.OpenLiveOrders))
	md += fmt.Sprintf("Live positions: %d\n", len(r.LivePositions))
	md += fmt.Sprintf("Data health: %s | %s\n", r.DataHealth.Status, r.DataHealth.Summary)
	if r.DataSanity.Status != "" {
		md += fmt.Sprintf("Data sanity: %s | %s\n", r.DataSanity.Status, r.DataSanity.Summary)
	}
	md += fmt.Sprintf("Reconcile safety: %s | %s\n", r.ReconcileSafety.Status, r.ReconcileSafety.Summary)
	md += fmt.Sprintf("Risk governor: %s | %s\n", r.RiskGovernor.Status, r.RiskGovernor.Summary)
	if len(r.DataHealth.Blockers) > 0 {
		md += "Data health blockers:\n"
		for _, reason := range r.DataHealth.Blockers {
			md += "- " + reason + "\n"
		}
	}
	if len(r.RiskGovernor.Blockers) > 0 {
		md += "Risk governor blockers:\n"
		for _, reason := range r.RiskGovernor.Blockers {
			md += "- " + reason + "\n"
		}
	}
	if r.Proof.Candidate.Symbol != "" {
		md += fmt.Sprintf("Candidate: %s %s limit %.8f qty %.8f notional %.2f\n", r.Proof.Candidate.Side, r.Proof.Candidate.Symbol, r.Proof.Candidate.Price, r.Proof.Candidate.Quantity, r.Proof.Candidate.Notional)
	}
	if r.Proof.Preflight.Enabled {
		md += fmt.Sprintf("Preflight: pass=%v inst_id=%s notional=%.2f reasons=%v\n", r.Proof.Preflight.Pass, r.Proof.Preflight.InstID, r.Proof.Preflight.Notional, r.Proof.Preflight.Reasons)
	}
	if len(r.AutoLiveBlockers) > 0 {
		md += "\nAuto live blockers:\n"
		for _, reason := range r.AutoLiveBlockers {
			md += "- " + reason + "\n"
		}
	}
	if len(r.ManagedCoinSummaries) > 0 && (r.Proof.Candidate.Symbol == "" || r.PlanState != agent2.StateActiveLimit) {
		md += "\nWhy no auto order:\n"
		for _, coin := range r.ManagedCoinSummaries {
			if coin.DesiredLayers > 0 || coin.Placed > 0 || coin.Kept > 0 {
				continue
			}
			reasons := firstStrings(coin.WhyNoOrder, 4)
			line := fmt.Sprintf("- %s: state=%s desired_layers=%d", coin.Symbol, coin.State, coin.DesiredLayers)
			if len(reasons) > 0 {
				line += " | " + strings.Join(reasons, "; ")
			}
			if coin.NextTrigger != "" {
				line += " | next=" + coin.NextTrigger
			}
			md += line + "\n"
		}
	}
	md += "\nNo order was placed.\n"
	return md
}

func runExecuteLiveProofOrder(ctx context.Context, cfg config.Config, db *storage.DB, confirm string) error {
	p, err := db.LatestPlan()
	if err != nil {
		return fmt.Errorf("load latest plan: %w", err)
	}
	client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
	var balanceReader liveguard.BalanceReader
	var filterReader liveguard.FilterReader
	var placer liveguard.OrderPlacer
	if err == nil {
		balanceReader = client
		filterReader = client
		placer = client
	}
	proof := liveguard.BuildProofWithChecks(ctx, cfg, p, balanceReader, filterReader)
	result := liveguard.ExecuteManualProofOrder(ctx, cfg, proof, confirm, placer, db)
	if result.Status == liveguard.LiveOrderSubmitted {
		if err := db.SaveLiveOrderFromParams(
			result.Order.ClientOrderID,
			result.Order.OrderID,
			result.Order.InstID,
			result.Candidate.Symbol,
			result.Candidate.Side,
			result.Candidate.Type,
			result.Candidate.Price,
			result.Candidate.Quantity,
			result.Candidate.Notional,
			live.StatusLiveOpen,
		); err != nil {
			return fmt.Errorf("save live order: %w", err)
		}
		if err := db.SaveLiveOrderEvent(live.OrderStatus{
			ClientOrderID: result.Order.ClientOrderID,
			OrderID:       result.Order.OrderID,
			InstID:        result.Order.InstID,
			Status:        live.StatusLiveOpen,
		}); err != nil {
			return fmt.Errorf("save live order event: %w", err)
		}
	}
	if err := saveJSONFile("reports", "live_order_proof_latest.json", result); err != nil {
		return err
	}
	md := liveOrderMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "live_order_proof_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(ctx, cfg, "manual-live-order", telegramreport.LiveOrderHumanText(result, false))
	}
	fmt.Println(md)
	return nil
}

func runAutoLiveOrder(ctx context.Context, cfg config.Config, db *storage.DB, dryRun bool) error {
	return runAutoLiveOrderWithNotify(ctx, cfg, db, dryRun, true)
}

func runAutoLiveOrderWithNotify(ctx context.Context, cfg config.Config, db *storage.DB, dryRun bool, notifyTelegram bool) error {
	if err := requireAutoLiveRuntime(cfg); err != nil {
		return err
	}
	p, err := refreshDeterministicPlanForLive(ctx, cfg, db)
	if err != nil {
		return err
	}
	open, err := db.OpenLiveOrdersDetailed()
	if err != nil {
		return fmt.Errorf("load open live orders: %w", err)
	}
	if len(open) > 0 || cfg.Live.OrderManagementEnabled {
		if err := runReconcileLiveOrdersWithNotify(ctx, cfg, db, notifyTelegram && !dryRun); err != nil {
			return fmt.Errorf("pre-auto reconcile live orders: %w", err)
		}
		open, err = db.OpenLiveOrdersDetailed()
		if err != nil {
			return fmt.Errorf("reload open live orders after reconcile: %w", err)
		}
	}
	positions, err := db.LivePositions()
	if err != nil {
		return fmt.Errorf("load live positions: %w", err)
	}
	analysis, err := db.LatestAnalysis()
	if err != nil {
		return fmt.Errorf("load latest analysis for safety gates: %w", err)
	}
	assets, err := loadAssets(cfg, db)
	if err != nil {
		return fmt.Errorf("load assets for safety gates: %w", err)
	}
	dataHealth := liveguard.CheckDataHealth(cfg, analysis, p, assets, open, positions, time.Now())
	dataSanity := liveguard.DataSanityResult{}
	benchmarks := map[string][]market.Candle{}
	if btc, err := loadBTC(cfg, db); err == nil {
		dataSanity = liveguard.CheckDataSanity(cfg, btc, assets, analysis, time.Now())
		if btc1d := btc["1d"]; len(btc1d) > 0 {
			benchmarks[cfg.Data.Symbols.BTC] = btc1d
			benchmarks["BTCUSDT"] = btc1d
		}
	}
	shadow := liveguard.BuildShadowProbeJournal(cfg, analysis, p, assets, benchmarks, dataSanity, time.Now())
	if err := liveguard.SaveShadowProbeJournal("reports", shadow); err != nil {
		log.Printf("shadow probe journal warning: %v", err)
	}
	reconcileSafety := liveguard.ReconcileSafety(liveguard.ReconcileResult{Checked: len(open), Orders: open})
	riskGovernor := liveguard.EvaluateRiskGovernor(cfg, analysis, p, open, positions, dataHealth, dataSanity, reconcileSafety)
	if dataHealth.Status == liveguard.DataHealthBlock || reconcileSafety.Status == liveguard.ReconcileBlock || riskGovernor.Status == liveguard.RiskGovernorBlock || !cfg.Live.OrderManagementEnabled {
		result := liveguard.ManagedCycleResult{GeneratedAt: time.Now(), Status: liveguard.ManagedCycleBlocked, PlanState: p.State, Desired: []liveguard.ManagedDesiredOrder{}, DryRun: dryRun, DataHealth: dataHealth, ReconcileSafety: reconcileSafety, RiskGovernor: riskGovernor}
		result.Reasons = append(result.Reasons, dataHealth.Blockers...)
		result.Reasons = append(result.Reasons, reconcileSafety.Blockers...)
		result.Reasons = append(result.Reasons, riskGovernor.Blockers...)
		if !cfg.Live.OrderManagementEnabled {
			result.Reasons = append(result.Reasons, "live.order_management_enabled=false")
		}
		result.Reasons = uniqueStringsMain(result.Reasons)
		result.PerCoin = liveguard.BuildManagedCoinSummaries(cfg, p, open, result)
		result.Summary = result.Status + ": " + strings.Join(result.Reasons, "; ")
		return writeAutoLiveManagementResult(ctx, cfg, db, result, notifyTelegram && !dryRun)
	}
	client, err := live.NewOKXFromEnv("", cfg.Live.APIKeyEnv, cfg.Live.APISecretEnv, cfg.Live.APIPassphraseEnv)
	var filterReader liveguard.FilterReader
	var placer liveguard.OrderPlacer
	var canceler liveguard.OrderCanceler
	if err == nil {
		filterReader = client
		placer = client
		canceler = client
		// Refresh total_capital from live OKX USDT balance each cycle so
		// AllocateLiveCapital uses real funds instead of a stale config value.
		if liveBalances, berr := client.AccountBalance(ctx); berr == nil {
			for _, b := range liveBalances {
				if strings.EqualFold(b.Asset, "USDT") && b.Free > 0 {
					cfg.Portfolio.TotalCapital = b.Free
					log.Printf("live capital updated from OKX balance: %.2f USDT", b.Free)
					break
				}
			}
		} else {
			log.Printf("live balance fetch warning (using config value %.2f): %v", cfg.Portfolio.TotalCapital, berr)
		}
	}
	// Skip OKX InstrumentFilters HTTP call when plan is not ACTIVE_LIMIT
	// and there are no open orders to cancel. Avoids unnecessary API usage every cycle.
	noActiveWork := p.State != agent2.StateActiveLimit && len(open) == 0 && !cfg.HermesOperator.CanExecute()
	filters := []live.InstrumentFilter{}
	if filterReader != nil && !noActiveWork {
		filters, err = filterReader.InstrumentFilters(ctx)
		if err != nil {
			return fmt.Errorf("load instrument filters for order management: %w", err)
		}
	}
	var recorder liveguard.ManagedOrderRecorder
	if !dryRun {
		recorder = db
	}
	hasHistory, historyErr := db.HasManagedRealOrderSubmission()
	if historyErr != nil {
		log.Printf("managed order history warning: %v", historyErr)
	}
	dryRunApproved := dryRun
	if !dryRun {
		approved, approvalReasons := loadFirstOrderDryRunApproval(filepath.Join("reports", "live_auto_audit_latest.json"), time.Now().UTC())
		dryRunApproved = approved
		if !approved {
			log.Printf("first-order dry-run audit not approved: %s", strings.Join(approvalReasons, "; "))
		}
	}
	execCtx := liveguard.ManagedExecutionContext{BTCAccumulationPhase: string(analysis.BTCAccumulation.Phase), FirstOrderDryRunApproved: dryRunApproved}
	if historyErr == nil {
		execCtx.ManagedOrderHistoryKnown = true
		execCtx.HasManagedRealOrderHistory = hasHistory
	}
	if cfg.HermesOperator.CanExecute() {
		if hermesResult, handled := executeLatestHermesDecision(ctx, cfg, db, p, analysis, open, positions, filters, dataHealth, reconcileSafety, riskGovernor, placer, execCtx, dryRun); handled {
			if !dryRun {
				_ = persistManagedCycleResult(db, hermesResult)
			}
			return writeAutoLiveManagementResult(ctx, cfg, db, hermesResult, notifyTelegram && !dryRun)
		}
	}
	result := liveguard.ManageLiveOrdersWithRecorderAndContext(ctx, cfg, p, open, positions, filters, placer, canceler, db, execCtx, recorder, dryRun)
	result.DataHealth = dataHealth
	result.ReconcileSafety = reconcileSafety
	result.RiskGovernor = riskGovernor
	if !dryRun {
		if err := persistManagedCycleResult(db, result); err != nil {
			return err
		}
	}
	if !dryRun && result.Status != liveguard.ManagedCycleBlocked && (len(result.Canceled) > 0 || len(result.Placed) > 0 || len(result.Replaced) > 0) {
		if err := runReconcileLiveOrders(ctx, cfg, db); err != nil {
			log.Printf("post-managed auto reconcile warning: %v", err)
		}
	}
	return writeAutoLiveManagementResult(ctx, cfg, db, result, notifyTelegram && !dryRun)
}

func refreshDeterministicPlanForLive(ctx context.Context, cfg config.Config, db *storage.DB) (agent2.Plan, error) {
	if err := fetch(ctx, cfg, db); err != nil {
		return agent2.Plan{}, err
	}
	analysis, err := analyze(ctx, cfg, db)
	if err != nil {
		return agent2.Plan{}, err
	}
	assets, err := loadAssets(cfg, db)
	if err != nil {
		return agent2.Plan{}, err
	}
	btc1d, err := db.LoadCandles(cfg.Data.Symbols.BTC, "1d", cfg.Data.CandleLimit)
	if err != nil {
		return agent2.Plan{}, fmt.Errorf("load BTC benchmark for live plan: %w", err)
	}
	benchmarks := map[string][]market.Candle{cfg.Data.Symbols.BTC: btc1d, "BTCUSDT": btc1d}
	p := agent2.BuildPlanWithBenchmarks(cfg, analysis, assets, benchmarks)
	p = applyMicrostructureAssetGate(cfg, p, latestMicrostructureSummary(cfg, db, time.Now().UTC()))
	if err := db.SavePlan(p); err != nil {
		return p, err
	}
	orders := agent2.OrdersFromPlan(p, cfg.Execution.OrderExpiryHours)
	if err := db.SaveOrders(orders); err != nil {
		return p, err
	}
	return p, nil
}

func writeAutoLiveManagementResult(ctx context.Context, cfg config.Config, db *storage.DB, result liveguard.ManagedCycleResult, notifyTelegram bool) error {
	if err := db.SaveManagedCycleReport(result); err != nil {
		return fmt.Errorf("save managed cycle report: %w", err)
	}
	if err := saveJSONFile("reports", "auto_live_management_latest.json", result); err != nil {
		return err
	}
	md := autoLiveManagementMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "auto_live_management_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	if err := writeFilterAttributionReportFromManaged(result); err != nil {
		log.Printf("filter attribution report warning: %v", err)
	}
	if snapshot, err := buildBotRuntimeSnapshot(cfg, db, liveguard.SupervisorResult{Managed: &result}); err != nil {
		log.Printf("analysis report snapshot warning: %v", err)
	} else {
		technicalReport := buildTechnicalScorecardReport(snapshot)
		if err := writeTechnicalScorecardReportFile(technicalReport); err != nil {
			log.Printf("technical scorecard report warning: %v", err)
		}
		capitalReport := buildCapitalPlanResearchReport(cfg, snapshot)
		if err := writeCapitalPlanResearchReportFile(capitalReport); err != nil {
			log.Printf("capital plan research report warning: %v", err)
		}
		filterReport := buildFilterAttributionReport(snapshot)
		scenario := buildScenarioReport(cfg, snapshot)
		if err := writeDecisionDashboardReport(snapshot, scenario, technicalReport, capitalReport, filterReport); err != nil {
			log.Printf("decision dashboard report warning: %v", err)
		}
	}
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" && notifyTelegram {
		sendTelegram(ctx, cfg, "auto-live-management", telegramreport.LiveOrderManagementHumanText(result))
	}
	fmt.Println(md)
	return nil
}

func persistManagedCycleResult(db *storage.DB, result liveguard.ManagedCycleResult) error {
	now := time.Now().Unix()
	for _, decision := range result.Canceled {
		status := decision.Order
		status.Status = live.StatusCanceled
		status.UpdatedAt = now
		status.LastManagementAction = "canceled: " + decision.Reason
		if err := db.SaveLiveOrderStatus(status); err != nil {
			return fmt.Errorf("save canceled live order: %w", err)
		}
		if err := db.SaveLiveOrderEvent(status); err != nil {
			return fmt.Errorf("save canceled live order event: %w", err)
		}
	}
	for _, decision := range result.Placed {
		order := decision.PlaceResult
		desired := decision.Desired
		if !order.Submitted {
			continue
		}
		expiresAt := now
		if !desired.ExpiresAt.IsZero() {
			expiresAt = desired.ExpiresAt.Unix()
		}
		meta := live.OrderStatus{ClientOrderID: order.ClientOrderID, OrderID: order.OrderID, InstID: order.InstID, Symbol: desired.Symbol, Side: desired.Side, OrderType: desired.Type, Price: desired.Price, Quantity: desired.Quantity, Notional: desired.Notional, Status: live.StatusSubmitted, LayerIndex: desired.LayerIndex, Source: desired.Source, InvalidationPrice: desired.InvalidationPrice, DecisionReason: desired.DecisionReason, LastManagementAction: "placed: " + decision.Reason, ExpiresAt: expiresAt}
		if err := db.SaveLiveOrderStatus(meta); err != nil {
			if err := db.SaveManagedLiveOrder(order.ClientOrderID, order.OrderID, order.InstID, desired.Symbol, desired.Side, desired.Type, desired.Price, desired.Quantity, desired.Notional, live.StatusSubmitted, meta); err != nil {
				return fmt.Errorf("save managed live order status: %w", err)
			}
		}
		if err := db.SaveLiveOrderEvent(meta); err != nil {
			return fmt.Errorf("save managed live order event: %w", err)
		}
	}
	return nil
}

func autoLiveManagementMarkdown(result liveguard.ManagedCycleResult) string {
	md := fmt.Sprintf("AUTO LIVE MANAGEMENT\n\nStatus: %s\nSummary: %s\nPlan state: %s\nDry run: %v\nDesired: %d | Kept: %d | Canceled: %d | Replaced: %d | Placed: %d | Blocked: %d\n", result.Status, result.Summary, result.PlanState, result.DryRun, len(result.Desired), len(result.Kept), len(result.Canceled), len(result.Replaced), len(result.Placed), len(result.Blocked))
	if result.DataHealth.Status != "" {
		md += fmt.Sprintf("Data health: %s | %s\n", result.DataHealth.Status, result.DataHealth.Summary)
	}
	if result.ReconcileSafety.Status != "" {
		md += fmt.Sprintf("Reconcile safety: %s | %s\n", result.ReconcileSafety.Status, result.ReconcileSafety.Summary)
	}
	if result.RiskGovernor.Status != "" {
		md += fmt.Sprintf("Risk governor: %s | %s\n", result.RiskGovernor.Status, result.RiskGovernor.Summary)
	}
	appendDecision := func(title string, items []liveguard.ManagedOrderDecision) {
		if len(items) == 0 {
			return
		}
		md += "\n" + title + ":\n"
		for _, d := range items {
			md += "- " + managementDecisionLine(d) + "\n"
		}
	}
	appendDecision("Kept", result.Kept)
	appendDecision("Canceled", result.Canceled)
	appendDecision("Replaced", result.Replaced)
	appendDecision("Placed", result.Placed)
	appendDecision("Blocked", result.Blocked)
	if len(result.Reasons) > 0 {
		md += "\nReasons: " + fmt.Sprint(result.Reasons) + "\n"
	}
	if len(result.PerCoin) > 0 {
		md += "\nPER COIN\n"
		for _, coin := range result.PerCoin {
			md += fmt.Sprintf("\n%s\nState: %s\nOpen orders: %d\nDesired layers: %d\nPending notional: %.2f USDT\n", coin.Symbol, coin.State, coin.OpenOrders, coin.DesiredLayers, coin.PendingNotional)
			if len(coin.Actions) == 0 {
				md += "Actions: none\n"
			} else {
				md += "Actions:\n"
				for _, action := range coin.Actions {
					md += "- " + managementDecisionLine(action) + "\n"
				}
			}
			if len(coin.Reasons) > 0 {
				md += "Reasons: " + strings.Join(coin.Reasons, "; ") + "\n"
			}
			if len(coin.WhyNoOrder) > 0 {
				md += "Why no order: " + strings.Join(coin.WhyNoOrder, "; ") + "\n"
			}
			if coin.NextTrigger != "" {
				md += "Next trigger: " + coin.NextTrigger + "\n"
			}
		}
	}
	return md
}

func managementDecisionLine(d liveguard.ManagedOrderDecision) string {
	symbol := firstNonEmpty(d.Symbol, d.Desired.Symbol, d.Order.Symbol, live.InternalSymbol(d.Order.InstID))
	layer := firstNonZero(d.LayerIndex, d.Desired.LayerIndex, d.Order.LayerIndex)
	price := d.Desired.Price
	notional := d.Desired.Notional
	if price <= 0 {
		price = d.Order.Price
	}
	if notional <= 0 {
		notional = d.Order.Notional
	}
	out := fmt.Sprintf("%s layer=%d action=%s", symbol, layer, d.Action)
	if price > 0 {
		out += fmt.Sprintf(" @ %.8f", price)
	}
	if notional > 0 {
		out += fmt.Sprintf(" notional=%.2f", notional)
	}
	if d.Desired.AllocationTier != "" {
		out += fmt.Sprintf(" tier=%s score=%.1f", d.Desired.AllocationTier, d.Desired.AllocationScore)
	}
	if d.Desired.AllocationReason != "" {
		out += " allocation=" + d.Desired.AllocationReason
	}
	if d.Reason != "" {
		out += ": " + d.Reason
	}
	if d.Order.ClientOrderID != "" {
		out += fmt.Sprintf(" clOrdId=%s", d.Order.ClientOrderID)
	}
	if d.Error != "" {
		out += " error=" + d.Error
	}
	if len(d.AuditTrail) > 0 {
		out += " audit=" + strings.Join(d.AuditTrail, " | ")
	}
	return out
}

func liveOrderMarkdown(result liveguard.ExecutionResult) string {
	md := fmt.Sprintf("MANUAL LIVE PROOF ORDER\n\nStatus: %s\nSummary: %s\nProof status: %s\n", result.Status, result.Summary, result.ProofStatus)
	if result.Candidate.Symbol != "" {
		md += fmt.Sprintf("Candidate: %s %s limit %.8f qty %.8f notional %.2f post_only=%v\n", result.Candidate.Side, result.Candidate.Symbol, result.Candidate.Price, result.Candidate.Quantity, result.Candidate.Notional, result.Candidate.PostOnly)
	}
	if result.Preflight.Enabled {
		md += fmt.Sprintf("Preflight: pass=%v inst_id=%s notional=%.2f\n", result.Preflight.Pass, result.Preflight.InstID, result.Preflight.Notional)
	}
	if result.Order.Submitted {
		md += fmt.Sprintf("Order: submitted=true inst_id=%s order_id=%s client_order_id=%s\n", result.Order.InstID, result.Order.OrderID, result.Order.ClientOrderID)
	} else {
		md += "Order: submitted=false\n"
	}
	if len(result.Reasons) > 0 {
		md += "Reasons: " + fmt.Sprint(result.Reasons) + "\n"
	}
	return md
}

func runSimulateLiveManager(cfg config.Config) error {
	result := liveguard.RunLiveManagerSimulation(cfg)
	if err := saveJSONFile("reports", "live_manager_simulation_latest.json", result); err != nil {
		return err
	}
	md := liveManagerSimulationMarkdown(result)
	if err := os.MkdirAll("reports", 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("reports", "live_manager_simulation_latest.md"), []byte(md), 0600); err != nil {
		return err
	}
	fmt.Println(md)
	if !result.Passed {
		return fmt.Errorf("live manager simulation failed")
	}
	return nil
}

func liveManagerSimulationMarkdown(result liveguard.LiveManagerSimulationResult) string {
	md := fmt.Sprintf("LIVE MANAGER SIMULATION\n\nGenerated: %s\nSummary: %s\nPassed: %v\nScenarios: %d\n\n", result.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), result.Summary, result.Passed, len(result.Scenarios))
	for i, scenario := range result.Scenarios {
		status := "PASS"
		if !scenario.Passed {
			status = "FAIL"
		}
		md += fmt.Sprintf("%d) %s — %s\n", i+1, scenario.Name, status)
		md += fmt.Sprintf("   Expected: %s\n", scenario.Expected)
		md += fmt.Sprintf("   Result: desired=%d kept=%d canceled=%d replaced=%d placed=%d blocked=%d\n", len(scenario.Result.Desired), len(scenario.Result.Kept), len(scenario.Result.Canceled), len(scenario.Result.Replaced), len(scenario.Result.Placed), len(scenario.Result.Blocked))
		if scenario.Failure != "" {
			md += "   Failure: " + scenario.Failure + "\n"
		}
	}
	md += "\nNo real order was placed or canceled. Simulation only.\n"
	return md
}
