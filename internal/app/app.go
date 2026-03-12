package app

import (
	"context"
	"net/http"
	"time"

	"image-pipeline/internal/auth"
	"image-pipeline/internal/config"
	"image-pipeline/internal/handlers"
	"image-pipeline/internal/logger"
	"image-pipeline/internal/queue"
	"image-pipeline/internal/repository"
	"image-pipeline/internal/resilence"
	s3client "image-pipeline/internal/s3"
	"image-pipeline/internal/services"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

type App struct {
	router *chi.Mux
	logger *zap.Logger
}

func NewApp() *App {
	logger := logger.NewLogger()

	// Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("Failed to load config", zap.Error(err))
	}

	// Connect MongoDB
	db, _ := repository.Connect(cfg.MongoURI)
	if err != nil {
		logger.Fatal("Mongo connection failed", zap.Error(err))
	}

	// Create S3 Client
	s3Client, _ := s3client.NewS3Client(cfg.AWSRegion, cfg.S3Bucket, logger)
	if err != nil {
		logger.Fatal("S3 connection failed", zap.Error(err))
	}

	// Create Resilience Executors
	mongoExec := resilence.NewExecutor(logger, "mongo", 3, 30*time.Second)
	s3Exec := resilence.NewExecutor(logger, "s3", 3, 30*time.Second)
	sqsExec := resilence.NewExecutor(logger, "sqs", 3, 30*time.Second)

	// Repository Layer
	imageRepo := repository.NewImageRepo(db, mongoExec)
	imageRepo.CreateIndexes(context.Background())

	idemRepo := repository.NewIdemRepo(db)

	userRepo := repository.NewUserRepo(db)

	// Create SQS client
	SQSClient, _ := queue.NewSQSClient(cfg.SQSQueueURL)
	// Service Layer
	imageService := services.NewImageService(
		imageRepo,
		idemRepo,
		logger,
		s3Client,
		s3Exec,
		SQSClient,
		sqsExec,
	)

	authService := auth.NewAuthService(
		userRepo,
		cfg.JWTSecret,
		logger,
	)

	userService := services.NewUserService(
		userRepo,
		cfg.JWTSecret,
		logger,
	)

	// Handler Layer
	authHandler := auth.NewAuthHandler(authService, logger)
	imageHandler := handlers.NewImageHandler(imageService, logger)
	userHandler := handlers.NewUserHandler(userService, logger)

	// Router Setup
	router := chi.NewRouter()
	RegisterRoutes(
		router,
		authHandler,
		imageHandler,
		userHandler,
		cfg.JWTSecret,
		idemRepo,
	)

	return &App{
		router: router,
		logger: logger,
	}
}

func (a *App) Run() {
	a.logger.Info("Server running on :8080")
	http.ListenAndServe(":8080", a.router)
}
