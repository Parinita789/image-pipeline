package models

import "time"

type Image struct {
	ID            string    `bson:"_id,omitempty"`
	Filename      string    `bson:"filename"`
	Status        string    `bson:"status"`
	OriginalURL   string    `bson:"original_url"`
	CompressedURL string    `bson:"compressed_url,omitempty"`
	CreatedAt     time.Time `bson:"created_at"`
}
