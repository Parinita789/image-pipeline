package auth

import (
	"context"
	"strings"
	"time"

	"image-pipeline/internal/logger"
	"image-pipeline/internal/metrics"
	"image-pipeline/internal/models"
	apperr "image-pipeline/pkg/errors"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type IUserRepo interface {
	CreateUser(ctx context.Context, user *models.User) (string, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	SetDefaultQuota(ctx context.Context, userId string) error
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

type AuthService struct {
	UserRepo  IUserRepo
	jwtSecret string
}

func NewAuthService(userRepo IUserRepo, secret string) *AuthService {
	return &AuthService{
		UserRepo:  userRepo,
		jwtSecret: secret,
	}
}

func (s *AuthService) Register(ctx context.Context, req *RegisterRequest) (string, error) {
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
