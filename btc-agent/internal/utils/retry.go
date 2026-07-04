package utils

import (
	"context"
	"time"
)

func Retry(ctx context.Context, attempts int, baseDelay time.Duration, fn func() error) error {
	if attempts <= 0 {
		attempts = 1
	}
	if baseDelay < 0 {
		baseDelay = 0
	}
	var last error
	for i := 0; i < attempts; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(); err != nil {
			last = err
		} else {
			return nil
		}
		if i == attempts-1 || baseDelay == 0 {
			continue
		}
		delay := baseDelay * time.Duration(1<<i)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return last
}
