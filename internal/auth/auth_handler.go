package auth

import (
	"context"
	"fmt"
	"image-pipeline/internal/dto"
	"image-pipeline/internal/middleware"
	"strings"

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
		response.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if err := validator.ValidateStruct(dto); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
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
		response.Error(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if err := validator.ValidateStruct(dto); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	req = LoginRequest{
		Email:    dto.Email,
		Password: dto.Password,
	}

	token, err := h.authSvc.Login(r.Context(), &req)
	if err != nil {
		response.Error(w, http.StatusUnauthorized, err.Error())
		return
	}

	response.Success(w, "User Logged In Successfully!", map[string]string{
		"token": token,
	})
}

func (h *AuthHandler) JWTAuth(secret string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := r.Header.Get("Authorization")
			tokenString = strings.TrimPrefix(tokenString, "Bearer ")
			tokenString = strings.TrimSpace(tokenString)

			if tokenString == "" {
				response.Error(w, http.StatusUnauthorized, "Missing token")
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
				response.Error(w, http.StatusUnauthorized, "Invalid token claims")
				return
			}

			userId, ok := claims["userId"].(string)
			if !ok {
				response.Error(w, http.StatusUnauthorized, "userId missing in token")
				return
			}

			// store userId in context
			ctx := context.WithValue(r.Context(), middleware.UserIdKey, userId)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
