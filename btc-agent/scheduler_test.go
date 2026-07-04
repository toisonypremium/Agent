package main

import (
	"testing"
	"time"
)

func TestGetNextRunTime(t *testing.T) {
	// Setup timezone locations
	hcm, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}

	t.Run("daily run time is later today", func(t *testing.T) {
		// Now: 2026-07-04 06:00:00 ICT
		now := time.Date(2026, 7, 4, 6, 0, 0, 0, hcm)
		dailyRunTime := "08:00"

		got, err := getNextRunTime(dailyRunTime, hcm, now)
		if err != nil {
			t.Fatal(err)
		}

		expected := time.Date(2026, 7, 4, 8, 0, 0, 0, hcm)
		if !got.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, got)
		}
	})

	t.Run("daily run time is earlier today", func(t *testing.T) {
		// Now: 2026-07-04 10:00:00 ICT
		now := time.Date(2026, 7, 4, 10, 0, 0, 0, hcm)
		dailyRunTime := "08:00"

		got, err := getNextRunTime(dailyRunTime, hcm, now)
		if err != nil {
			t.Fatal(err)
		}

		expected := time.Date(2026, 7, 5, 8, 0, 0, 0, hcm)
		if !got.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, got)
		}
	})

	t.Run("daily run time is exactly now", func(t *testing.T) {
		// Now: 2026-07-04 08:00:00 ICT
		now := time.Date(2026, 7, 4, 8, 0, 0, 0, hcm)
		dailyRunTime := "08:00"

		got, err := getNextRunTime(dailyRunTime, hcm, now)
		if err != nil {
			t.Fatal(err)
		}

		expected := time.Date(2026, 7, 5, 8, 0, 0, 0, hcm)
		if !got.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, got)
		}
	})

	t.Run("invalid daily run time format", func(t *testing.T) {
		now := time.Now()
		_, err := getNextRunTime("invalid", time.UTC, now)
		if err == nil {
			t.Error("expected error for invalid format")
		}
	})
}
