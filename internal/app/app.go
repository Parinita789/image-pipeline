package app

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"image-pipeline/internal/auth"
	"image-pipeline/internal/config"
	"image-pipeline/internal/handlers"
	applogger "image-pipeline/internal/logger"
	"image-pipeline/internal/middleware"
	"image-pipeline/internal/queue"
	"image-pipeline/internal/repository"
	"image-pipeline/internal/resilence"
	s3client "image-pipeline/internal/s3"

	"image-pipeline/internal/services"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type App struct {
	router      *chi.Mux
	logger      *zap.Logger
	mongoClient *mongo.Client
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
	mongoClient, db, err := repository.Connect(cfg.MongoURI)
	if err != nil {
		log.Fatal("Mongo connection failed", zap.Error(err))
	}

	// Create S3 Client
	s3Client, _ := s3client.NewS3Client(cfg.AWSRegion, cfg.S3Bucket)
	if err != nil {
		log.Fatal("S3 connection failed", zap.Error(err))
	}
	// Create SQS client
	SQSClient, _ := queue.NewSQSClient(cfg.SQSQueueURL)
	if err != nil {
		log.Fatal("SQS connection failed", zap.Error(err))
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

	// Service Layer
	imageService := services.NewImageService(
		imageRepo,
		idemRepo,
		s3Client,
		s3Exec,
		SQSClient,
		sqsExec,
		cfg.CloudFrontDomain,
	)

	authService := auth.NewAuthService(
		userRepo,
		cfg.JWTSecret,
	)

	// userService := services.NewUserService(
	// 	userRepo,
	// 	cfg.JWTSecret,
	// )

	// Handler Layer
	authHandler := auth.NewAuthHandler(authService)
	imageHandler := handlers.NewImageHandler(imageService)
	// userHandler := handlers.NewUserHandler(userService)

	// Router Setup
	router := chi.NewRouter()
	RegisterRoutes(
		router,
		authHandler,
		imageHandler,
		// userHandler,
		cfg.JWTSecret,
		idemRepo,
		middleware.NewRateLimiter(rate.Every(200*time.Millisecond), 10), // production
	)

	return &App{
		router:      router,
		logger:      log,
		mongoClient: mongoClient,
	}
}

func (a *App) Run() {
	srv := &http.Server{
		Addr:    ":8080",
		Handler: a.router,
	}

	go func() {
		a.logger.Info("server starting", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Fatal("server error", zap.Error(err))
		}
	}()

	// block until OS signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	sig := <-quit
	a.logger.Info("shutdown signal received", zap.String("signal", sig.String()))

	// in-flight requests 30s to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		a.logger.Error("server forced to shutdown", zap.Error(err))
	}
	a.logger.Info("http server stopped")

	// close MongoDB after requests are drained
	if err := a.mongoClient.Disconnect(ctx); err != nil {
		a.logger.Error("mongo disconnect error", zap.Error(err))
	}

	a.logger.Info("mongo disconnected")

	// flush logger
	a.logger.Info("shutdown complete")
	a.logger.Sync()
}
