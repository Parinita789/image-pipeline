package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ImageFilters struct {
	Search string
	Status string
}

type Image struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID          string             `bson:"userId" json:"userId"`
	RequestID       string             `bson:"requestId" json:"requestId"`
	Filename        string             `bson:"filename" json:"filename"`
	Status          string             `bson:"status" json:"status"`
	OriginalURL     string             `bson:"originalUrl" json:"originalUrl"`
	CompressedURL   string             `bson:"compressedUrl,omitempty" json:"compressedUrl"`
	Transformations []string           `bson:"transformations,omitempty" json:"transformations,omitempty"`
	TransformedURL  string             `bson:"transformedUrl,omitempty" json:"transformedUrl,omitempty"`
	OriginalSize    int64              `bson:"originalSize" json:"originalSize"`
	CompressedSize  int64              `bson:"compressedSize" json:"compressedSize"`
	CreatedAt       time.Time          `bson:"createdAt" json:"createdAt"`
}
