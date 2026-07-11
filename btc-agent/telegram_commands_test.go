package main

import (
	"strings"
	"testing"
	"time"

	"btc-agent/internal/agent2"
	"btc-agent/internal/liveguard"
)

func TestNormalizeTelegramCommandReadOnlyAllowlist(t *testing.T) {
	for _, input := range []string{"/status", "/why now", "/coins@btc_agent_bot", "/filters", "/orders", "/positions", "/doctor", "/supervisor", "/next", "/risk", "/help"} {
		if got := normalizeTelegramCommand(input); got == "" {
			t.Fatalf("expected allowed command for %q", input)
		}
	}
	for _, input := range []string{"buy", "/buy", "/sell", "/market", "/leverage", "/override", "/resume", "/halt", "/cancel", "/close"} {
		if got := normalizeTelegramCommand(input); got != "" {
			t.Fatalf("expected blocked command for %q, got %q", input, got)
		}
	}
}

func TestTelegramChatAllowedExactMatch(t *testing.T) {
	if !telegramChatAllowed("12345", 12345) {
		t.Fatal("expected exact chat id allowed")
	}
	if telegramChatAllowed("12345", 67890) {
		t.Fatal("expected different chat id blocked")
	}
	if telegramChatAllowed("", 12345) {
		t.Fatal("expected empty configured chat blocked")
	}
}

func TestTelegramCommandsHelpIsReadOnly(t *testing.T) {
	text := telegramCommandsHelp()
	for _, want := range []string{"/status", "/why", "/coins", "/filters", "/orders", "/positions", "/doctor", "/supervisor", "/next", "/risk", "Không có lệnh đặt mua/bán"} {
		if !strings.Contains(text, want) {
			t.Fatalf("help missing %q:\n%s", want, text)
		}
	}
	for _, blocked := range []string{"/buy", "/sell", "/market", "/leverage", "/override", "/cancel", "/close"} {
		if strings.Contains(text, blocked) {
			t.Fatalf("help exposes blocked command %q:\n%s", blocked, text)
		}
	}
}

func TestTelegramCommandFiltersIsReadOnly(t *testing.T) {
	report := FilterAttributionReport{
		Summary:   "Filter attribution coins=1 near_actionable=0 top_blocker=DISCOUNT_ZONE",
		Aggregate: []FilterAttributionAggregateRow{{Key: "DISCOUNT_ZONE", Count: 1}},
		Coins:     []FilterAttributionCoinRow{{Symbol: "ETHUSDT", State: agent2.StateWatch, SetupScore: 0.62, TopBlockerKey: "DISCOUNT_ZONE", FailedSoft: 2}},
		Safety:    "spot limit BUY post-only only; no futures, no leverage, no market order",
	}
	text := telegramCommandFilters(report)
	for _, want := range []string{"BTC Agent — Filters", "DISCOUNT_ZONE", "Read-only", "không bypass ACTIVE_LIMIT"} {
		if !strings.Contains(text, want) {
			t.Fatalf("filters reply missing %q:\n%s", want, text)
		}
	}
}

func TestTelegramCommandPositionsIsReadOnly(t *testing.T) {
	report := liveguard.LiveLedgerReport{GeneratedAt: time.Now(), Summary: "no live positions recorded"}
	text := telegramCommandPositions(report)
	for _, want := range []string{"BTC Agent — Positions", "Không có vị thế live", "Read-only"} {
		if !strings.Contains(text, want) {
			t.Fatalf("positions reply missing %q:\n%s", want, text)
		}
	}
}
