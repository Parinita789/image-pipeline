package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func GenerateJWT(userId string, secret string) (string, error) {
	claims := jwt.MapClaims{
		"userId": userId,
		"exp":    time.Now().Add(24 * time.Hour).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
