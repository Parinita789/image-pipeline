package s3

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

type S3Client struct {
	S3       *s3.Client
	Uploader *manager.Uploader
	Bucket   string
	Logger   *zap.Logger
}

func NewS3Client(region string, bucket string, logger *zap.Logger) (*S3Client, error) {
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))

	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg)

	uploader := manager.NewUploader(client, func(u *manager.Uploader) {
		u.PartSize = 5 * 1024 * 1024 // 5MB per part
		u.Concurrency = 3
		u.BufferProvider = manager.NewBufferedReadSeekerWriteToPool(25 * 1024 * 1024)
	})

	return &S3Client{
		S3:       client,
		Bucket:   bucket,
		Uploader: uploader,
		Logger:   logger,
	}, nil
}

// UploadStream - streams directly from reader, never loads full file into RAM
func (s *S3Client) UploadStream(
	parentCtx context.Context,
	key string,
	body io.Reader,
) (string, error) {
	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Minute)
	defer cancel()
	_, err := s.Uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
		Body:   body, // io.Reader — streamed, not buffered
	})

	if err != nil {
		return "", fmt.Errorf("s3 stream upload failed: %w", err)
	}

	url := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.Bucket, key)
	return url, nil
}

// DownloadStream - returns a reader, caller streams it (no RAM buffer)
func (s *S3Client) DownloadStream(
	ctx context.Context,
	key string,
) (io.ReadCloser, error) {

	out, err := s.S3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.Bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return nil, fmt.Errorf("s3 download failed: %w", err)
	}

	return out.Body, nil
}

// CopyObject copies an object within S3 — no data leaves AWS, zero bandwidth cost
func (s *S3Client) CopyObject(ctx context.Context, srcKey, dstKey string) (string, error) {

	copySource := url.QueryEscape(s.Bucket + "/" + srcKey)

	_, err := s.S3.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.Bucket),
		CopySource: aws.String(copySource),
		Key:        aws.String(dstKey),
	})

	if err != nil {
		return "", fmt.Errorf("s3 copy failed: %w", err)
	}

	url := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", s.Bucket, dstKey)
	return url, nil
}

func (c *S3Client) DeleteObject(ctx context.Context, key string) error {
	_, err := c.S3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &c.Bucket,
		Key:    &key,
	})

	return err
}
