package middleware

import (
	"context"
	"image-pipeline/internal/logger"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestId := r.Header.Get("X-Request-Id")
		if requestId == "" {
			requestId = uuid.New().String()
		}
		w.Header().Set("X-Request-ID", requestId)
		log := logger.FromContext(r.Context()).With(
			zap.String("requestId", requestId),
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
		)
		ctx := context.WithValue(r.Context(), RequestIdKey, requestId)
		ctx = logger.WithContext(ctx, log)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
