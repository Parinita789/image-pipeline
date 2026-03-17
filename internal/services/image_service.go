package services

import (
	"bytes"
	"context"
	"errors"
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
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/zap"
)

var (
	ErrImageNotFound = errors.New("image not found")
	ErrUnauthorized  = errors.New("unauthorized!")
)

type IIdempotencyRepo interface {
	Get(ctx context.Context, key string) (*models.IdempotencyRecord, error)
	UpdateStatus(ctx context.Context, key string, status models.IdempotencyStatus) error
	Acquire(ctx context.Context, key, hash string) (*models.IdempotencyRecord, bool, error)
}

type IImageRepo interface {
	Save(ctx context.Context, image models.Image) error
	FindRequestById(ctx context.Context, requestId string) (*models.Image, error)
	GetPaginatedImages(ctx context.Context, page, limit int, userId string, filters models.ImageFilters) ([]models.Image, int64, error)
	DeleteImage(ctx context.Context, id string) (*models.Image, error)
	DeleteManyImages(ctx context.Context, ids []string, userId string) ([]models.Image, error)
	UpdateImage(ctx context.Context, id string, update bson.M) (*models.Image, error)
}

type IS3Client interface {
	UploadStream(ctx context.Context, key string, body io.Reader) (string, error)
	DownloadStream(ctx context.Context, key string) (io.ReadCloser, error)
	CopyObject(ctx context.Context, srcKey, dstKey string) (string, error)
	DeleteObject(ctx context.Context, key string) error
	DeleteObjects(ctx context.Context, keys []string) ([]string, error)
	PresignPutObject(ctx context.Context, key, contentType string, size int64, expiry time.Duration) (string, error)
	ObjectURL(key string) string
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
	cdnDomain string
}

type PaginatedResponse struct {
	Total  int64          `json:"total"`
	Page   int            `json:"page"`
	Limit  int            `json:"limit"`
	Images []models.Image `json:"images"`
}

// PrepareFile is the per-file input for PrepareUpload.
type PrepareFile struct {
	Filename    string
	ContentType string
	Size        int64
}

// PreparedUpload is returned to the client — contains the presigned PUT URL.
type PreparedUpload struct {
	Key       string `json:"key"`
	UploadURL string `json:"uploadUrl"`
	Filename  string `json:"filename"`
	RequestID string `json:"requestId"`
}

// ConfirmFile is the per-file input for ConfirmUpload.
type ConfirmFile struct {
	Key       string
	Filename  string
	RequestID string
}

func NewImageService(
	repo IImageRepo,
	idemRepo IIdempotencyRepo,
	s3 IS3Client,
	s3Exec resilence.Executor,
	sqsQueue ISQSClient,
	sqsExec resilence.Executor,
	cdnDomain string,
) *ImageService {
	return &ImageService{
		ImageRepo: repo,
		IdemRepo:  idemRepo,
		S3:        s3,
		s3Exec:    s3Exec,
		sqsQueue:  sqsQueue,
		sqsExec:   sqsExec,
		cdnDomain: cdnDomain,
	}
}

// PrepareUpload generates presigned S3 PUT URLs for each file.
// No file bytes touch the API server — the client uploads directly to S3.
func (s *ImageService) PrepareUpload(ctx context.Context, userId string, files []PrepareFile) ([]PreparedUpload, error) {
	result := make([]PreparedUpload, 0, len(files))
	for _, f := range files {
		requestId := uuid.New().String()
		key := fmt.Sprintf("raw/%s/%s_%s", userId, requestId, f.Filename)
		url, err := s.S3.PresignPutObject(ctx, key, f.ContentType, f.Size, 15*time.Minute)
		if err != nil {
			return nil, fmt.Errorf("presign %s: %w", f.Filename, err)
		}
		result = append(result, PreparedUpload{
			Key:       key,
			UploadURL: url,
			Filename:  f.Filename,
			RequestID: requestId,
		})
	}
	return result, nil
}

// ConfirmUpload is called after the client has PUT all files directly to S3.
// It publishes an SQS message per file so the worker can compress and save metadata.
func (s *ImageService) ConfirmUpload(ctx context.Context, userId, idemKey string, files []ConfirmFile) (int, error) {
	enqueued := 0
	for i, f := range files {
		fileIdemKey := fmt.Sprintf("%s-%d", idemKey, i)
		msg := models.UploadMessage{
			IdempotencyKey: fileIdemKey,
			RequestId:      f.RequestID,
			UserId:         userId,
			FileName:       f.Filename,
			RawS3Key:       f.Key,
		}
		if err := s.publishToSQS(ctx, msg); err != nil {
			return enqueued, err
		}
		enqueued++
	}
	return enqueued, nil
}

func (s *ImageService) publishToSQS(ctx context.Context, msg models.UploadMessage) error {
	return s.sqsExec.Execute(ctx, func(ctx context.Context) error {
		return s.sqsQueue.PublishUpload(ctx, msg)
	})
}

// ProcessUpload is called by the worker. The file is already at msg.RawS3Key —
// no copy step needed. Worker: download → compress → upload compressed → save metadata.
func (s *ImageService) ProcessUpload(ctx context.Context, msg models.UploadMessage) error {
	log := logger.FromContext(ctx)
	idemKey := msg.IdempotencyKey

	log.Info("processing upload",
		zap.String("requestId", msg.RequestId),
		zap.String("idemKey", idemKey),
		zap.String("file", msg.FileName),
	)

	record, err := s.IdemRepo.Get(ctx, idemKey)
	if err != nil {
		log.Error("failed to fetch idempotency record", zap.Error(err))
		return err
	}
	if record != nil && record.Status == models.StatusCompleted {
		log.Info("request already processed", zap.String("idemKey", idemKey))
		return nil
	}

	if err = s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusProcessing); err != nil {
		log.Error("failed to mark processing", zap.Error(err))
	}

	// File is already in S3 at the raw location — client PUT it there directly.
	rawKey := msg.RawS3Key
	rawUrl := s.S3.ObjectURL(rawKey)
	log.Info("raw s3 key", zap.String("rawKey", rawKey))

	rawData, err := s.downloadBytesFromS3(ctx, rawKey)
	if err != nil {
		s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusFailed)
		log.Error("failed to download raw image", zap.Error(err))
		return err
	}

	compressedData, err := s.CompressImage(rawData)
	if err != nil {
		s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusFailed)
		return err
	}

	compressedKey := fmt.Sprintf("compressed/%s/%s_%s", msg.UserId, idemKey, msg.FileName)
	log.Info("compressed key", zap.String("compressedKey", compressedKey))

	compressedUrl, err := s.UploadToS3(ctx, compressedKey, compressedData, "compressed")
	if err != nil {
		s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusFailed)
		log.Error("compressed upload failed", zap.Error(err))
		return err
	}

	cdnUrl := s.toCDNUrl(compressedUrl)

	if err = s.SaveMetaData(ctx, idemKey, msg.UserId, msg.FileName, rawUrl, cdnUrl); err != nil {
		s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusFailed)
		log.Error("failed to save metadata", zap.Error(err))
		return err
	}

	if err = s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusCompleted); err != nil {
		log.Error("failed to mark completed — requires manual reconciliation", zap.Error(err))
	}

	log.Info("upload pipeline completed",
		zap.String("raw_url", rawUrl),
		zap.String("compressed_url", compressedUrl),
	)
	return nil
}

func (s *ImageService) UploadToS3(ctx context.Context, key string, data []byte, imageType string) (string, error) {
	log := logger.FromContext(ctx)
	var url string
	err := s.runS3(ctx, func(ctx context.Context) error {
		var err error
		url, err = s.S3.UploadStream(ctx, key, bytes.NewReader(data))
		return err
	})
	if err != nil {
		log.Error("s3_upload_failed", zap.String("type", imageType), zap.Error(err))
		return "", err
	}
	log.Info("s3_upload_success", zap.String("type", imageType), zap.String("url", url))
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

func (s *ImageService) CompressImage(data []byte) ([]byte, error) {
	_ = jpeg.Decode
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	switch format {
	case "jpeg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 60})
	case "png":
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		err = encoder.Encode(&buf, img)
	default:
		return nil, fmt.Errorf("unsupported image format: %s", format)
	}
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *ImageService) SaveMetaData(ctx context.Context, requestID, userID, filename, rawURL, compressedUrl string) error {
	log := logger.FromContext(ctx)
	image := models.Image{
		RequestID:     requestID,
		UserID:        userID,
		Filename:      filename,
		OriginalURL:   rawURL,
		CompressedURL: compressedUrl,
	}
	if err := s.ImageRepo.Save(ctx, image); err != nil {
		log.Error("image save failed", zap.Error(err))
		return err
	}
	log.Info("image saved in db successfully")
	return nil
}

func (s *ImageService) GetImages(ctx context.Context, page, limit int, userId string, filters models.ImageFilters) (*PaginatedResponse, error) {
	images, total, err := s.ImageRepo.GetPaginatedImages(ctx, page, limit, userId, filters)
	if err != nil {
		return nil, err
	}
	return &PaginatedResponse{Total: total, Page: page, Limit: limit, Images: images}, nil
}

func (s *ImageService) DeleteImage(ctx context.Context, id string, userId string) error {
	log := logger.FromContext(ctx)

	img, err := s.ImageRepo.DeleteImage(ctx, id)
	if err != nil {
		log.Error("failed to delete image", zap.Error(err))
		return err
	}
	if img == nil || img.ID.IsZero() {
		return ErrImageNotFound
	}

	rawKey := extractS3Key(img.OriginalURL)
	compressedKey := extractS3Key(img.CompressedURL)

	if img.UserID != userId {
		return ErrUnauthorized
	}

	if rawKey != "" {
		if err = s.s3Exec.Execute(ctx, func(ctx context.Context) error {
			return s.S3.DeleteObject(ctx, rawKey)
		}); err != nil {
			log.Error("failed to delete raw image from S3", zap.String("key", rawKey), zap.Error(err))
			return err
		}
	}

	if compressedKey != "" {
		if err = s.s3Exec.Execute(ctx, func(ctx context.Context) error {
			return s.S3.DeleteObject(ctx, compressedKey)
		}); err != nil {
			log.Error("failed to delete compressed image from S3", zap.String("key", compressedKey), zap.Error(err))
			return err
		}
	}

	log.Info("image deleted", zap.String("id", id))
	return nil
}

type BatchDeleteResult struct {
	Deleted []string `json:"deleted"`
	Failed  []string `json:"failed"`
}

func (s *ImageService) BatchDeleteImages(ctx context.Context, ids []string, userId string) (*BatchDeleteResult, error) {
	log := logger.FromContext(ctx)

	imgs, err := s.ImageRepo.DeleteManyImages(ctx, ids, userId)
	if err != nil {
		return nil, err
	}

	// Collect all S3 keys from the deleted documents.
	keys := make([]string, 0, len(imgs)*2)
	for _, img := range imgs {
		if k := extractS3Key(img.OriginalURL); k != "" {
			keys = append(keys, k)
		}
		if k := extractS3Key(img.CompressedURL); k != "" {
			keys = append(keys, k)
		}
	}

	// One S3 API call deletes up to 1000 keys.
	var failedKeys []string
	if len(keys) > 0 {
		failedKeys, err = s.S3.DeleteObjects(ctx, keys)
		if err != nil {
			log.Error("s3 batch delete failed", zap.Error(err))
			// MongoDB records are already gone — log and continue; orphaned S3 objects
			// are a cost concern, not a correctness issue.
		}
		if len(failedKeys) > 0 {
			log.Warn("some s3 keys failed to delete", zap.Strings("keys", failedKeys))
		}
	}

	deleted := make([]string, 0, len(imgs))
	for _, img := range imgs {
		deleted = append(deleted, img.ID.Hex())
	}

	return &BatchDeleteResult{Deleted: deleted, Failed: failedKeys}, nil
}

func extractS3Key(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	if idx := strings.Index(rawURL, ".amazonaws.com/"); idx != -1 {
		return rawURL[idx+len(".amazonaws.com/"):]
	}
	if idx := strings.Index(rawURL, ".cloudfront.net/"); idx != -1 {
		return rawURL[idx+len(".cloudfront.net/"):]
	}
	return ""
}

func (s *ImageService) GetImageByRequestId(ctx context.Context, requestId string) (*models.Image, error) {
	return s.ImageRepo.FindRequestById(ctx, requestId)
}

func (s *ImageService) toCDNUrl(s3Url string) string {
	if s.cdnDomain == "" {
		return s3Url
	}
	parts := strings.SplitN(s3Url, ".amazonaws.com/", 2)
	if len(parts) != 2 {
		return s3Url
	}
	return fmt.Sprintf("https://%s/%s", s.cdnDomain, parts[1])
}
