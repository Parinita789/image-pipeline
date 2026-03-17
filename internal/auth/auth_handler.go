package auth

import (
	"context"
	"fmt"
	"image-pipeline/internal/dto"
	"image-pipeline/internal/logger"
	"image-pipeline/internal/middleware"

	apperr "image-pipeline/pkg/errors"
	"image-pipeline/pkg/request"
	"image-pipeline/pkg/response"
	"image-pipeline/pkg/validator"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"net/http"
	"strings"
)

type AuthHandler struct {
	authSvc *AuthService
}

func NewAuthHandler(authSvc *AuthService) *AuthHandler {
	return &AuthHandler{
		authSvc: authSvc,
	}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)

	var req RegisterRequest
	var dto dto.CreateUserDTO

	if err := request.DecodeJSON(r, &dto); err != nil {
		response.AppError(w, apperr.ErrInvalidJSON)
		return
	}

	if err := validator.ValidateStruct(dto); err != nil {
		log.Error("Invalid data", zap.Error(err))
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
		response.HandleError(w, err)
		return
	}

	response.Success(w, "user created", userId)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)

	var req LoginRequest
	var dto dto.LoginUserDTO

	if err := request.DecodeJSON(r, &dto); err != nil {
		response.AppError(w, apperr.ErrInvalidJSON)
		return
	}

	if err := validator.ValidateStruct(dto); err != nil {
		log.Error("Invalid login credentials", zap.Error(err))
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	req = LoginRequest{
		Email:    dto.Email,
		Password: dto.Password,
	}

	token, err := h.authSvc.Login(r.Context(), &req)
	if err != nil {
		log.Error("Login failed", zap.Error(err))
		response.HandleError(w, err)
		return
	}

	response.Success(w, "login successful", map[string]string{
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
				response.AppError(w, apperr.ErrMissingToken)
				return
			}
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return []byte(secret), nil
			})

			if err != nil || token == nil || !token.Valid {
				response.AppError(w, apperr.ErrInvalidToken)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				response.AppError(w, apperr.ErrInvalidClaims)
				return
			}

			userId, ok := claims["userId"].(string)
			if !ok {
				response.AppError(w, apperr.ErrMissingUserID)
				return
			}

			// store userId in context
			ctx := context.WithValue(r.Context(), middleware.UserIdKey, userId)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
