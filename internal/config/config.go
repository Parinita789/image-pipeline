package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AWSRegion   string
	S3Bucket    string
	SQSQueueURL string
	MongoURI    string
	MongoDbName string
	JWTSecret   string
	Port        string
	WorkerCount int
}

func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		AWSRegion:   os.Getenv("AWS_REGION"),
		S3Bucket:    os.Getenv("S3_BUCKET"),
		SQSQueueURL: os.Getenv("SQS_QUEUE_URL"),
		MongoURI:    os.Getenv("MONGO_URI"),
		MongoDbName: os.Getenv("MONGO_DB"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Port:        os.Getenv("PORT"),
		WorkerCount: 5,
	}

	return cfg, nil
}
