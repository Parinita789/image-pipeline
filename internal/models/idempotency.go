package models

import "time"

type Idempotencytatus string

const (
	StatusStarted    Idempotencytatus = "STARTED"
	StatusProcessing Idempotencytatus = "PROCESSING"
	StatusS3Uploaded Idempotencytatus = "S3_UPLOADED"
	StatusCompleted  Idempotencytatus = "COMPLETED"
	StatusFailed     Idempotencytatus = "FAILED"
)

type IdempotencyRecord struct {
	IdempotencyKey string           `bson:"_id" json:"id"`
	RequestHash    string           `bson:"requestHash" json:"requestHash"`
	Status         Idempotencytatus `bson:"status" json:"status"`
	Response       interface{}      `bson:"response" json:"response"`
	CreatedAt      time.Time        `bson:"createdAt" json:"createdAt"`
	UpdatedAt      time.Time        `bson:"updatedAt" json:"updatedAt"`
}
