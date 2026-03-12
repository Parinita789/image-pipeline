package main

import (
	"context"
	"time"

	"image-pipeline/internal/config"
	"image-pipeline/internal/logger"
	"image-pipeline/internal/queue"
	"image-pipeline/internal/repository"
	"image-pipeline/internal/resilence"
	s3 "image-pipeline/internal/s3"
	"image-pipeline/internal/services"
	"image-pipeline/internal/worker"

	"go.uber.org/zap"
)

func main() {

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
	s3Client, _ := s3.NewS3Client(cfg.AWSRegion, cfg.S3Bucket, logger)
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

	// Worker
	w := worker.NewWorker(
		*idemRepo,
		SQSClient,
		imageService,
		logger,
		cfg.WorkerCount,
	)

	// Start consuming messages
	ctx := context.Background()
	w.StartWorker(ctx)
}
