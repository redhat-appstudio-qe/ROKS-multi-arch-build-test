package providers

import (
	"context"
	"fmt"
	"time"
)

const (
	DefaultMax5xxRetries = 3
	Default5xxRetryDelay = 3 * time.Minute
)

type RetryPolicy struct {
	MaxRetries int
	Delay      time.Duration
	Sleep      func(context.Context, time.Duration) error
}

// DefaultRetryPolicy returns the configured maximum retry count and delay for provider 5xx errors.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxRetries: DefaultMax5xxRetries, Delay: Default5xxRetryDelay}
}

// RetryOn5xx retries an operation after classified 5xx errors, up to policy.MaxRetries times.
func RetryOn5xx(ctx context.Context, policy RetryPolicy, operation func() error, is5xx func(error) bool) error {
	if operation == nil {
		return fmt.Errorf("retry operation is required")
	}
	if is5xx == nil {
		return fmt.Errorf("5xx classifier is required")
	}
	if policy.MaxRetries < 0 {
		policy.MaxRetries = 0
	}
	sleep := policy.Sleep
	if sleep == nil {
		sleep = waitRetryDelay
	}

	for attempt := 0; ; attempt++ {
		err := operation()
		if err == nil || !is5xx(err) || attempt >= policy.MaxRetries {
			return err
		}
		if err := sleep(ctx, policy.Delay); err != nil {
			return err
		}
	}
}

// waitRetryDelay waits for the retry delay or returns when the context is canceled.
func waitRetryDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
