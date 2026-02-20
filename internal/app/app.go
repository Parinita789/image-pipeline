package app

import (
	"net/http"

	"image-pipeline/internal/handlers"
	"image-pipeline/internal/logger"
	"image-pipeline/internal/repository"
	s3client "image-pipeline/internal/s3"
	"image-pipeline/internal/services"
	"image-pipeline/pkg/config"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type App struct {
	router *chi.Mux
	logger *zap.Logger
}

func NewApp() *App {
	log := logger.NewLogger()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Error("Failed to load config", zap.Error(err))
	}

	// create low level deps
	db, _ := repository.Connect(cfg.MongoURI)
	repo := repository.NewImageRepo(db)
	s3, _ := s3client.NewS3Client(cfg.S3Bucket)

	// create services
	uploadService := services.NewUploadService(s3, repo, log)

	// create handlers
	uploadHandler := handlers.NewUploadHandler(uploadService)
	router := chi.NewRouter()
	RegisterRoutes(router, uploadHandler)
	return &App{router: router, logger: log}
}

func (a *App) Run() {
	a.logger.Info("Server running on :8080")
	http.ListenAndServe(":8080", a.router)
}
