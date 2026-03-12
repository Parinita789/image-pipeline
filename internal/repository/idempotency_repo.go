package repository

import (
	"context"
	"image-pipeline/internal/models"
	"image-pipeline/internal/utils"
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
		UpdatedAt:      time.Now(),
	}

	_, err := r.Collection.InsertOne(ctx, record)
	return err
}

func (r *IdempotencyRepo) UpdateStatus(
	ctx context.Context,
	key string,
	status models.Idempotencytatus,
) error {

	update := bson.M{
		"$set": bson.M{
			"status": status,
		},
	}

	_, err := r.Collection.UpdateOne(
		ctx,
		bson.M{"_id": key},
		update,
	)

	return err
}

func (r *IdempotencyRepo) Acquire(
	ctx context.Context,
	key string,
	hash string,
) (*models.IdempotencyRecord, bool, error) {
	rec, err := r.Get(ctx, key)
	if err != nil {
		return nil, false, err
	}
	if rec != nil {
		return rec, false, nil
	}

	err = r.Create(ctx, key, hash)
	if err != nil {
		if utils.IsDuplicateKeyError(err) {
			rec, err = r.Get(ctx, key)
			return rec, false, err
		}
		return nil, false, err
	}

	rec, err = r.Get(ctx, key)
	if err != nil {
		return nil, false, err
	}
	return rec, true, nil
}
