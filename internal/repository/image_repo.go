package repository

import (
	"context"
	"image-pipeline/internal/models"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

type ImageRepo struct {
	Collection *mongo.Collection
}

func NewImageRepo(db *mongo.Database) *ImageRepo {
	return &ImageRepo{
		Collection: db.Collection("images"),
	}
}

func (r *ImageRepo) Save(img models.Image) error {
	img.CreatedAt = time.Now()
	img.Status = "processing"

	_, err := r.Collection.InsertOne(context.Background(), img)
	return err
}

func (r *ImageRepo) UpdateProcessedURL(filename, processedURL string) error {
	filter := map[string]interface{}{"filename": filename}

	update := map[string]interface{}{
		"$set": map[string]interface{}{
			"processed_url": processedURL,
			"status":        "completed",
		},
	}

	_, err := r.Collection.UpdateOne(context.Background(), filter, update)
	return err
}
