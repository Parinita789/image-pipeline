package main

import (
	"context"
	"time"

	"image-pipeline/internal/config"
	applogger "image-pipeline/internal/logger"
	"image-pipeline/internal/queue"
	"image-pipeline/internal/repository"
	"image-pipeline/internal/resilence"
	s3 "image-pipeline/internal/s3"
	"image-pipeline/internal/services"
	"image-pipeline/internal/worker"

	"go.uber.org/zap"
)

func main() {

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
	s3Client, _ := s3.NewS3Client(cfg.AWSRegion, cfg.S3Bucket)
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

	// Worker
	w := worker.NewWorker(
		*idemRepo,
		SQSClient,
		imageService,
		log,
		cfg.WorkerCount,
	)

	// Start consuming messages
	ctx := context.Background()
	w.StartWorker(ctx)
}
