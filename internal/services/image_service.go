package services

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image-pipeline/internal/logger"
	"image-pipeline/internal/metrics"
	"image-pipeline/internal/models"
	"image-pipeline/internal/resilence"
	apperr "image-pipeline/pkg/errors"
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

type IIdempotencyRepo interface {
	Get(ctx context.Context, key string) (*models.IdempotencyRecord, error)
	UpdateStatus(ctx context.Context, key string, status models.IdempotencyStatus) error
	Acquire(ctx context.Context, key, hash string) (*models.IdempotencyRecord, bool, error)
}

type IImageRepo interface {
	Save(ctx context.Context, image models.Image) error
	FindById(ctx context.Context, id string) (*models.Image, error)
	FindRequestById(ctx context.Context, requestId string) (*models.Image, error)
	GetPaginatedImages(ctx context.Context, page, limit int, userId string, filters models.ImageFilters) ([]models.Image, int64, error)
	DeleteImage(ctx context.Context, id string) (*models.Image, error)
	DeleteManyImages(ctx context.Context, ids []string, userId string) ([]models.Image, error)
	UpdateImage(ctx context.Context, id string, update bson.M) (*models.Image, error)
	SumStorageByUser(ctx context.Context, userId string) (int64, error)
	CreateProcessingRecord(ctx context.Context, requestId, userId, filename, rawS3Key string) error
}

type IUserRepo interface {
	GetUserById(ctx context.Context, userId string) (*models.User, error)
	UpdateStorageUsed(ctx context.Context, userId string, deltaBytes int64) error
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
	UserRepo  IUserRepo
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
	userRepo IUserRepo,
	s3 IS3Client,
	s3Exec resilence.Executor,
	sqsQueue ISQSClient,
	sqsExec resilence.Executor,
	cdnDomain string,
) *ImageService {
	return &ImageService{
		ImageRepo: repo,
		IdemRepo:  idemRepo,
		UserRepo:  userRepo,
		S3:        s3,
		s3Exec:    s3Exec,
		sqsQueue:  sqsQueue,
		sqsExec:   sqsExec,
		cdnDomain: cdnDomain,
	}
}

func (s *ImageService) PrepareUpload(ctx context.Context, userId string, files []PrepareFile) ([]PreparedUpload, error) {
	log := logger.FromContext(ctx)
	result := make([]PreparedUpload, 0, len(files))
	for _, f := range files {
		requestId := uuid.New().String()
		key := fmt.Sprintf("raw/%s/%s_%s", userId, requestId, f.Filename)
		url, err := s.S3.PresignPutObject(ctx, key, f.ContentType, f.Size, 15*time.Minute)
		if err != nil {
			log.Error("failed to generate presignedUrl", zap.Error(err))
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

func (s *ImageService) CheckStorageQuota(ctx context.Context, userId string, incomingBytes int64) error {
	user, err := s.UserRepo.GetUserById(ctx, userId)
	if err != nil {
		return err
	}
	if user.StorageLimitBytes > 0 && user.StorageUsedBytes+incomingBytes > user.StorageLimitBytes {
		return apperr.ErrStorageQuotaExceeded
	}
	return nil
}

func (s *ImageService) ConfirmUpload(ctx context.Context, userId, idemKey string, files []ConfirmFile) (int, error) {
	log := logger.FromContext(ctx)
	enqueued := 0
	for i, f := range files {
		// Create a "processing" record immediately so the frontend can see it
		if err := s.ImageRepo.CreateProcessingRecord(ctx, f.RequestID, userId, f.Filename, f.Key); err != nil {
			log.Error("failed to create processing record", zap.Error(err))
			// Non-fatal — the worker will create it via Save if this fails
		}

		fileIdemKey := fmt.Sprintf("%s-%d", idemKey, i)
		msg := models.UploadMessage{
			Action:         models.ActionCompress,
			IdempotencyKey: fileIdemKey,
			RequestId:      f.RequestID,
			UserId:         userId,
			FileName:       f.Filename,
			RawS3Key:       f.Key,
		}
		if err := s.publishToSQS(ctx, msg); err != nil {
			log.Error("failed to publish msg to sqs", zap.Error(err))
			metrics.UploadErrorsTotal.WithLabelValues("sqs").Inc()
			return enqueued, err
		}
		enqueued++
		metrics.UploadEnqueuedTotal.Inc()
	}
	return enqueued, nil
}

func (s *ImageService) publishToSQS(ctx context.Context, msg models.UploadMessage) error {
	return s.sqsExec.Execute(ctx, func(ctx context.Context) error {
		return s.sqsQueue.PublishUpload(ctx, msg)
	})
}

func (s *ImageService) ProcessUpload(ctx context.Context, msg models.UploadMessage) error {
	start := time.Now()
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
		metrics.WorkerJobsTotal.WithLabelValues("failed").Inc()
		log.Error("failed to download raw image", zap.Error(err))
		return err
	}

	metrics.ImageSizeBytes.Observe(float64(len(rawData)))

	compressedData, err := s.CompressImage(rawData)
	if err != nil {
		s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusFailed)
		metrics.WorkerJobsTotal.WithLabelValues("failed").Inc()
		return err
	}

	if len(rawData) > 0 {
		ratio := float64(len(compressedData)) / float64(len(rawData))
		metrics.CompressionRatio.Observe(ratio)
	}

	compressedKey := fmt.Sprintf("compressed/%s/%s_%s", msg.UserId, idemKey, msg.FileName)
	log.Info("compressed key", zap.String("compressedKey", compressedKey))

	compressedUrl, err := s.UploadToS3(ctx, compressedKey, compressedData, "compressed")
	if err != nil {
		s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusFailed)
		metrics.WorkerJobsTotal.WithLabelValues("failed").Inc()
		log.Error("compressed upload failed", zap.Error(err))
		return err
	}

	cdnUrl := s.toCDNUrl(compressedUrl)

	originalSize := int64(len(rawData))
	compressedSize := int64(len(compressedData))

	// Apply transforms if requested during initial upload
	var transformedCdnUrl string
	if len(msg.Transformations) > 0 {
		transformedCdnUrl, err = s.applyAndUploadTransforms(ctx, compressedData, msg)
		if err != nil {
			log.Error("failed to apply transforms", zap.Error(err))
		}
	}

	if err = s.SaveMetaData(ctx, idemKey, msg.UserId, msg.FileName, rawUrl, cdnUrl, originalSize, compressedSize, msg.Transformations, transformedCdnUrl); err != nil {
		s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusFailed)
		metrics.WorkerJobsTotal.WithLabelValues("failed").Inc()
		log.Error("failed to save metadata", zap.Error(err))
		return err
	}

	// Update user storage
	if s.UserRepo != nil {
		if err = s.UserRepo.UpdateStorageUsed(ctx, msg.UserId, originalSize+compressedSize); err != nil {
			log.Error("failed to update storage used", zap.Error(err))
		}
	}

	if err = s.IdemRepo.UpdateStatus(ctx, idemKey, models.StatusCompleted); err != nil {
		log.Error("failed to mark completed — requires manual reconciliation", zap.Error(err))
	}

	metrics.WorkerJobsTotal.WithLabelValues("completed").Inc()
	metrics.WorkerJobDurationSeconds.Observe(time.Since(start).Seconds())

	log.Info("upload pipeline completed",
		zap.String("raw_url", rawUrl),
		zap.String("compressed_url", compressedUrl),
	)
	return nil
}

func (s *ImageService) ProcessTransform(ctx context.Context, msg models.UploadMessage) error {
	start := time.Now()
	log := logger.FromContext(ctx)

	log.Info("processing transform",
		zap.String("requestId", msg.RequestId),
		zap.String("idemKey", msg.IdempotencyKey),
		zap.Strings("transforms", msg.Transformations),
	)

	// Check if the image is still in "processing" state — if not, it was cancelled
	img, err := s.ImageRepo.FindRequestById(ctx, msg.RequestId)
	if err != nil || img == nil {
		log.Info("image not found for transform, skipping")
		return nil // return nil so the SQS message gets deleted
	}
	if img.Status != "processing" {
		log.Info("image no longer processing (cancelled?), skipping transform",
			zap.String("status", img.Status))
		return nil // message consumed, no action needed
	}

	// Download the source image (compressed key)
	sourceData, err := s.downloadBytesFromS3(ctx, msg.RawS3Key)
	if err != nil {
		log.Error("failed to download source for transform", zap.Error(err))
		// Revert status
		s.updateImageStatusByRequestId(ctx, msg.RequestId, "compressed")
		metrics.WorkerJobsTotal.WithLabelValues("failed").Inc()
		return err
	}

	// Apply transforms
	transformedCdnUrl, err := s.applyAndUploadTransforms(ctx, sourceData, msg)
	if err != nil {
		log.Error("transform failed", zap.Error(err))
		s.updateImageStatusByRequestId(ctx, msg.RequestId, "compressed")
		metrics.WorkerJobsTotal.WithLabelValues("failed").Inc()
		return err
	}

	// Update the existing image record with transform results
	img, err = s.ImageRepo.FindRequestById(ctx, msg.RequestId)
	if err != nil || img == nil {
		log.Error("failed to find image for transform update", zap.Error(err))
		metrics.WorkerJobsTotal.WithLabelValues("failed").Inc()
		return err
	}

	_, err = s.ImageRepo.UpdateImage(ctx, img.ID.Hex(), bson.M{
		"transformations": msg.Transformations,
		"transformedUrl":  transformedCdnUrl,
		"status":          "compressed",
	})
	if err != nil {
		log.Error("failed to update image with transform results", zap.Error(err))
		metrics.WorkerJobsTotal.WithLabelValues("failed").Inc()
		return err
	}

	metrics.WorkerJobsTotal.WithLabelValues("completed").Inc()
	metrics.WorkerJobDurationSeconds.Observe(time.Since(start).Seconds())

	log.Info("transform job completed",
		zap.String("requestId", msg.RequestId),
		zap.String("transformedUrl", transformedCdnUrl),
	)
	return nil
}

func (s *ImageService) updateImageStatusByRequestId(ctx context.Context, requestId, status string) {
	img, err := s.ImageRepo.FindRequestById(ctx, requestId)
	if err != nil || img == nil {
		return
	}
	s.ImageRepo.UpdateImage(ctx, img.ID.Hex(), bson.M{"status": status})
}

func (s *ImageService) applyAndUploadTransforms(ctx context.Context, compressedData []byte, msg models.UploadMessage) (string, error) {
	img, format, err := image.Decode(bytes.NewReader(compressedData))
	if err != nil {
		return "", fmt.Errorf("decode for transforms: %w", err)
	}

	transformed, err := ApplyTransforms(img, msg.Transformations)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	switch format {
	case "jpeg":
		err = jpeg.Encode(&buf, transformed, &jpeg.Options{Quality: 90})
	case "png":
		err = png.Encode(&buf, transformed)
	default:
		return "", fmt.Errorf("unsupported format for transforms: %s", format)
	}
	if err != nil {
		return "", err
	}

	transformSuffix := strings.Join(msg.Transformations, "-")
	transformedKey := fmt.Sprintf("transformed/%s/%s_%s_%s", msg.UserId, msg.IdempotencyKey, transformSuffix, msg.FileName)
	transformedUrl, err := s.UploadToS3(ctx, transformedKey, buf.Bytes(), "transformed")
	if err != nil {
		return "", err
	}
	return s.toCDNUrl(transformedUrl), nil
}

func (s *ImageService) CancelTransform(ctx context.Context, imageId string, userId string) error {
	log := logger.FromContext(ctx)

	img, err := s.ImageRepo.FindById(ctx, imageId)
	if err != nil {
		return err
	}
	if img == nil {
		return apperr.ErrImageNotFound
	}
	if img.UserID != userId {
		return apperr.ErrImageForbidden
	}
	if img.Status != "processing" {
		return nil // nothing to cancel
	}

	if _, err := s.ImageRepo.UpdateImage(ctx, imageId, bson.M{"status": "compressed"}); err != nil {
		log.Error("failed to cancel transform", zap.Error(err))
		return err
	}

	log.Info("transform cancelled", zap.String("imageId", imageId))
	return nil
}

func (s *ImageService) TransformExistingImage(ctx context.Context, imageId string, userId string, transformations []string) (*models.Image, error) {
	log := logger.FromContext(ctx)

	img, err := s.ImageRepo.FindById(ctx, imageId)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, apperr.ErrImageNotFound
	}
	if img.UserID != userId {
		return nil, apperr.ErrImageForbidden
	}
	if img.Status == "processing" {
		return nil, fmt.Errorf("image is already being processed")
	}

	// Use the compressed S3 key as the source
	compressedKey := extractS3Key(img.CompressedURL)
	if compressedKey == "" {
		compressedKey = extractS3Key(img.OriginalURL)
	}
	if compressedKey == "" {
		return nil, fmt.Errorf("no source image found")
	}

	// Download, transform, upload — all synchronously
	sourceData, err := s.downloadBytesFromS3(ctx, compressedKey)
	if err != nil {
		log.Error("failed to download source for transform", zap.Error(err))
		return nil, err
	}

	msg := models.UploadMessage{
		IdempotencyKey:  fmt.Sprintf("transform-%s-%s", img.ID.Hex(), strings.Join(transformations, "-")),
		RequestId:       img.RequestID,
		UserId:          userId,
		FileName:        img.Filename,
		RawS3Key:        compressedKey,
		Transformations: transformations,
	}

	transformedCdnUrl, err := s.applyAndUploadTransforms(ctx, sourceData, msg)
	if err != nil {
		log.Error("transform failed", zap.Error(err))
		return nil, err
	}

	// Update MongoDB with the result
	updated, err := s.ImageRepo.UpdateImage(ctx, imageId, bson.M{
		"transformations": transformations,
		"transformedUrl":  transformedCdnUrl,
		"status":          "compressed",
	})
	if err != nil {
		log.Error("failed to update image with transform results", zap.Error(err))
		return nil, err
	}

	log.Info("transform completed",
		zap.String("imageId", imageId),
		zap.String("transformedUrl", transformedCdnUrl),
	)
	return updated, nil
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

func (s *ImageService) SaveMetaData(ctx context.Context, requestID, userID, filename, rawURL, compressedUrl string, originalSize, compressedSize int64, transformations []string, transformedUrl string) error {
	log := logger.FromContext(ctx)
	img := models.Image{
		RequestID:       requestID,
		UserID:          userID,
		Filename:        filename,
		OriginalURL:     rawURL,
		CompressedURL:   compressedUrl,
		OriginalSize:    originalSize,
		CompressedSize:  compressedSize,
		Transformations: transformations,
		TransformedURL:  transformedUrl,
	}
	if err := s.ImageRepo.Save(ctx, img); err != nil {
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
		return apperr.ErrImageNotFound
	}

	rawKey := extractS3Key(img.OriginalURL)
	compressedKey := extractS3Key(img.CompressedURL)

	if img.UserID != userId {
		return apperr.ErrImageForbidden
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

	// Decrement user storage
	if s.UserRepo != nil {
		totalSize := img.OriginalSize + img.CompressedSize
		if totalSize > 0 {
			if err = s.UserRepo.UpdateStorageUsed(ctx, userId, -totalSize); err != nil {
				log.Error("failed to decrement storage", zap.Error(err))
			}
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

	var failedKeys []string
	if len(keys) > 0 {
		failedKeys, err = s.S3.DeleteObjects(ctx, keys)
		if err != nil {
			log.Error("s3 batch delete failed", zap.Error(err))
			// MongoDB records are already gone — log and continue; orphaned S3 objects
		}
		if len(failedKeys) > 0 {
			log.Warn("some s3 keys failed to delete", zap.Strings("keys", failedKeys))
		}
	}

	deleted := make([]string, 0, len(imgs))
	var totalSize int64
	for _, img := range imgs {
		deleted = append(deleted, img.ID.Hex())
		totalSize += img.OriginalSize + img.CompressedSize
	}

	// Decrement user storage
	if s.UserRepo != nil && totalSize > 0 && userId != "" {
		if err = s.UserRepo.UpdateStorageUsed(ctx, userId, -totalSize); err != nil {
			log.Error("failed to decrement storage after batch delete", zap.Error(err))
		}
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
