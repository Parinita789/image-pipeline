package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
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
	defer log.Sync()

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
	s3Client, _ := s3.NewS3Client(cfg.AWSRegion, cfg.S3Bucket)
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

	// cancellable context — cancel stops the SQS polling loop
	ctx, cancel := context.WithCancel(context.Background())

	// start worker pool in background
	go func() {
		log.Info("worker starting", zap.Int("workers", cfg.WorkerCount))
		w.StartWorker(ctx)
	}()

	// block until SIGTERM or SIGINT
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	sig := <-quit
	log.Info("shutdown signal received", zap.String("signal", sig.String()))

	// stop polling — no new jobs picked up after this
	cancel()

	// wait for in-flight jobs to finish
	log.Info("draining in-flight jobs")
	w.Wait()
	log.Info("all jobs drained")

	// close MongoDB after all jobs are done
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := mongoClient.Disconnect(shutdownCtx); err != nil {
		log.Error("mongo disconnect error", zap.Error(err))
	}

	log.Info("worker stopped cleanly")
}
