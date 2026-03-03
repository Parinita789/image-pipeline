package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"
)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestId := r.Header.Get("X-Idempotency-Key")

		// Generate requestId if missing
		if requestId == "" {
			requestId = generateFallbackId(r)
		}

		// Put in context
		ctx := context.WithValue(r.Context(), RequestIdKey, requestId)

		// Add to response Header
		w.Header().Set("X-Request-ID", requestId)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func generateFallbackId(r *http.Request) string {

	data := r.RemoteAddr +
		r.Method +
		r.URL.Path +
		time.Now().Format("2026-01-02-15") // hourly bucket

	hash := sha256.Sum256([]byte(data))

	return hex.EncodeToString(hash[:])
}
