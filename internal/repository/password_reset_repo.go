package repository

import (
	"context"
	"image-pipeline/internal/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type PasswordResetRepo struct {
	collection *mongo.Collection
}

func NewPasswordResetRepo(db *mongo.Database) *PasswordResetRepo {
	col := db.Collection("passwordResets")

	// TTL index: auto-delete expired docs after 1 hour past expiresAt
	col.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys:    bson.D{{Key: "expiresAt", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(3600),
	})

	return &PasswordResetRepo{collection: col}
}

func (r *PasswordResetRepo) Create(ctx context.Context, reset *models.PasswordReset) error {
	reset.CreatedAt = time.Now()
	_, err := r.collection.InsertOne(ctx, reset)
	return err
}

func (r *PasswordResetRepo) FindValidToken(ctx context.Context, tokenHash string) (*models.PasswordReset, error) {
	var reset models.PasswordReset
	err := r.collection.FindOne(ctx, bson.M{
		"tokenHash": tokenHash,
		"used":      false,
		"expiresAt": bson.M{"$gt": time.Now()},
	}).Decode(&reset)
	if err != nil {
		return nil, err
	}
	return &reset, nil
}

func (r *PasswordResetRepo) MarkUsed(ctx context.Context, id primitive.ObjectID) error {
	_, err := r.collection.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"used": true}})
	return err
}

// InvalidateAllForUser marks all pending reset tokens for a user as used
func (r *PasswordResetRepo) InvalidateAllForUser(ctx context.Context, userID primitive.ObjectID) error {
	_, err := r.collection.UpdateMany(ctx, bson.M{
		"userId": userID,
		"used":   false,
	}, bson.M{"$set": bson.M{"used": true}})
	return err
}
