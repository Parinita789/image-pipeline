package services

import (
	"bytes"
	"image-pipeline/internal/models"
	db "image-pipeline/internal/repository"
	s3client "image-pipeline/internal/s3"
	"time"

	"github.com/disintegration/imaging"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type UploadService struct {
	S3     *s3client.S3Client
	Repo   *db.ImageRepo
	Logger *zap.Logger
}

func NewUploadService(s3 *s3client.S3Client, repo *db.ImageRepo, logger *zap.Logger) *UploadService {
	return &UploadService{
		S3:     s3,
		Repo:   repo,
		Logger: logger,
	}
}

func (s *UploadService) ProcessUpload(filename string, fileData []byte) (string, string, error) {
	id := uuid.New().String()

	rawKey := "raw/" + id + "_" + filename
	compressedKey := "compressed/" + id + "_" + filename

	// upload raw image to S3

	rawUrl, err := s.S3.UploadObject(rawKey, bytes.NewReader(fileData))
	if err != nil {
		return "", "", err
	}

	// compress image and upload to S3
	compressData, err := CompressImage(fileData)
	if err != nil {
		s.Logger.Error("error occurred", zap.Error(err))
		return "", "", err
	}

	// upload compressed image to S3
	compressedUrl, err := s.S3.UploadObject(compressedKey, bytes.NewReader(compressData))
	if err != nil {
		return "", "", err
	}

	// save image metadata to database
	image := models.Image{
		Filename:      filename,
		OriginalURL:   rawUrl,
		CompressedURL: compressedUrl,
		Status:        "compressed",
		CreatedAt:     time.Now(),
	}
	err = s.Repo.Save(image)
	if err != nil {
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
