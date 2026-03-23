package repository

import (
	"context"
	"image-pipeline/internal/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type BatchRepo struct {
	Collection *mongo.Collection
}

func NewBatchRepo(db *mongo.Database) *BatchRepo {
	return &BatchRepo{
		Collection: db.Collection("batches"),
	}
}

func (r *BatchRepo) Create(ctx context.Context, batch models.BatchJob) (string, error) {
	batch.CreatedAt = time.Now()
	batch.Status = "processing"
	result, err := r.Collection.InsertOne(ctx, batch)
	if err != nil {
		return "", err
	}
	return result.InsertedID.(primitive.ObjectID).Hex(), nil
}

func (r *BatchRepo) FindById(ctx context.Context, id string) (*models.BatchJob, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	var batch models.BatchJob
	err = r.Collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&batch)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return &batch, err
}

func (r *BatchRepo) IncrementCompleted(ctx context.Context, batchId string) error {
	objID, err := primitive.ObjectIDFromHex(batchId)
	if err != nil {
		return err
	}
	_, err = r.Collection.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{
		"$inc": bson.M{"completed": 1},
	})
	return err
}

func (r *BatchRepo) IncrementFailed(ctx context.Context, batchId string, imageId string, errMsg string) error {
	objID, err := primitive.ObjectIDFromHex(batchId)
	if err != nil {
		return err
	}
	_, err = r.Collection.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{
		"$inc": bson.M{"failed": 1},
		"$push": bson.M{"errors": models.BatchError{
			ImageId: imageId,
			Error:   errMsg,
		}},
	})
	return err
}

func (r *BatchRepo) Finalize(ctx context.Context, batchId string) error {
	objID, err := primitive.ObjectIDFromHex(batchId)
	if err != nil {
		return err
	}
	batch, err := r.FindById(ctx, batchId)
	if err != nil || batch == nil {
		return err
	}
	status := "completed"
	if batch.Failed > 0 && batch.Completed > 0 {
		status = "partial"
	} else if batch.Failed > 0 && batch.Completed == 0 {
		status = "failed"
	}
	_, err = r.Collection.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{
		"$set": bson.M{"status": status},
	})
	return err
}
