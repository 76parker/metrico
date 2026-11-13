package reporter

import (
	"context"
	"time"
)

const maxRetries = 3

func withRetry(ctx context.Context, fn func() error) error {
	retryAttempts := 0
	delay := 1 * time.Second
	for {
		// Если это не первый вызов, то ждем перед повторным вызовом
		if retryAttempts != 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return context.Cause(ctx)
			case <-timer.C:
			}
			delay += 2 * time.Second
		}
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		err := fn()
		if ctx.Err() != nil {
			return context.Cause(ctx)
		}
		if err == nil {
			return nil
		}
		retryAttempts++
		// Выходим сразу если количество попыток превысило maxRetries
		if retryAttempts > maxRetries || !isRetryable(err) {
			return err
		}
	}
}
