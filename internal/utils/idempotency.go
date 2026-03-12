package utils

import (
	"encoding/hex"
	"fmt"
)

func HashBody(filename string, contentLength int, userId string) string {
	hashInput := fmt.Sprintf("%s:%d:%s", filename, contentLength, userId)
	return hex.EncodeToString([]byte(hashInput))
}
