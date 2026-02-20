package resilence

import (
	"context"
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
	breaker *gobreaker.CircuitBreaker
}

func NewExecutor(logger *zap.Logger, name string) Executor {
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
	}
}

func (e *executor) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	var err error
	// Circuit breaker execution

	_, err = e.breaker.Execute(func() (interface{}, error) {
		for i := 0; i < 3; i++ {
			runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = fn(runCtx)
			cancel()

			if err == nil {
				return nil, nil
			}

			e.logger.Warn("retrying",
				zap.String("executor", e.name),
				zap.Int("attempt", i+1),
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
