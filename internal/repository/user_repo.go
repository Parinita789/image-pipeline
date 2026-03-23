package repository

import (
	"context"
	"errors"
	"image-pipeline/internal/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type UserRepo struct {
	collection *mongo.Collection
}

func NewUserRepo(db *mongo.Database) *UserRepo {
	return &UserRepo{
		collection: db.Collection("users"),
	}
}

func (r *UserRepo) CreateUser(ctx context.Context, user *models.User) (string, error) {
	var existing models.User
	err := r.collection.FindOne(ctx, map[string]interface{}{"email": user.Email}).Decode(&existing)
	if err == nil {
		return "", errors.New("email already exists")
	}
	user.CreatedAt = time.Now()
	result, err := r.collection.InsertOne(ctx, user)
	if err != nil {
		return "", err
	}
	id := result.InsertedID.(primitive.ObjectID)
	return id.Hex(), nil
}

func (r *UserRepo) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := r.collection.FindOne(ctx, map[string]interface{}{"email": email}).Decode(&user)
	return &user, err
}

func (r *UserRepo) GetUserById(ctx context.Context, userId string) (*models.User, error) {
	objID, err := primitive.ObjectIDFromHex(userId)
	if err != nil {
		return nil, err
	}
	var user models.User
	err = r.collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepo) UpdateStorageUsed(ctx context.Context, userId string, deltaBytes int64) error {
	objID, err := primitive.ObjectIDFromHex(userId)
	if err != nil {
		return err
	}
	_, err = r.collection.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{"$inc": bson.M{"storageUsedBytes": deltaBytes}})
	return err
}

func (r *UserRepo) UpdatePassword(ctx context.Context, userId string, hashedPassword string) error {
	objID, err := primitive.ObjectIDFromHex(userId)
	if err != nil {
		return err
	}
	_, err = r.collection.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{"$set": bson.M{"password": hashedPassword}})
	return err
}

func (r *UserRepo) SetDefaultQuota(ctx context.Context, userId string) error {
	objID, err := primitive.ObjectIDFromHex(userId)
	if err != nil {
		return err
	}
	_, err = r.collection.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{"$set": bson.M{"storageLimitBytes": int64(1 * 1024 * 1024 * 1024)}})
	return err
}
