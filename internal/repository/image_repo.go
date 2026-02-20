package repository

import (
	"context"
	"image-pipeline/internal/models"
	"image-pipeline/internal/resilence"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

type ImageRepo struct {
	Collection *mongo.Collection
	exec       resilence.Executor
}

func NewImageRepo(db *mongo.Database, exec resilence.Executor) *ImageRepo {
	return &ImageRepo{
		Collection: db.Collection("images"),
		exec:       exec,
	}
}

func (r *ImageRepo) Save(ctx context.Context, img models.Image) error {
	return r.runMongo(ctx, func(ctx context.Context) error {
		img.CreatedAt = time.Now()
		img.Status = "compressed"
		_, err := r.Collection.InsertOne(ctx, img)
		return err
	})
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
