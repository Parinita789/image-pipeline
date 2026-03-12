package handlers

import (
	"image-pipeline/internal/services"
)

type UserHandler struct {
	userSvc *services.UserService
}

func NewUserHandler(service *services.UserService) *UserHandler {
	return &UserHandler{
		userSvc: service,
	}
}
