package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"image-pipeline/internal/models"
	"image-pipeline/internal/queue"
	"image-pipeline/internal/repository"
	"image-pipeline/internal/s3"
	"image-pipeline/internal/services"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"go.uber.org/zap"
)

type Worker struct {
	idemRepo     repository.IdempotencyRepo
	s3           s3.S3Client
	sqs          *queue.SQSClient
	imageService *services.ImageService
	logger       *zap.Logger
	workerCount  int
	jobChan      chan types.Message
	wg           sync.WaitGroup
}

func NewWorker(
	idemRepo repository.IdempotencyRepo,
	sqs *queue.SQSClient,
	imageService *services.ImageService,
	logger *zap.Logger,
	workerCount int,
) *Worker {
	return &Worker{
		idemRepo:     idemRepo,
		sqs:          sqs,
		imageService: imageService,
		logger:       logger,
		workerCount:  workerCount,
		jobChan:      make(chan types.Message, 100),
	}
}

func (w *Worker) StartWorker(ctx context.Context) {
	// start worker
	for i := 0; i < w.workerCount; i++ {
		w.wg.Add(1)
		go func(id int) {
			defer w.wg.Done()
			w.workerLoop(ctx, id)
		}(i)
	}

	// polling loop feeds jobChan
	w.pollSQS(ctx)
	close(w.jobChan)
}

func (w *Worker) pollSQS(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("polling stopped")
			return
		default:
			msgs, err := w.sqs.ReceiveMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				w.logger.Error("sqs receive failed", zap.Error(err))
				time.Sleep(2 * time.Second)
				continue
			}

			for _, msg := range msgs {
				select {
				case w.jobChan <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

func (w *Worker) workerLoop(ctx context.Context, id int) {
	w.logger.Info("worker started", zap.Int("worker_id", id))

	for msg := range w.jobChan {
		var payload models.UploadMessage

		err := json.Unmarshal([]byte(*msg.Body), &payload)
		if err != nil {
			w.logger.Error("Invalid message", zap.Error(err))
		}

		jobLog := w.logger.With(
			zap.String("idempotency_key", payload.IdempotencyKey),
			zap.String("worker_id", fmt.Sprintf("%d", id)),
			zap.String("file", payload.FileName),
		)

		jobCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)

		switch payload.Action {
		case models.ActionTransform:
			err = w.imageService.ProcessTransform(jobCtx, payload)
		default:
			// "compress" or empty (backwards compat)
			err = w.imageService.ProcessUpload(jobCtx, payload)
		}
		cancel()

		if err != nil {
			jobLog.Error("job failed", zap.Error(err))
			continue
		}

		err = w.sqs.DeleteMessage(ctx, *msg.ReceiptHandle)
		if err != nil {
			jobLog.Error("delete message failed", zap.Error(err))
		}
	}
}

func (w *Worker) Wait() {
	w.wg.Wait()
}
