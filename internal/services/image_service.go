package services

import (
	"bytes"
	"context"
	"fmt"
	"image-pipeline/internal/models"
	db "image-pipeline/internal/repository"
	"image-pipeline/internal/resilence"
	s3client "image-pipeline/internal/s3"

	"github.com/disintegration/imaging"
	"go.mongodb.org/mongo-driver/bson"
	"go.uber.org/zap"
)

type ImageService struct {
	ImageRepo *db.ImageRepo
	Logger    *zap.Logger
	S3        *s3client.S3Client
	s3Exec    resilence.Executor
}

type PaginatedResponse struct {
	Total  int64          `json:"total"`
	Page   int            `json:"page"`
	Limit  int            `json:"limit"`
	Images []models.Image `json:"images"`
}

func NewImageService(repo *db.ImageRepo, logger *zap.Logger, s3 *s3client.S3Client, executor resilence.Executor) *ImageService {
	return &ImageService{
		ImageRepo: repo,
		Logger:    logger,
		S3:        s3,
		s3Exec:    executor,
	}
}

func (s *ImageService) ProcessUpload(ctx context.Context, requestId string, filename string, fileData []byte) (string, string, error) {
	// Check if request is already being processed
	existing, err := s.ImageRepo.FindRequestById(ctx, requestId)

	if err != nil {
		s.Logger.Error("failed to find existing request", zap.Error(err))
		return "", "", err
	}

	if existing != nil {
		s.Logger.Info("request already exists", zap.String("request_id", requestId))
		return existing.OriginalURL, existing.CompressedURL, nil
	}

	// id := uuid.New().String()

	rawKey := fmt.Sprintf("raw/%s_%s", requestId, filename)
	compressedKey := fmt.Sprintf("compressed/%s_%s", requestId, filename)

	var rawUrl string
	// upload raw image to S3 with resilience
	err = s.runS3(ctx, func(ctx context.Context) error {
		var err error
		rawUrl, err = s.S3.UploadObject(ctx, rawKey, bytes.NewReader(fileData))
		return err
	})

	if err != nil {
		s.Logger.Error("failed to upload raw image", zap.Error(err))
		return "", "", err
	}

	// compress image and upload to S3
	compressData, err := CompressImage(fileData)
	if err != nil {
		s.Logger.Error("error occurred", zap.Error(err))
		return "", "", err
	}

	var compressedUrl string
	// upload compressed image to S3 with resilience
	err = s.runS3(ctx, func(ctx context.Context) error {
		var err error
		compressedUrl, err = s.S3.UploadObject(ctx, compressedKey, bytes.NewReader(compressData))
		if err != nil {
			s.Logger.Error("failed to upload compressed image", zap.Error(err))
		}
		return err
	})

	if err != nil {
		s.Logger.Error("failed to upload compressed image", zap.Error(err))
		return "", "", err
	}

	// save image metadata to database
	image := models.Image{
		RequestID:     requestId,
		Filename:      filename,
		OriginalURL:   rawUrl,
		CompressedURL: compressedUrl,
	}
	err = s.ImageRepo.Save(ctx, image)
	if err != nil {
		s.Logger.Error("failed to save image metadata in db", zap.Error(err))
		return "", "", err
	}

	return rawUrl, compressedUrl, err
}

func CompressImage(data []byte) ([]byte, error) {

	//Decode image from bytes
	img, err := imaging.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	// Resize image to width of 800px while maintaining aspect ratio
	thumb := imaging.Resize(img, 800, 0, imaging.Lanczos)

	// Encode compressed image to JPEG format
	buf := new(bytes.Buffer)
	err = imaging.Encode(buf, thumb, imaging.JPEG)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (s *ImageService) GetImages(ctx context.Context, page, limit int, userId string) (*PaginatedResponse, error) {
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
	img, err := s.ImageRepo.DeleteImage(ctx, id)
	if err != nil {
		s.Logger.Error("failed to delete image", zap.Error(err))
		return err
	}

	// Delete raw and compressed images from S3
	err = s.s3Exec.Execute(ctx, func(ctx context.Context) error {
		return s.S3.DeleteObject(ctx, img.OriginalURL)
	})

	if err != nil {
		s.Logger.Error("failed to delete raw image from S3", zap.Error(err))
		return err
	}

	return s.s3Exec.Execute(ctx, func(ctx context.Context) error {
		return s.S3.DeleteObject(ctx, img.CompressedURL)
	})
}

func (s *ImageService) UpdateImage(ctx context.Context, id string, fileData []byte, filename string) error {
	old, err := s.ImageRepo.FindRequestById(ctx, id)
	if err != nil {
		s.Logger.Error("failed to find old image metadata", zap.Error(err))
		return err
	}

	rawKey := fmt.Sprintf("raw/%s_%s", id, filename)
	compressedKey := fmt.Sprintf("compressed/%s_%s", id, filename)

	var rawUrl string

	err = s.s3Exec.Execute(ctx, func(ctx context.Context) error {
		var err error
		rawUrl, err = s.S3.UploadObject(ctx, rawKey, bytes.NewReader(fileData))
		return err
	})

	if err != nil {
		s.Logger.Error("failed to upload new raw image", zap.Error(err))
		return err
	}

	compressed, err := CompressImage(fileData)
	if err != nil {
		s.Logger.Error("failed to compress image", zap.Error(err))
		return err
	}
	var compressedUrl string

	err = s.s3Exec.Execute(ctx, func(ctx context.Context) error {
		var err error
		compressedUrl, err = s.S3.UploadObject(ctx, compressedKey, bytes.NewReader(compressed))
		return err
	})

	if err != nil {
		s.Logger.Error("failed to upload new compressed image", zap.Error(err))
		return err
	}

	// Update DB
	_, err = s.ImageRepo.UpdateImage(ctx, id, bson.M{
		"originalUrl":   rawUrl,
		"compressedUrl": compressedUrl,
	})

	if err != nil {
		s.Logger.Error("failed to update image metadata in db", zap.Error(err))
		return err
	}

	// Delete old images from S3
	err = s.s3Exec.Execute(ctx, func(ctx context.Context) error {
		return s.S3.DeleteObject(ctx, old.OriginalURL)
	})

	if err != nil {
		s.Logger.Error("failed to delete old raw image from S3", zap.Error(err))
		return err
	}

	return s.s3Exec.Execute(ctx, func(ctx context.Context) error {
		return s.S3.DeleteObject(ctx, old.CompressedURL)
	})
}
