package auth

import (
	"context"
	"errors"
	"strings"

	"image-pipeline/internal/logger"
	"image-pipeline/internal/models"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type IUserRepo interface {
	CreateUser(ctx context.Context, user *models.User) (string, error)
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
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
			return "", errors.New("email already exists")
		}
		return "", errors.New("registration failed")
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
	token, _ := GenerateJWT(user.ID.Hex(), user.FirstName, s.jwtSecret)

	return token, nil
}
