package s3client

import (
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Client struct {
	S3     *s3.Client
	Bucket string
}

func NewS3Client(bucket string) (*S3Client, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())

	if err != nil {
		return nil, err
	}

	return &S3Client{
		S3:     s3.NewFromConfig(cfg),
		Bucket: bucket,
	}, nil
}

func New(region string, bucket string) (*S3Client, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())

	if err != nil {
		return nil, err
	}

	return &S3Client{
		S3:     s3.NewFromConfig(cfg),
		Bucket: bucket,
	}, nil
}

func (c *S3Client) UploadObject(key string, body io.Reader) (string, error) {
	_, err := c.S3.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: &c.Bucket,
		Key:    &key,
		Body:   body,
	})

	if err != nil {
		return "", err
	}
	url := "https://" + c.Bucket + ".s3.amazonaws.com/" + key
	println("Uploaded to S3 at URL:", url)
	return url, nil
}

func (c *S3Client) GetObject(key string) (io.ReadCloser, error) {
	out, err := c.S3.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: &c.Bucket,
		Key:    &key,
	})

	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (c *S3Client) DeleteObject(key string) error {
	_, err := c.S3.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: &c.Bucket,
		Key:    &key,
	})

	return err
}

func (c *S3Client) UploadBytes(key string, data io.Reader) error {
	_, err := c.UploadObject(key, data)
	return err
}

func (c *S3Client) URL(key string) string {
	return "https://" + c.Bucket + ".s3.amazonaws.com/" + key
}
