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

// Register godoc
// @Summary      Register a new user
// @Description  Create a new account with email, name, and password. Password must meet policy: min 8 chars, 1 uppercase, 1 lowercase, 1 digit, 1 special character.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        body  body      docs.RegisterRequest  true  "Registration payload"
// @Success      200   {object}  docs.APIResponse{data=string}  "User ID"
// @Failure      400   {object}  docs.APIResponse  "Validation error or weak password"
// @Failure      409   {object}  docs.APIResponse  "Email already exists"
// @Failure      500   {object}  docs.APIResponse  "Registration failed"
// @Router       /auth/register [post]
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

// Login godoc
// @Summary      Log in
// @Description  Authenticate with email and password, returns a JWT token valid for 24 hours.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        body  body      docs.LoginRequest  true  "Login credentials"
// @Success      200   {object}  docs.APIResponse{data=docs.LoginData}
// @Failure      400   {object}  docs.APIResponse  "Validation error"
// @Failure      401   {object}  docs.APIResponse  "Invalid credentials"
// @Router       /auth/login [post]
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

// ForgotPassword godoc
// @Summary      Request password reset
// @Description  Generates a reset token (15 min expiry). Always returns 200 to prevent email enumeration. In production the token would be emailed.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        body  body      docs.ForgotPasswordRequest  true  "Email address"
// @Success      200   {object}  docs.APIResponse{data=docs.ForgotPasswordData}
// @Failure      400   {object}  docs.APIResponse  "Validation error"
// @Failure      500   {object}  docs.APIResponse  "Internal error"
// @Router       /auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)

	var dto dto.ForgotPasswordDTO
	if err := request.DecodeJSON(r, &dto); err != nil {
		response.AppError(w, apperr.ErrInvalidJSON)
		return
	}
	if err := validator.ValidateStruct(dto); err != nil {
		log.Error("Invalid forgot password request", zap.Error(err))
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err := h.authSvc.ForgotPassword(ctx, dto.Email)
	if err != nil {
		response.HandleError(w, err)
		return
	}
	response.Success(w, "if the email exists, a reset link has been sent", nil)
}

// ResetPassword godoc
// @Summary      Reset password
// @Description  Set a new password using the reset token from forgot-password. New password must meet password policy.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        body  body      docs.ResetPasswordRequest  true  "Token and new password"
// @Success      200   {object}  docs.APIResponse  "Password reset successful"
// @Failure      400   {object}  docs.APIResponse  "Invalid/expired token or weak password"
// @Failure      500   {object}  docs.APIResponse  "Internal error"
// @Router       /auth/reset-password [post]
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)

	var dto dto.ResetPasswordDTO
	if err := request.DecodeJSON(r, &dto); err != nil {
		response.AppError(w, apperr.ErrInvalidJSON)
		return
	}
	if err := validator.ValidateStruct(dto); err != nil {
		log.Error("Invalid reset password request", zap.Error(err))
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.authSvc.ResetPassword(ctx, dto.Token, dto.NewPassword); err != nil {
		response.HandleError(w, err)
		return
	}

	response.Success(w, "password reset successful", nil)
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.FromContext(ctx)

	userId, ok := ctx.Value(middleware.UserIdKey).(string)
	if !ok || userId == "" {
		response.AppError(w, apperr.ErrMissingUserID)
		return
	}

	var dto dto.ChangePasswordDTO
	if err := request.DecodeJSON(r, &dto); err != nil {
		response.AppError(w, apperr.ErrInvalidJSON)
		return
	}
	if err := validator.ValidateStruct(dto); err != nil {
		log.Error("Invalid change password request", zap.Error(err))
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.authSvc.ChangePassword(ctx, userId, dto.CurrentPassword, dto.NewPassword); err != nil {
		response.HandleError(w, err)
		return
	}

	response.Success(w, "password changed successfully", nil)
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
