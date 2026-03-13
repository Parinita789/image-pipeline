package models

import "time"

type IdempotencyStatus string

const (
	StatusStarted    IdempotencyStatus = "STARTED"
	StatusProcessing IdempotencyStatus = "PROCESSING"
	StatusS3Uploaded IdempotencyStatus = "S3_UPLOADED"
	StatusCompleted  IdempotencyStatus = "COMPLETED"
	StatusFailed     IdempotencyStatus = "FAILED"
)

type IdempotencyRecord struct {
	IdempotencyKey string            `bson:"_id" json:"id"`
	RequestHash    string            `bson:"requestHash" json:"requestHash"`
	Status         IdempotencyStatus `bson:"status" json:"status"`
	Response       interface{}       `bson:"response" json:"response"`
	CreatedAt      time.Time         `bson:"createdAt" json:"createdAt"`
	UpdatedAt      time.Time         `bson:"updatedAt" json:"updatedAt"`
}
