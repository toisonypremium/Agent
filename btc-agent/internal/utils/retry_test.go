package utils

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetrySucceedsAfterFailure(t *testing.T) {
	attempts := 0
	err := Retry(context.Background(), 3, 0, func() error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d want 2", attempts)
	}
}

func TestRetryStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Retry(ctx, 3, time.Millisecond, func() error {
		t.Fatal("fn should not run after context is canceled")
		return nil
	})
	if err == nil {
		t.Fatal("expected context error")
	}
}
