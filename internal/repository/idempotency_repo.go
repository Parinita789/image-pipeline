package repository

import (
	"context"
	"image-pipeline/internal/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type IdempotencyRepo struct {
	Collection *mongo.Collection
}

func NewIdemRepo(db *mongo.Database) *IdempotencyRepo {
	return &IdempotencyRepo{
		Collection: db.Collection("idempotency"),
	}
}

func (r *IdempotencyRepo) Get(ctx context.Context, key string) (*models.IdempotencyRecord, error) {
	var record models.IdempotencyRecord

	err := r.Collection.FindOne(ctx, bson.M{"_id": key}).Decode(&record)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &record, err
}

func (r *IdempotencyRepo) Create(ctx context.Context, key string, hash string) error {
	record := models.IdempotencyRecord{
		IdempotencyKey: key,
		RequestHash:    hash,
		Status:         models.StatusProcessing,
		CreatedAt:      time.Now(),
	}

	_, err := r.Collection.InsertOne(ctx, record)
	return err
}

func (r *IdempotencyRepo) UpdateStatus(
	ctx context.Context,
	key string,
	status models.Idempotencytatus,
	// extra bson.M,
) error {

	update := bson.M{
		"$set": bson.M{
			"status": status,
		},
	}

	// if extra != nil {
	// 	update["$set"].(bson.M)["extra"] = extra
	// }

	_, err := r.Collection.UpdateOne(
		ctx,
		bson.M{"_id": key},
		update,
	)

	return err
}
