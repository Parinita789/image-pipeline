package handlers

import (
	"image-pipeline/internal/services"

	"go.uber.org/zap"
)

type UserHandler struct {
	userSvc *services.UserService
	logger  *zap.Logger
}

func NewUserHandler(service *services.UserService, logger *zap.Logger) *UserHandler {
	return &UserHandler{
		userSvc: service,
		logger:  logger,
	}
}
