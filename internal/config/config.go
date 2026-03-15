package config

import (
	"fmt"
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
}

func LoadConfig() (*Config, error) {
	fmt.Println("CLOUDFRONT FROM ENV:", os.Getenv("CLOUDFRONT_DOMAIN"))
	_ = godotenv.Load()
	fmt.Println("CLOUDFRONT AFTER LOAD:", os.Getenv("CLOUDFRONT_DOMAIN"))

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
	}

	return cfg, nil
}
