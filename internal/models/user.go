package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	ID                primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	FirstName         string             `bson:"firstName" json:"firstName"`
	LastName          string             `bson:"lastName" json:"lastName"`
	Email             string             `bson:"email" json:"email"`
	Password          string             `bson:"password" json:"-"`
	StorageUsedBytes  int64              `bson:"storageUsedBytes" json:"storageUsedBytes"`
	StorageLimitBytes int64              `bson:"storageLimitBytes" json:"storageLimitBytes"`
	CreatedAt         time.Time          `bson:"createdAt" json:"createdAt"`
}

type PasswordReset struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	UserID    primitive.ObjectID `bson:"userId"`
	TokenHash string             `bson:"tokenHash"`
	ExpiresAt time.Time          `bson:"expiresAt"`
	Used      bool               `bson:"used"`
	CreatedAt time.Time          `bson:"createdAt"`
}
