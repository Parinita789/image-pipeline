package app

import (
	"image-pipeline/internal/auth"
	"image-pipeline/internal/handlers"
	"image-pipeline/internal/middleware"
	"image-pipeline/internal/repository"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func RegisterRoutes(
	router *chi.Mux,
	auth *auth.AuthHandler,
	imageHandler *handlers.ImageHandler,
	// userHandler *handlers.UserHandler,
	jwtSecret string,
	idemRepo *repository.IdempotencyRepo,
	rateLimiter *middleware.RateLimiter,
) {
	// Global middleware
	router.Use(middleware.RequestID)
	router.Use(rateLimiter.RateLimit)
	router.Use(middleware.Logger)

	// healthcheck api
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Public auth routes
	router.Route("/auth", func(r chi.Router) {
		r.Post("/register", auth.Register)
		r.Post("/login", auth.Login)
	})

	//Protected routes
	router.Group(func(pr chi.Router) {
		pr.Use(auth.JWTAuth(jwtSecret))
		pr.With(middleware.IdempotencyCheck(idemRepo)).Post("/image/upload", imageHandler.Upload)
		pr.Get("/images", imageHandler.GetImages)
		pr.Delete("/image/{id}", imageHandler.DeleteImage)
	})
}
