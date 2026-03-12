package auth

import (
	"context"
	"errors"

	"image-pipeline/internal/logger"
	"image-pipeline/internal/models"
	db "image-pipeline/internal/repository"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

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
	UserRepo  *db.UserRepo
	jwtSecret string
}

func NewAuthService(userRepo *db.UserRepo, secret string) *AuthService {
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
		return "", err
	}
	return result, nil
}

func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (string, error) {
	log := logger.FromContext(ctx)
	user, err := s.UserRepo.GetUserByEmail(ctx, req.Email)
	if err != nil || user == nil {
		return "", errors.New("invalid email or password")
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
	if err != nil {
		log.Info("Failed login attempt", zap.String("email", req.Email))
		return "", errors.New("invalid credentials")
	}
	token, _ := GenerateJWT(user.ID.Hex())

	return token, nil
}
