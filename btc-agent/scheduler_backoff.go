package main

import (
	"btc-agent/internal/config"
	"btc-agent/internal/liveguard"
	"btc-agent/internal/opsplan"
	"btc-agent/internal/storage"
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

func marketWatchSuccessDelay(base time.Duration, report opsplan.Report) time.Duration {
	if base <= 0 {
		base = 15 * time.Minute
	}
	recommended := time.Duration(report.Monitoring.RecommendedScanMinutes) * time.Minute
	if recommended <= 0 {
		return base
	}
	if recommended < base {
		return recommended
	}
	return base
}

func marketWatchFailureDelay(base time.Duration, consecutiveErrors int) time.Duration {
	if base <= 0 {
		base = 15 * time.Minute
	}
	if consecutiveErrors < 1 {
		consecutiveErrors = 1
	}
	factor := consecutiveErrors
	if factor > 4 {
		factor = 4
	}
	delay := base / time.Duration(factor)
	if delay < 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}

func shouldRefreshMarketDataForDoctor(doctor liveguard.RuntimeDoctorResult) bool {
	if doctor.Status != liveguard.DoctorBlock {
		return false
	}
	for _, blocker := range doctor.Blockers {
		b := strings.ToLower(blocker)
		if strings.Contains(b, "analysis stale") || strings.Contains(b, "plan stale") || strings.Contains(b, "1d candle") {
			return true
		}
	}
	if doctor.DataHealth.Status == liveguard.DataHealthBlock {
		for _, blocker := range doctor.DataHealth.Blockers {
			b := strings.ToLower(blocker)
			if strings.Contains(b, "analysis stale") || strings.Contains(b, "plan stale") || strings.Contains(b, "1d candle") {
				return true
			}
		}
	}
	return false
}

func updateDoctorBlockWatchdog(ctx context.Context, cfg config.Config, db *storage.DB, doctor liveguard.RuntimeDoctorResult, consecutive int) int {
	if doctor.Status != liveguard.DoctorBlock {
		if consecutive > 0 {
			log.Printf("[Scheduler] Live doctor block watchdog reset after status=%s", doctor.Status)
		}
		return 0
	}
	consecutive++
	threshold := cfg.Live.AutoHaltAfterErrors
	log.Printf("[Scheduler] Live doctor block watchdog: consecutive=%d threshold=%d summary=%s", consecutive, threshold, doctor.Summary)
	if threshold <= 0 || consecutive < threshold {
		return consecutive
	}
	halted, err := db.IsHalted()
	if err != nil {
		log.Printf("[Scheduler] Live doctor block watchdog halt check error: %v", err)
		return consecutive
	}
	if halted {
		log.Printf("[Scheduler] Live doctor block watchdog threshold reached; operator halt already active")
		return consecutive
	}
	if err := db.SetHaltStatus(true); err != nil {
		log.Printf("[Scheduler] Live doctor block watchdog set halt error: %v", err)
		return consecutive
	}
	text := fmt.Sprintf("Operator halt: ACTIVE (auto-halt after %d consecutive live-doctor blocks). Last doctor: %s", consecutive, doctor.Summary)
	log.Printf("[Scheduler] %s", text)
	if cfg.Notify.Enabled && cfg.Notify.Provider == "telegram" {
		sendTelegram(ctx, cfg, "operator-halt", text)
	}
	return consecutive
}
