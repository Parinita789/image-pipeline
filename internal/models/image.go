package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ImageStatus string

const (
	ImageStatusProcessing  ImageStatus = "processing"
	ImageStatusCompressed  ImageStatus = "compressed"
	ImageStatusFailed      ImageStatus = "failed"
)

// Processing step names
const (
	StepUploaded    ImageStatus = "uploaded"
	StepCompressed  ImageStatus = "compressed"
	StepTransformed ImageStatus = "transformed"
	StepReverted    ImageStatus = "reverted"
)

type ImageFilters struct {
	Search string
	Status string
}

type TransformConfig struct {
	Type     string `bson:"type" json:"type"`
	Width    int    `bson:"width,omitempty" json:"width,omitempty"`
	Height   int    `bson:"height,omitempty" json:"height,omitempty"`
	X        int    `bson:"x,omitempty" json:"x,omitempty"`
	Y        int    `bson:"y,omitempty" json:"y,omitempty"`
	Text     string `bson:"text,omitempty" json:"text,omitempty"`
	Format   string `bson:"format,omitempty" json:"format,omitempty"`
	Position string `bson:"position,omitempty" json:"position,omitempty"`
	LogoURL  string `bson:"logoUrl,omitempty" json:"logoUrl,omitempty"`
}

type ProcessingStep struct {
	Step       ImageStatus `bson:"step" json:"step"`
	SizeBytes  int64       `bson:"sizeBytes" json:"sizeBytes"`
	DurationMs int64       `bson:"durationMs" json:"durationMs"`
	Detail     string      `bson:"detail,omitempty" json:"detail,omitempty"`
	Timestamp  time.Time   `bson:"timestamp" json:"timestamp"`
}

type Image struct {
	ID                primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID            string             `bson:"userId" json:"userId"`
	RequestID         string             `bson:"requestId" json:"requestId"`
	Filename          string             `bson:"filename" json:"filename"`
	Status            ImageStatus        `bson:"status" json:"status"`
	OriginalURL       string             `bson:"originalUrl" json:"originalUrl"`
	CompressedURL     string             `bson:"compressedUrl,omitempty" json:"compressedUrl"`
	Transformations   []TransformConfig  `bson:"transformations,omitempty" json:"transformations,omitempty"`
	TransformedURL    string             `bson:"transformedUrl,omitempty" json:"transformedUrl,omitempty"`
	OriginalSize      int64              `bson:"originalSize" json:"originalSize"`
	CompressedSize    int64              `bson:"compressedSize" json:"compressedSize"`
	ProcessingHistory []ProcessingStep   `bson:"processingHistory,omitempty" json:"processingHistory,omitempty"`
	CreatedAt         time.Time          `bson:"createdAt" json:"createdAt"`
}
