package app

import (
	"image-pipeline/internal/auth"
	"image-pipeline/internal/handlers"
	"image-pipeline/internal/middleware"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)

func RegisterRoutes(
	router *chi.Mux,
	auth *auth.AuthHandler,
	imageHandler *handlers.ImageHandler,
	jwtSecret string,
	rateLimiter *middleware.RateLimiter,
) {
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173"},
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Idempotency-Key", "X-Request-Id"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	router.Use(middleware.RequestID)
	router.Use(rateLimiter.RateLimit)
	router.Use(middleware.Logger)

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	router.Route("/auth", func(r chi.Router) {
		r.Post("/register", auth.Register)
		r.Post("/login", auth.Login)
	})

	router.Group(func(pr chi.Router) {
		pr.Use(auth.JWTAuth(jwtSecret))
		pr.Post("/images/prepare", imageHandler.PrepareUpload)
		pr.Post("/images/confirm", imageHandler.ConfirmUpload)
		pr.Get("/images", imageHandler.GetImages)
		pr.Get("/images/{requestId}", imageHandler.GetImageByRequestId)
		pr.Delete("/image/{id}", imageHandler.DeleteImage)
		pr.Delete("/images", imageHandler.BatchDeleteImages)
	})
}
