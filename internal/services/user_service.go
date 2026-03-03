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

func NewUserService(userRepo *db.UserRepo, secret string, logger *zap.Logger) *UserService {
	return &UserService{
		UserRepo:  userRepo,
		jwtSecret: secret,
		logger:    logger,
	}
}

// func (s *UserService) Register(ctx context.Context, req *RegisterRequest) (string, error) {
// 	hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), 10)
// 	fmt.Println("\n\n\nrequest >>> ", req)
// 	user := models.User{
// 		FirstName: req.FirstName,
// 		LastName:  req.LastName,
// 		Email:     req.Email,
// 		Password:  string(hash),
// 	}
// 	result, err := s.UserRepo.CreateUser(ctx, &user)
// 	if err != nil {
// 		return "", err
// 	}
// 	fmt.Println("\n\n\nresult >>> ", result)

// 	return result, nil
// }

// func (s *UserService) Login(ctx context.Context, req *LoginRequest) (string, error) {
// 	user, err := s.UserRepo.GetUserByEmail(ctx, req.Email)
// 	if err != nil || user == nil {
// 		return "", errors.New("invalid email or password")
// 	}

// 	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password))
// 	if err != nil {
// 		s.logger.Info("Failed login attempt", zap.String("email", req.Email))
// 		return "", errors.New("invalid credentials")
// 	}

// 	token, _ := helpers.GenerateJWT(user.ID.String())

// 	return token, nil
// }
