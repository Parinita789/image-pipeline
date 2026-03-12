package resilence

import (
	"context"
	"errors"
	"time"

	"github.com/sony/gobreaker"
	"go.mongodb.org/mongo-driver/mongo"
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
	var err error
	// Circuit breaker execution
	maxAttempts := e.retries + 1
	_, err = e.breaker.Execute(func() (interface{}, error) {
		for i := 0; i < maxAttempts; i++ {
			runCtx, cancel := context.WithTimeout(ctx, e.timeout)
			err = fn(runCtx)
			cancel()

			if err == nil {
				return nil, nil
			}

			if isNonRetryable(err) {
				e.logger.Warn("non-retryable error, stopping",
					zap.String("executor", e.name),
					zap.Error(err),
				)
				return nil, err
			}

			e.logger.Warn("retrying",
				zap.String("executor", e.name),
				zap.Int("attempt", i+1),
				zap.Int("max_attempts", maxAttempts),
				zap.Error(err),
			)

			time.Sleep(time.Duration(i) * time.Second)
		}
		return nil, err
	})

	if err != nil {
		e.logger.Error("circuit breaker error",
			zap.String("executor", e.name),
			zap.Error(err),
		)
	}
	return err
}

func isNonRetryable(err error) bool {
	var writeErr mongo.WriteException
	if errors.As(err, &writeErr) {
		for _, we := range writeErr.WriteErrors {
			if we.Code == 11000 { // duplicate key error code
				return true
			}
		}
	}

	// Context cancelled by caller (not timeout) — don't retry
	if errors.Is(err, context.Canceled) {
		return true
	}

	return false
}
