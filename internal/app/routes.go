package app

import (
	"image-pipeline/internal/handlers"
	"image-pipeline/internal/middleware"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter(uploadHandler *handlers.UploadHandler) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/upload", uploadHandler.Upload)
	http.ListenAndServe(":8080", r)
	return r
}

func RegisterRoutes(router *chi.Mux, uploadHandler *handlers.UploadHandler) {
	router.HandleFunc("/upload", uploadHandler.Upload)
}
