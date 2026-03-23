package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AWSRegion        string
	S3Bucket         string
	SQSQueueURL      string
	MongoURI         string
	MongoDbName      string
	JWTSecret        string
	Port             string
	WorkerCount      int
	CloudFrontDomain string
	SMTPHost         string
	SMTPPort         string
	SMTPUsername     string
	SMTPPassword     string
	SMTPFromEmail    string
	FrontendURL      string
}

func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		AWSRegion:        os.Getenv("AWS_REGION"),
		S3Bucket:         os.Getenv("S3_BUCKET"),
		SQSQueueURL:      os.Getenv("SQS_QUEUE_URL"),
		MongoURI:         os.Getenv("MONGO_URI"),
		MongoDbName:      os.Getenv("MONGO_DB"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		Port:             os.Getenv("PORT"),
		WorkerCount:      5,
		CloudFrontDomain: os.Getenv("CLOUDFRONT_DOMAIN"),
		SMTPHost:         os.Getenv("SMTP_HOST"),
		SMTPPort:         os.Getenv("SMTP_PORT"),
		SMTPUsername:     os.Getenv("SMTP_USERNAME"),
		SMTPPassword:     os.Getenv("SMTP_PASSWORD"),
		SMTPFromEmail:    os.Getenv("SMTP_FROM_EMAIL"),
		FrontendURL:      os.Getenv("FRONTEND_URL"),
	}

	return cfg, nil
}
