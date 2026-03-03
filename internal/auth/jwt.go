package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var secret = []byte("super-secret")

func GenerateJWT(userId string) (string, error) {
	claims := jwt.MapClaims{
		"user_id": userId,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}
