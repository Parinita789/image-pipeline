package services

import (
	db "image-pipeline/internal/repository"

	"go.uber.org/zap"
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

type UserService struct {
	UserRepo  *db.UserRepo
	jwtSecret string
	logger    *zap.Logger
}

func NewUserService(userRepo *db.UserRepo, secret string) *UserService {
	return &UserService{
		UserRepo:  userRepo,
		jwtSecret: secret,
	}
}
