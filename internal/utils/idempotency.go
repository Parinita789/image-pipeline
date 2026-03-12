package utils

import (
	"encoding/hex"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
)

func HashBody(filename string, contentLength int, userId string) string {
	hashInput := fmt.Sprintf("%s:%d:%s", filename, contentLength, userId)
	return hex.EncodeToString([]byte(hashInput))
}

func IsDuplicateKeyError(err error) bool {
	var writeErr mongo.WriteException
	if errors.As(err, &writeErr) {
		for _, we := range writeErr.WriteErrors {
			if we.Code == 11000 {
				return true
			}
		}
	}
	return false
}
