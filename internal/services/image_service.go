package services

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image-pipeline/internal/logger"
	"image-pipeline/internal/models"
	"image-pipeline/internal/resilence"
	"image/jpeg"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"

	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/zap"
)

type IIdempotencyRepo interface {
	Get(ctx context.Context, key string) (*models.IdempotencyRecord, error)
	UpdateStatus(ctx context.Context, key string, status models.IdempotencyStatus) error
	Acquire(ctx context.Context, key, hash string) (*models.IdempotencyRecord, bool, error)
}

type IImageRepo interface {
	Save(ctx context.Context, image models.Image) error
	FindRequestById(ctx context.Context, requestId string) (*models.Image, error)
	GetPaginatedImages(ctx context.Context, page, limit int, userId string) ([]models.Image, int64, error)
	DeleteImage(ctx context.Context, id string) (*models.Image, error)
	UpdateImage(ctx context.Context, id string, update bson.M) (*models.Image, error)
}

type IS3Client interface {
	UploadStream(ctx context.Context, key string, body io.Reader) (string, error)
	DownloadStream(ctx context.Context, key string) (io.ReadCloser, error)
	CopyObject(ctx context.Context, srcKey, dstKey string) (string, error)
	DeleteObject(ctx context.Context, key string) error
}

type ISQSClient interface {
	PublishUpload(ctx context.Context, msg models.UploadMessage) error
}

type ImageService struct {
	ImageRepo IImageRepo
	IdemRepo  IIdempotencyRepo
	S3        IS3Client
	s3Exec    resilence.Executor
	sqsQueue  ISQSClient
	sqsExec   resilence.Executor
}

type PaginatedResponse struct {
	Total  int64          `json:"total"`
	Page   int            `json:"page"`
	Limit  int            `json:"limit"`
	Images []models.Image `json:"images"`
}

func NewImageService(
	repo IImageRepo,
	idemRepo IIdempotencyRepo,
	s3 IS3Client,
	s3Exec resilence.Executor,
	sqsQueue ISQSClient,
	sqsExec resilence.Executor,
) *ImageService {
	return &ImageService{
		ImageRepo: repo,
		IdemRepo:  idemRepo,
		S3:        s3,
		s3Exec:    s3Exec,
		sqsQueue:  sqsQueue,
		sqsExec:   sqsExec,
	}
}

func (s *ImageService) EnqueueUpload(
	ctx context.Context,
	requestId string,
	userId string,
	idemKey string,
	filename string,
	body io.Reader,
) error {
	log := logger.FromContext(ctx)
	tempKey := fmt.Sprintf("tmp/%s/%s/%s", userId, requestId, filename)
	log.Info("S3 temp Key", zap.String("s3TempKey", tempKey))

	_, err := s.S3.UploadStream(ctx, tempKey, body)
	if err != nil {
		log.Error("temp upload failed:", zap.Error(err))
		return err
	}

	msg := models.UploadMessage{
		IdempotencyKey: idemKey,
		RequestId:      requestId,
		UserId:         userId,
		FileName:       filename,
		TempS3Key:      tempKey,
	}

	return s.publishToSQS(ctx, msg)
}

func (s *ImageService) publishToSQS(ctx context.Context, msg models.UploadMessage) error {
	return s.sqsExec.Execute(ctx, func(ctx context.Context) error {
		return s.sqsQueue.PublishUpload(ctx, msg)
	})
}

func (s *ImageService) ProcessUpload(
	ctx context.Context,
	msg models.UploadMessage,
) error {
	log := logger.FromContext(ctx)
	idemKey := msg.IdempotencyKey

	log.Info("processing upload",
		zap.String("requestid", msg.RequestId),
		zap.String("idemKey", idemKey),
		zap.String("file", msg.FileName),
	)
	// check idempotency
	record, err := s.IdemRepo.Get(ctx, idemKey)

	if err != nil {
		log.Error("Failed to fetch idempotency record", zap.Error(err))
		return err
	}

	if record != nil && record.Status == models.StatusCompleted {
		log.Info("request already processed", zap.String("idemKey", idemKey))
		return nil
	}

	// Mark as processing
	err = s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusProcessing)
	if err != nil {
		log.Error("failed to mark processing", zap.Error(err))
	}

	// Generate S3 key
	rawKey := fmt.Sprintf(
		"raw/%s/%s_%s",
		msg.UserId,
		idemKey,
		msg.FileName,
	)
	log.Info("s3 raw key", zap.String("s3RawKey", rawKey))

	// Move temp uploaded file to raw location
	rawUrl, err := s.copyInS3(ctx, msg.TempS3Key, rawKey)
	if err != nil {
		s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusFailed)
		log.Error("failed to move temp files to raw", zap.Error(err))
		return err
	}

	rawData, err := s.downloadBytesFromS3(ctx, rawKey)
	if err != nil {
		s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusFailed)
		log.Error("failed to download raw image for compression", zap.Error(err))
		return err
	}

	if err != nil {
		s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusFailed)
		log.Error("failed to read image stream", zap.Error(err))
		return err
	}

	// Compress image
	compressedData, err := s.CompressImage(rawData)
	if err != nil {
		s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusFailed)
		return err
	}

	// Upload compressed image
	compressedKey := fmt.Sprintf(
		"compressed/%s/%s_%s",
		msg.UserId,
		idemKey,
		msg.FileName,
	)
	log.Info("s3Compressedkey", zap.String("compressedKey", compressedKey))

	compressedUrl, err := s.UploadToS3(
		ctx,
		compressedKey,
		compressedData,
		"compressed",
	)
	if err != nil {
		s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusFailed)
		log.Error("compressed upload failed", zap.Error(err))
		return err
	}

	// save metadata
	err = s.SaveMetaData(ctx, idemKey, msg.UserId, msg.FileName, rawUrl, compressedUrl)
	if err != nil {
		s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusFailed)
		log.Error("Error while saving image record in db", zap.Error(err))
		return err
	}

	// Delete temp file — raw key is the permanent location now
	if err := s.S3.DeleteObject(ctx, msg.TempS3Key); err != nil {
		log.Warn("failed to delete temp file", zap.String("key", msg.TempS3Key), zap.Error(err))
	}

	// Mark completed
	err = s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusCompleted)
	if err != nil {
		log.Error("failed to mark completed — requires manual reconciliation", zap.Error(err))
	}

	log.Info("upload pipeline completed",
		zap.String("raw_url", rawUrl),
		zap.String("compressed_url", compressedUrl),
	)

	return nil

}

func (s *ImageService) UploadToS3(
	ctx context.Context,
	key string,
	data []byte,
	imageType string,
) (string, error) {
	log := logger.FromContext(ctx)
	var url string

	err := s.runS3(ctx, func(ctx context.Context) error {
		var err error
		url, err = s.S3.UploadStream(ctx, key, bytes.NewReader(data))
		return err
	})

	if err != nil {
		log.Error(
			"s3_upload_failed",
			zap.String("type", imageType),
			zap.Error(err),
		)
		return "", err
	}

	log.Info(
		"s3_upload_success",
		zap.String("type", imageType),
		zap.String("url", url),
	)

	return url, nil
}

func (s *ImageService) downloadBytesFromS3(ctx context.Context, key string) ([]byte, error) {
	var data []byte

	err := s.s3Exec.Execute(ctx, func(ctx context.Context) error {
		stream, err := s.S3.DownloadStream(ctx, key)
		if err != nil {
			return err
		}
		defer stream.Close()

		data, err = io.ReadAll(stream)
		return err
	})

	return data, err
}

func (s *ImageService) copyInS3(ctx context.Context, src, dst string) (string, error) {
	var url string
	err := s.s3Exec.Execute(ctx, func(ctx context.Context) error {
		var err error
		url, err = s.S3.CopyObject(ctx, src, dst)
		return err
	})
	return url, err
}

func (s *ImageService) CompressImage(data []byte) ([]byte, error) {
	_ = jpeg.Decode
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer

	switch format {

	case "jpeg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{
			Quality: 60,
		})

	case "png":
		encoder := png.Encoder{
			CompressionLevel: png.BestCompression,
		}
		err = encoder.Encode(&buf, img)

	default:
		return nil, fmt.Errorf("unsupported image format: %s", format)
	}

	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (s *ImageService) SaveMetaData(
	ctx context.Context,
	requestID string,
	userID string,
	filename string,
	rawURL string,
	compressedUrl string,
) error {
	log := logger.FromContext(ctx)
	image := models.Image{
		RequestID:     requestID,
		UserID:        userID,
		Filename:      filename,
		OriginalURL:   rawURL,
		CompressedURL: compressedUrl,
	}
	err := s.ImageRepo.Save(ctx, image)

	if err != nil {
		log.Error("image save failed", zap.Error(err))
		return err
	}

	log.Info("image saved in db successfully")

	return nil
}

func (s *ImageService) GetImages(
	ctx context.Context,
	page int,
	limit int,
	userId string,
) (*PaginatedResponse, error) {
	images, total, err := s.ImageRepo.GetPaginatedImages(ctx, page, limit, userId)
	if err != nil {
		return nil, err
	}

	return &PaginatedResponse{
		Total:  total,
		Page:   page,
		Limit:  limit,
		Images: images,
	}, nil
}

func (s *ImageService) DeleteImage(ctx context.Context, id string) error {
	log := logger.FromContext(ctx)
	img, err := s.ImageRepo.DeleteImage(ctx, id)
	if err != nil {
		log.Error("failed to delete image", zap.Error(err))
		return err
	}

	// Delete raw and compressed images from S3
	err = s.s3Exec.Execute(ctx, func(ctx context.Context) error {
		return s.S3.DeleteObject(ctx, img.OriginalURL)
	})

	if err != nil {
		log.Error("failed to delete raw image from S3", zap.Error(err))
		return err
	}

	return s.s3Exec.Execute(ctx, func(ctx context.Context) error {
		return s.S3.DeleteObject(ctx, img.CompressedURL)
	})
}
