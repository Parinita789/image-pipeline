package app

import (
	"image-pipeline/internal/auth"
	"image-pipeline/internal/handlers"
	"image-pipeline/internal/middleware"

	"github.com/go-chi/chi/v5"
)

func RegisterRoutes(
	router *chi.Mux,
	auth *auth.AuthHandler,
	imageHandler *handlers.ImageHandler,
	userHandler *handlers.UserHandler,
	jwtSecret string,
) {
	// Global middleware
	router.Use(middleware.RequestID)
	router.Use(middleware.RateLimit)
	router.Use(middleware.Logger)

	// Public auth routes
	router.Route("/auth", func(r chi.Router) {
		r.Post("/register", auth.Register)
		r.Post("/login", auth.Login)
	})

	//Protected routes
	router.Group(func(pr chi.Router) {
		pr.Use(auth.JWTAuth(jwtSecret))

		pr.Post("/image/upload", imageHandler.Upload)
		pr.Get("/images", imageHandler.GetImages)
		pr.Delete("/image/{id}", imageHandler.DeleteImage)

	})
}
