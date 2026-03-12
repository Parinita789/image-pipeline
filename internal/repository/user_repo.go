package repository

import (
	"context"
	"errors"
	"image-pipeline/internal/models"
	"time"

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
