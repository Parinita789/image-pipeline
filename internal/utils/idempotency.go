package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

func GenerateFingerPrint(filename string, data []byte, userId string) string {
	h := sha256.New()
	h.Write([]byte(filename))
	h.Write(data)
	h.Write([]byte(userId))
	return hex.EncodeToString(h.Sum(nil))
}
