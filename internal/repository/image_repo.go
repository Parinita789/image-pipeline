package repository

import (
	"context"
	"image-pipeline/internal/models"
	"image-pipeline/internal/resilence"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

// func (r *ImageRepo) UpdateProcessedURL(filename, processedURL string) error {
// 	filter := map[string]interface{}{"filename": filename}

// 	update := map[string]interface{}{
// 		"$set": map[string]interface{}{
// 			"processed_url": processedURL,
// 			"status":        "completed",
// 		},
// 	}

// 	_, err := r.Collection.UpdateOne(context.Background(), filter, update)
// 	return err
// }

func (r *ImageRepo) GetPaginatedImages(ctx context.Context, page, limit int, userId string) ([]models.Image, int64, error) {
	var images []models.Image
	var total int64

	err := r.runMongo(ctx, func(ctx context.Context) error {
		filter := bson.M{"userId": userId}

		count, err := r.Collection.CountDocuments(ctx, filter)
		if err != nil {
			return err
		}
		total = count

		opts := options.Find().
			SetSkip(int64((page - 1) * limit)).
			SetLimit(int64(limit)).
			SetSort(bson.M{"createdat": -1})

		cursor, err := r.Collection.Find(ctx, filter, opts)
		if err != nil {
			return err
		}
		return cursor.All(ctx, &images)
	})
	return images, total, err
}

func (r *ImageRepo) CreateIndexes(ctx context.Context) error {
	index := mongo.IndexModel{
		Keys: bson.M{"requestId": 1},
		Options: options.Index().
			SetUnique(true).
			SetName("uniqueRequestId"),
	}
	_, err := r.Collection.Indexes().CreateOne(ctx, index)
	return err
}

func (r *ImageRepo) FindRequestById(ctx context.Context, requestId string) (*models.Image, error) {
	var img models.Image

	err := r.exec.Execute(ctx, func(ctx context.Context) error {
		return r.Collection.
			FindOne(ctx, bson.M{"request_id": requestId}).
			Decode(&img)
	})

	if err == mongo.ErrNilDocument {
		return nil, nil
	}
	return &img, err
}

func (r *ImageRepo) DeleteImage(ctx context.Context, id string) (*models.Image, error) {
	var img models.Image

	err := r.exec.Execute(ctx, func(ctx context.Context) error {
		err := r.Collection.FindOne(ctx, bson.M{"_id": id}).Decode(&img)
		if err != nil {
			return err
		}

		_, err = r.Collection.DeleteOne(ctx, bson.M{"_id": id})
		return err
	})
	return &img, err
}

func (r *ImageRepo) UpdateImage(ctx context.Context, id string, update bson.M) (*models.Image, error) {
	var img models.Image
	err := r.exec.Execute(ctx, func(ctx context.Context) error {
		_, err := r.Collection.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": update})
		if err != nil {
			return err
		}
		return r.Collection.FindOne(ctx, bson.M{"_id": id}).Decode(&img)
	})

	return &img, err
}
