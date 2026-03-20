package repository

import (
	"context"
	"image-pipeline/internal/models"
	"image-pipeline/internal/resilence"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

func (r *ImageRepo) Save(ctx context.Context, image models.Image) error {
	setFields := bson.M{
		"requestId":      image.RequestID,
		"userId":         image.UserID,
		"filename":       image.Filename,
		"originalUrl":    image.OriginalURL,
		"status":         "compressed",
		"compressedUrl":  image.CompressedURL,
		"originalSize":   image.OriginalSize,
		"compressedSize": image.CompressedSize,
	}
	if len(image.Transformations) > 0 {
		setFields["transformations"] = image.Transformations
	}
	if image.TransformedURL != "" {
		setFields["transformedUrl"] = image.TransformedURL
	}
	_, err := r.Collection.UpdateOne(
		ctx,
		bson.M{"requestId": image.RequestID},
		bson.M{
			"$set":         setFields,
			"$setOnInsert": bson.M{"createdAt": time.Now()},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

func (r *ImageRepo) CreateProcessingRecord(ctx context.Context, requestId, userId, filename, rawS3Key string) error {
	_, err := r.Collection.UpdateOne(
		ctx,
		bson.M{"requestId": requestId},
		bson.M{
			"$setOnInsert": bson.M{
				"requestId":   requestId,
				"userId":      userId,
				"filename":    filename,
				"originalUrl": rawS3Key,
				"status":      "processing",
				"createdAt":   time.Now(),
			},
		},
		options.Update().SetUpsert(true),
	)
	return err
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

func (r *ImageRepo) GetPaginatedImages(ctx context.Context, page, limit int, userId string, filters models.ImageFilters) ([]models.Image, int64, error) {
	var images []models.Image
	var total int64

	err := r.runMongo(ctx, func(ctx context.Context) error {
		filter := bson.M{"userId": userId}

		// filename search — case insensitive partial match
		if filters.Search != "" {
			filter["filename"] = bson.M{"$regex": filters.Search, "$options": "i"}
		}

		if filters.Status != "" {
			filter["status"] = filters.Status
		}

		count, err := r.Collection.CountDocuments(ctx, filter)
		if err != nil {
			return err
		}
		total = count

		opts := options.Find().
			SetSkip(int64((page - 1) * limit)).
			SetLimit(int64(limit)).
			SetSort(bson.M{"createdAt": -1})

		cursor, err := r.Collection.Find(ctx, filter, opts)
		if err != nil {
			return err
		}
		return cursor.All(ctx, &images)
	})
	return images, total, err
}

func (r *ImageRepo) FindById(ctx context.Context, id string) (*models.Image, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	var img models.Image
	err = r.Collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&img)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &img, nil
}

func (r *ImageRepo) FindRequestById(ctx context.Context, requestId string) (*models.Image, error) {
	var img models.Image

	err := r.Collection.
		FindOne(ctx, bson.M{"requestId": requestId}).
		Decode(&img)

	if err == mongo.ErrNoDocuments {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &img, err
}

func (r *ImageRepo) DeleteImage(ctx context.Context, id string) (*models.Image, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	var img models.Image

	err = r.exec.Execute(ctx, func(ctx context.Context) error {
		err := r.Collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&img)
		if err == mongo.ErrNoDocuments {
			return mongo.ErrNoDocuments
		}
		if err != nil {
			return err
		}

		_, err = r.Collection.DeleteOne(ctx, bson.M{"_id": objID})
		return err
	})
	return &img, err
}

func (r *ImageRepo) DeleteManyImages(ctx context.Context, ids []string, userId string) ([]models.Image, error) {
	objIDs := make([]primitive.ObjectID, 0, len(ids))
	for _, id := range ids {
		oid, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			continue
		}
		objIDs = append(objIDs, oid)
	}
	if len(objIDs) == 0 {
		return nil, nil
	}

	filter := bson.M{"_id": bson.M{"$in": objIDs}, "userId": userId}

	var deleted []models.Image
	err := r.exec.Execute(ctx, func(ctx context.Context) error {
		cursor, err := r.Collection.Find(ctx, filter)
		if err != nil {
			return err
		}
		if err = cursor.All(ctx, &deleted); err != nil {
			return err
		}
		if len(deleted) == 0 {
			return nil
		}
		_, err = r.Collection.DeleteMany(ctx, filter)
		return err
	})
	return deleted, err
}

func (r *ImageRepo) SumStorageByUser(ctx context.Context, userId string) (int64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"userId": userId}}},
		{{Key: "$group", Value: bson.M{
			"_id":        nil,
			"totalBytes": bson.M{"$sum": bson.M{"$add": []string{"$originalSize", "$compressedSize"}}},
		}}},
	}
	cursor, err := r.Collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var results []struct {
		TotalBytes int64 `bson:"totalBytes"`
	}
	if err = cursor.All(ctx, &results); err != nil {
		return 0, err
	}
	if len(results) == 0 {
		return 0, nil
	}
	return results[0].TotalBytes, nil
}

func (r *ImageRepo) UpdateImage(ctx context.Context, id string, update bson.M) (*models.Image, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	var img models.Image
	err = r.exec.Execute(ctx, func(ctx context.Context) error {
		_, err := r.Collection.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{"$set": update})
		if err != nil {
			return err
		}
		return r.Collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&img)
	})

	return &img, err
}
