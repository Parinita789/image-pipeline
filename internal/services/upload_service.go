package services

import (
	"bytes"
	"context"
	"image-pipeline/internal/models"
	db "image-pipeline/internal/repository"
	"image-pipeline/internal/resilence"
	s3client "image-pipeline/internal/s3"

	"github.com/disintegration/imaging"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type UploadService struct {
	S3     *s3client.S3Client
	Repo   *db.ImageRepo
	Logger *zap.Logger
	s3Exec resilence.Executor
}

func NewUploadService(s3 *s3client.S3Client, repo *db.ImageRepo, logger *zap.Logger, executor resilence.Executor) *UploadService {
	return &UploadService{
		S3:     s3,
		Repo:   repo,
		Logger: logger,
		s3Exec: executor,
	}
}

func (s *UploadService) ProcessUpload(ctx context.Context, filename string, fileData []byte) (string, string, error) {
	id := uuid.New().String()

	rawKey := "raw/" + id + "_" + filename
	compressedKey := "compressed/" + id + "_" + filename

	var rawUrl string
	// upload raw image to S3 with resilience
	err := s.runS3(ctx, func(ctx context.Context) error {
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
		Filename:      filename,
		OriginalURL:   rawUrl,
		CompressedURL: compressedUrl,
	}
	err = s.Repo.Save(ctx, image)
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
