package app

import (
	"context"
	"net/http"
	"time"

	"image-pipeline/internal/auth"
	"image-pipeline/internal/config"
	"image-pipeline/internal/handlers"
	applogger "image-pipeline/internal/logger"
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
	log := applogger.NewLogger()
	applogger.Init(log)
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
	mongoExec := resilence.NewExecutor(log, "mongo", 3, 30*time.Second)
	s3Exec := resilence.NewExecutor(log, "s3", 3, 30*time.Second)
	sqsExec := resilence.NewExecutor(log, "sqs", 3, 30*time.Second)

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
		s3Client,
		s3Exec,
		SQSClient,
		sqsExec,
	)

	authService := auth.NewAuthService(
		userRepo,
		cfg.JWTSecret,
	)

	userService := services.NewUserService(
		userRepo,
		cfg.JWTSecret,
	)

	// Handler Layer
	authHandler := auth.NewAuthHandler(authService)
	imageHandler := handlers.NewImageHandler(imageService)
	userHandler := handlers.NewUserHandler(userService)

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
		logger: log,
	}
}

func (a *App) Run() {
	a.logger.Info("Server running on :8080")
	http.ListenAndServe(":8080", a.router)
}
