package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"

	"image-pipeline/internal/logger"
	"image-pipeline/internal/metrics"
	"image-pipeline/internal/models"
	apperr "image-pipeline/pkg/errors"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type IUserRepo interface {
	CreateUser(ctx context.Context, user *models.User) (string, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	GetUserById(ctx context.Context, userId string) (*models.User, error)
	UpdatePassword(ctx context.Context, userId string, hashedPassword string) error
	SetDefaultQuota(ctx context.Context, userId string) error
}

type IPasswordResetRepo interface {
	Create(ctx context.Context, reset *models.PasswordReset) error
	FindValidToken(ctx context.Context, tokenHash string) (*models.PasswordReset, error)
	MarkUsed(ctx context.Context, id primitive.ObjectID) error
	InvalidateAllForUser(ctx context.Context, userID primitive.ObjectID) error
}

type RegisterRequest struct {
	FirstName string
	LastName  string
	Email     string
	Password  string
}

type LoginRequest struct {
	Email    string
	Password string
}

type IEmailService interface {
	SendPasswordResetEmail(toEmail, token string) error
}

type AuthService struct {
	UserRepo     IUserRepo
	ResetRepo    IPasswordResetRepo
	EmailService IEmailService
	jwtSecret    string
}

func NewAuthService(userRepo IUserRepo, resetRepo IPasswordResetRepo, emailSvc IEmailService, secret string) *AuthService {
	return &AuthService{
		UserRepo:     userRepo,
		ResetRepo:    resetRepo,
		EmailService: emailSvc,
		jwtSecret:    secret,
	}
}

func ValidatePasswordPolicy(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("must be at least 8 characters")
	}
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSpecial = true
		}
	}
	var missing []string
	if !hasUpper {
		missing = append(missing, "uppercase letter")
	}
	if !hasLower {
		missing = append(missing, "lowercase letter")
	}
	if !hasDigit {
		missing = append(missing, "digit")
	}
	if !hasSpecial {
		missing = append(missing, "special character")
	}
	if len(missing) > 0 {
		return fmt.Errorf("must contain at least one %s", strings.Join(missing, ", "))
	}
	return nil
}

func (s *AuthService) Register(ctx context.Context, req *RegisterRequest) (string, error) {
	if err := ValidatePasswordPolicy(req.Password); err != nil {
		return "", apperr.ErrWeakPassword.Withf(err.Error())
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
	user := models.User{
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Email:     req.Email,
		Password:  string(hash),
	}
	result, err := s.UserRepo.CreateUser(ctx, &user)
	if err != nil {
		if strings.Contains(err.Error(), "email already exists") {
			metrics.AuthRegistrationsTotal.WithLabelValues("duplicateEmail").Inc()
			return "", apperr.ErrEmailExists
		}
		metrics.AuthRegistrationsTotal.WithLabelValues("failed").Inc()
		return "", apperr.ErrRegistrationFailed
	}

	// Set default storage quota (1 GB)
	if err := s.UserRepo.SetDefaultQuota(ctx, result); err != nil {
		log := logger.FromContext(ctx)
		log.Error("failed to set default quota", zap.String("userId", result), zap.Error(err))
	}

	metrics.AuthRegistrationsTotal.WithLabelValues("success").Inc()
	return result, nil
}

func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (string, error) {
	start := time.Now()
	log := logger.FromContext(ctx)
	user, err := s.UserRepo.GetUserByEmail(ctx, req.Email)
	if err != nil || user == nil {
		metrics.AuthLoginsTotal.WithLabelValues("notNound").Inc()
		return "", apperr.ErrInvalidCredentials
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
	metrics.AuthLoginDurationSeconds.Observe(time.Since(start).Seconds())

	if err != nil {
		log.Info("Failed login attempt", zap.String("email", req.Email))
		metrics.AuthLoginsTotal.WithLabelValues("invalidCredentials").Inc()
		return "", apperr.ErrInvalidCredentials
	}
	token, _ := GenerateJWT(user.ID.Hex(), user.FirstName, s.jwtSecret)
	metrics.AuthLoginsTotal.WithLabelValues("success").Inc()
	return token, nil
}

const resetTokenExpiry = 15 * time.Minute

func (s *AuthService) ForgotPassword(ctx context.Context, email string) (string, error) {
	log := logger.FromContext(ctx)

	user, err := s.UserRepo.GetUserByEmail(ctx, email)
	if err != nil || user == nil {
		log.Info("forgot password for unknown email", zap.String("email", email))
		return "", nil
	}

	_ = s.ResetRepo.InvalidateAllForUser(ctx, user.ID)

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", apperr.ErrInternalServer
	}
	rawToken := hex.EncodeToString(tokenBytes)

	hash := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(hash[:])

	reset := &models.PasswordReset{
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(resetTokenExpiry),
		Used:      false,
	}
	if err := s.ResetRepo.Create(ctx, reset); err != nil {
		log.Error("failed to create password reset", zap.Error(err))
		return "", apperr.ErrInternalServer
	}

	// Send reset email
	if err := s.EmailService.SendPasswordResetEmail(email, rawToken); err != nil {
		log.Error("failed to send reset email", zap.Error(err))
		return "", apperr.ErrEmailSendFailed
	}

	log.Info("password reset email sent", zap.String("userId", user.ID.Hex()))
	return "", nil
}

func (s *AuthService) ResetPassword(ctx context.Context, token string, newPassword string) error {
	log := logger.FromContext(ctx)

	if err := ValidatePasswordPolicy(newPassword); err != nil {
		return apperr.ErrWeakPassword.Withf(err.Error())
	}

	// Hash the incoming token to look up
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	reset, err := s.ResetRepo.FindValidToken(ctx, tokenHash)
	if err != nil || reset == nil {
		return apperr.ErrInvalidResetToken
	}

	// Hash new password
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), 10)
	if err != nil {
		return apperr.ErrInternalServer
	}

	if err := s.UserRepo.UpdatePassword(ctx, reset.UserID.Hex(), string(hashed)); err != nil {
		log.Error("failed to update password", zap.Error(err))
		return apperr.ErrInternalServer
	}

	_ = s.ResetRepo.MarkUsed(ctx, reset.ID)
	// Invalidate all other tokens for this user
	_ = s.ResetRepo.InvalidateAllForUser(ctx, reset.UserID)

	log.Info("password reset successful", zap.String("userId", reset.UserID.Hex()))
	return nil
}

func (s *AuthService) ChangePassword(ctx context.Context, userId string, currentPassword string, newPassword string) error {
	log := logger.FromContext(ctx)

	if err := ValidatePasswordPolicy(newPassword); err != nil {
		return apperr.ErrWeakPassword.Withf(err.Error())
	}

	user, err := s.UserRepo.GetUserById(ctx, userId)
	if err != nil || user == nil {
		return apperr.ErrInternalServer
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(currentPassword)); err != nil {
		return apperr.ErrIncorrectPassword
	}

	// Ensure new password is different
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(newPassword)); err == nil {
		return apperr.ErrSamePassword
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), 10)
	if err != nil {
		return apperr.ErrInternalServer
	}

	if err := s.UserRepo.UpdatePassword(ctx, userId, string(hashed)); err != nil {
		log.Error("failed to change password", zap.Error(err))
		return apperr.ErrInternalServer
	}

	log.Info("password changed", zap.String("userId", userId))
	return nil
}
