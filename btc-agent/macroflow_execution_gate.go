package main

import (
	"fmt"
	"time"

	"btc-agent/internal/freeapi"
	"btc-agent/internal/macroflow"
)

// macroExposureBlockReason is deliberately a one-way veto. It cannot create
// BUY authority and stale/unavailable advisory data cannot replace the existing
// deterministic market, plan, risk, lifecycle, capital, or operator gates.
func macroExposureBlockReason(report freeapi.Report, now time.Time, maxAge time.Duration) string {
	if maxAge <= 0 || report.GeneratedAt.IsZero() || now.Before(report.GeneratedAt) || now.Sub(report.GeneratedAt) > maxAge {
		return ""
	}
	if macroflow.BlocksNewExposure(report.MacroFlow) {
		return fmt.Sprintf("macro-flow veto: regime=%s confidence=%.2f", report.MacroFlow.Regime, report.MacroFlow.Confidence)
	}
	return ""
}
