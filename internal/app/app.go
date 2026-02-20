package app

import (
	"net/http"

	"image-pipeline/internal/handlers"
	"image-pipeline/internal/logger"
	"image-pipeline/internal/repository"
	"image-pipeline/internal/resilence"
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

	// Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Error("Failed to load config", zap.Error(err))
	}

	// Connect MongoDB
	db, _ := repository.Connect(cfg.MongoURI)
	if err != nil {
		log.Fatal("Mongo connection failed", zap.Error(err))
	}

	// Create S3 Client
	s3Client, _ := s3client.NewS3Client(cfg.AWSRegion, cfg.S3Bucket)
	if err != nil {
		log.Fatal("S3 connection failed", zap.Error(err))
	}

	// Create Resilience Executors
	mongoExec := resilence.NewExecutor(log, "mongo")
	s3Exec := resilence.NewExecutor(log, "s3")

	// Repository Layer
	repo := repository.NewImageRepo(db, mongoExec)

	// Service Layer
	uploadService := services.NewUploadService(
		s3Client,
		repo,
		log,
		s3Exec,
	)

	// Handler Layer
	uploadHandler := handlers.NewUploadHandler(uploadService)

	// Router Setup
	router := chi.NewRouter()
	RegisterRoutes(router, uploadHandler)

	return &App{
		router: router,
		logger: log,
	}
}

func (a *App) Run() {
	a.logger.Info("Server running on :8080")
	http.ListenAndServe(":8080", a.router)
}
