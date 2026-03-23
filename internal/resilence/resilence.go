package resilence

import (
	"context"
	"errors"
	"image-pipeline/internal/utils"
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

type Executor interface {
	Execute(ctx context.Context, fn func(ctx context.Context) error) error
}

type executor struct {
	logger  *zap.Logger
	name    string
	retries int
	timeout time.Duration
	breaker *gobreaker.CircuitBreaker
}

func NewExecutor(logger *zap.Logger, name string, retries int, timeout time.Duration) Executor {
	settings := gobreaker.Settings{
		Name:        name,
		MaxRequests: 3,
		Interval:    30 * time.Second,
		Timeout:     10 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	}

	cb := gobreaker.NewCircuitBreaker(settings)
	return &executor{
		logger:  logger,
		name:    name,
		breaker: cb,
		retries: retries,
		timeout: timeout,
	}
}

func (e *executor) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	var callerErr error
	// Circuit breaker execution
	maxAttempts := e.retries + 1
	_, cbErr := e.breaker.Execute(func() (interface{}, error) {
		for i := 0; i < maxAttempts; i++ {
			runCtx, cancel := context.WithTimeout(ctx, e.timeout)
			callerErr = fn(runCtx)
			cancel()

			if callerErr == nil {
				return nil, nil
			}

			if isClientCancellation(callerErr) {
				e.logger.Info("client context cancelled, not counting as failure",
					zap.String("executor", e.name),
				)
				return nil, nil
			}

			if isNonRetryable(callerErr) {
				e.logger.Warn("non-retryable error, stopping",
					zap.String("executor", e.name),
					zap.Error(callerErr),
				)
				return nil, callerErr
			}

			e.logger.Warn("retrying",
				zap.String("executor", e.name),
				zap.Int("attempt", i+1),
				zap.Int("max_attempts", maxAttempts),
				zap.Error(callerErr),
			)

			time.Sleep(time.Duration(i) * time.Second)
		}
		return nil, callerErr
	})

	err := callerErr
	if cbErr != nil && err == nil {
		err = cbErr
	}

	if err != nil && !isClientCancellation(err) {
		e.logger.Error("circuit breaker error",
			zap.String("executor", e.name),
			zap.Error(err),
		)
	}
	return err
}

func isNonRetryable(err error) bool {
	// Duplicate key error
	if utils.IsDuplicateKeyError(err) {
		return true
	}

	// Context cancelled by caller (not timeout) — don't retry
	if errors.Is(err, context.Canceled) {
		return true
	}

	return false
}

func isClientCancellation(err error) bool {
	return errors.Is(err, context.Canceled)
}
