package middleware

import (
	"image-pipeline/pkg/response"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Store a limiter per client
type RateLimiter struct {
	limiters sync.Map
	rate     rate.Limit
	burst    int
}

func NewRateLimiter(r rate.Limit, burst int) *RateLimiter {
	rl := &RateLimiter{rate: r, burst: burst}

	//Clean up old limiters every 5 miutes
	go func() {
		for range time.Tick(5 * time.Minute) {
			rl.limiters.Range(func(key, _ any) bool {
				rl.limiters.Delete(key)
				return true
			})
		}
	}()

	return rl
}

func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	limiter, _ := rl.limiters.LoadOrStore(key, rate.NewLimiter(rl.rate, rl.burst))
	return limiter.(*rate.Limiter)
}

func (rl *RateLimiter) RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := GetUserID(r)
		if key == "" {
			key = r.RemoteAddr
		}

		if !rl.getLimiter(key).Allow() {
			response.Error(w, http.StatusMethodNotAllowed, "Too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}
