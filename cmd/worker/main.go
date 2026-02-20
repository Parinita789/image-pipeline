package main

import (
	"image-pipeline/internal/config"
	"log"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Worker started...")
	log.Println("SQS URL:", cfg.SQSQueueURL)

	select {}
}
