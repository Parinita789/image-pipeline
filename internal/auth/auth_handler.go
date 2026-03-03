package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"image-pipeline/internal/dto"
	"image-pipeline/internal/middleware"

	"image-pipeline/pkg/errors"
	"image-pipeline/pkg/request"
	"image-pipeline/pkg/response"
	"image-pipeline/pkg/validator"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"net/http"
)

type AuthHandler struct {
	authSvc *AuthService
	logger  *zap.Logger
}

func NewAuthHandler(authSvc *AuthService, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		authSvc: authSvc,
		logger:  logger,
	}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	var dto dto.CreateUserDTO

	if err := request.DecodeJSON(r, &dto); err != nil {
		response.Error(w, 400, "Invalid JSON")
		return
	}

	if err := validator.ValidateStruct(dto); err != nil {
		response.Error(w, 400, err.Error())
		return
	}

	req = RegisterRequest{
		FirstName: dto.FirstName,
		LastName:  dto.LastName,
		Email:     dto.Email,
		Password:  dto.Password,
	}

	userId, err := h.authSvc.Register(r.Context(), &req)

	if err != nil {
		appErr := err.(*errors.AppError)
		response.Error(w, appErr.Code, appErr.Message)
		return
	}

	response.Success(w, "User Created", userId)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	var dto dto.LoginUserDTO

	if err := request.DecodeJSON(r, &dto); err != nil {
		response.Error(w, 400, "Invalid JSON")
		return
	}

	if err := validator.ValidateStruct(dto); err != nil {
		response.Error(w, 400, err.Error())
		return
	}

	req = LoginRequest{
		Email:    dto.Email,
		Password: dto.Password,
	}

	token, err := h.authSvc.Login(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), 401)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"token": token,
	})
}

func (h *AuthHandler) JWTAuth(secret string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := r.Header.Get("Authorization")
			if tokenString == "" {
				http.Error(w, "Missing token", http.StatusUnauthorized)
				return
			}
			token, _ := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
				}
				return []byte(secret), nil
			})

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, "Invalid token claims", http.StatusUnauthorized)
				return
			}

			userId, ok := claims["user_id"].(string)
			if !ok {
				http.Error(w, "user_id missing in token", http.StatusUnauthorized)
				return
			}

			// store userId in context
			ctx := context.WithValue(r.Context(), middleware.UserIdKey, userId)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
