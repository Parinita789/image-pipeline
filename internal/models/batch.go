package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type BatchJob struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID          string             `bson:"userId" json:"userId"`
	Type            string             `bson:"type" json:"type"` // "transform" or "revert"
	ImageIds        []string           `bson:"imageIds" json:"imageIds"`
	Transformations []TransformConfig  `bson:"transformations,omitempty" json:"transformations,omitempty"`
	Total           int                `bson:"total" json:"total"`
	Completed       int                `bson:"completed" json:"completed"`
	Failed          int                `bson:"failed" json:"failed"`
	Errors          []BatchError       `bson:"errors,omitempty" json:"errors,omitempty"`
	Status          string             `bson:"status" json:"status"` // "processing", "completed", "partial"
	CreatedAt       time.Time          `bson:"createdAt" json:"createdAt"`
}

type BatchError struct {
	ImageId string `bson:"imageId" json:"imageId"`
	Error   string `bson:"error" json:"error"`
}
