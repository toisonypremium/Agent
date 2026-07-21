package main

import (
	"strings"
	"time"

	"btc-agent/internal/config"
)

func hermesSchedulePolicy(cfg config.Config) (available, scheduled bool, interval time.Duration) {
	interval = time.Duration(cfg.AI.HermesIntervalMinutes) * time.Minute
	available = cfg.AI.Enabled && cfg.Live.SupervisorEnabled
	scheduled = available && interval > 0
	return available, scheduled, interval
}

func telegramBriefScheduleEnabled(cfg config.Config) bool {
	return cfg.Notify.Enabled && strings.EqualFold(strings.TrimSpace(cfg.Notify.Provider), "telegram")
}
