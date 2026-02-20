package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	AWSRegion   string
	S3Bucket    string
	SQSQueueURL string
	MongoURI    string
	MongoDbName string
	Port        string
	WorkerCount int
}

func LoadConfig() (*Config, error) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(" Error loading .env file")
	}

	cfg := &Config{
		AWSRegion:   os.Getenv("AWS_REGION"),
		S3Bucket:    os.Getenv("S3_BUCKET"),
		SQSQueueURL: os.Getenv("SQS_QUEUE_URL"),
		MongoURI:    os.Getenv("MONGO_URI"),
		MongoDbName: os.Getenv("MONGO_DB"),
		Port:        os.Getenv("PORT"),
		WorkerCount: 5,
	}

	return cfg, err
}
