package middleware

import (
	"image-pipeline/pkg/response"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

var limiter = rate.NewLimiter(rate.Every(200*time.Millisecond), 5)

func RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			response.Error(w, 429, "Too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}
