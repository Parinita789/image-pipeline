package logger

import (
	"context"

	"go.uber.org/zap"
)

type contextKey string

const LoggerKey contextKey = "logger"

var defaultLogger *zap.Logger

func NewLogger() *zap.Logger {
	logger, _ := zap.NewProduction()
	return logger
}

func Init(l *zap.Logger) {
	defaultLogger = l
}

// FromContext — gets logger from context, falls back to default
func FromContext(ctx context.Context) *zap.Logger {
	if l, ok := ctx.Value(LoggerKey).(*zap.Logger); ok && l != nil {
		return l
	}
	return defaultLogger
}

// WithContext — stores logger in context
func WithContext(ctx context.Context, l *zap.Logger) context.Context {
	return context.WithValue(ctx, LoggerKey, l)
}
