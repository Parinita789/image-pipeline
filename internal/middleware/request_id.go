package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestId := r.Header.Get("X-Request-Id")
		if requestId == "" {
			requestId = uuid.New().String()
		}
		ctx := context.WithValue(r.Context(), RequestIdKey, requestId)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
