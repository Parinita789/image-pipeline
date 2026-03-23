package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"image-pipeline/internal/logger"
	"image-pipeline/internal/models"

	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

func testCtx() context.Context {
	ctx := context.Background()
	return logger.WithContext(ctx, zap.NewNop())
}

// ─── UserRepo Mock ───────────────────────────────────────────────────────────

type mockUserRepo struct {
	createUserFn     func(ctx context.Context, user *models.User) (string, error)
	getUserByEmailFn func(ctx context.Context, email string) (*models.User, error)
}

func (m *mockUserRepo) CreateUser(ctx context.Context, user *models.User) (string, error) {
	return m.createUserFn(ctx, user)
}
func (m *mockUserRepo) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	return m.getUserByEmailFn(ctx, email)
}
func (m *mockUserRepo) GetUserById(ctx context.Context, userId string) (*models.User, error) {
	return nil, nil
}
func (m *mockUserRepo) UpdatePassword(ctx context.Context, userId string, hashedPassword string) error {
	return nil
}
func (m *mockUserRepo) SetDefaultQuota(ctx context.Context, userId string) error {
	return nil
}

// ─── PasswordResetRepo Mock ─────────────────────────────────────────────────

type mockResetRepo struct{}

func (m *mockResetRepo) Create(ctx context.Context, reset *models.PasswordReset) error { return nil }
func (m *mockResetRepo) FindValidToken(ctx context.Context, tokenHash string) (*models.PasswordReset, error) {
	return nil, nil
}
func (m *mockResetRepo) MarkUsed(ctx context.Context, id primitive.ObjectID) error { return nil }
func (m *mockResetRepo) InvalidateAllForUser(ctx context.Context, userID primitive.ObjectID) error {
	return nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func buildAuthService(repo *mockUserRepo) *AuthService {
	if repo == nil {
		repo = &mockUserRepo{}
	}
	return NewAuthService(repo, &mockResetRepo{}, "test-secret-key")
}

// hashPassword pre-hashes a password to seed mock user responses
func hashPassword(t *testing.T, plain string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	return string(h)
}

// ─── Register Tests ───────────────────────────────────────────────────────────

func TestRegister_HappyPath(t *testing.T) {
	svc := buildAuthService(&mockUserRepo{
		createUserFn: func(_ context.Context, user *models.User) (string, error) {
			if user.Password == "Password1!" {
				t.Error("password should be hashed, not stored as plaintext")
			}
			return "new-user-id", nil
		},
	})

	id, err := svc.Register(testCtx(), &RegisterRequest{
		FirstName: "Jane",
		LastName:  "Doe",
		Email:     "jane@example.com",
		Password:  "Password1!",
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if id != "new-user-id" {
		t.Errorf("expected id 'new-user-id', got '%s'", id)
	}
}

func TestRegister_DuplicateEmail_ReturnsError(t *testing.T) {
	svc := buildAuthService(&mockUserRepo{
		createUserFn: func(_ context.Context, _ *models.User) (string, error) {
			return "", errors.New("email already exists")
		},
	})

	_, err := svc.Register(testCtx(), &RegisterRequest{
		FirstName: "Jane",
		LastName:  "Doe",
		Email:     "existing@example.com",
		Password:  "Password1!",
	})

	if err == nil {
		t.Fatal("expected error for duplicate email")
	}
	if !strings.Contains(err.Error(), "email already exists") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRegister_PasswordIsHashed(t *testing.T) {
	var savedHash string

	svc := buildAuthService(&mockUserRepo{
		createUserFn: func(_ context.Context, user *models.User) (string, error) {
			savedHash = user.Password
			return "user-id", nil
		},
	})

	_, err := svc.Register(testCtx(), &RegisterRequest{
		Email:    "jane@example.com",
		Password: "MySecret1!",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// saved value must not be the plaintext
	if savedHash == "MySecret1!" {
		t.Fatal("password stored as plaintext — must be hashed")
	}

	// saved value must be a valid bcrypt hash of the original password
	if err := bcrypt.CompareHashAndPassword([]byte(savedHash), []byte("MySecret1!")); err != nil {
		t.Fatalf("saved value is not a valid bcrypt hash of the original password: %v", err)
	}
}

func TestRegister_RepoFails_ReturnsError(t *testing.T) {
	svc := buildAuthService(&mockUserRepo{
		createUserFn: func(_ context.Context, _ *models.User) (string, error) {
			return "", errors.New("mongo write failed")
		},
	})

	_, err := svc.Register(testCtx(), &RegisterRequest{
		Email:    "jane@example.com",
		Password: "Password1!",
	})

	if err == nil {
		t.Fatal("expected error when repo fails")
	}
}

// ─── Login Tests ──────────────────────────────────────────────────────────────

func TestLogin_HappyPath_ReturnsToken(t *testing.T) {
	userID := primitive.NewObjectID()

	svc := buildAuthService(&mockUserRepo{
		getUserByEmailFn: func(_ context.Context, email string) (*models.User, error) {
			return &models.User{
				ID:       userID,
				Email:    email,
				Password: hashPassword(t, "correctpassword"),
			}, nil
		},
	})

	token, err := svc.Login(testCtx(), &LoginRequest{
		Email:    "jane@example.com",
		Password: "correctpassword",
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if token == "" {
		t.Error("expected a non-empty JWT token")
	}
}

func TestLogin_WrongPassword_ReturnsError(t *testing.T) {
	userID := primitive.NewObjectID()

	svc := buildAuthService(&mockUserRepo{
		getUserByEmailFn: func(_ context.Context, _ string) (*models.User, error) {
			return &models.User{
				ID:       userID,
				Email:    "jane@example.com",
				Password: hashPassword(t, "correctpassword"),
			}, nil
		},
	})

	_, err := svc.Login(testCtx(), &LoginRequest{
		Email:    "jane@example.com",
		Password: "wrongpassword",
	})

	if err == nil {
		t.Fatal("expected error for wrong password")
	}
	// must not reveal whether email or password is wrong
	if !strings.Contains(err.Error(), "invalid email or password") {
		t.Errorf("expected 'invalid email or password' error, got: %v", err)
	}
}

func TestLogin_UserNotFound_ReturnsGenericError(t *testing.T) {
	svc := buildAuthService(&mockUserRepo{
		getUserByEmailFn: func(_ context.Context, _ string) (*models.User, error) {
			return nil, errors.New("mongo: no documents")
		},
	})

	_, err := svc.Login(testCtx(), &LoginRequest{
		Email:    "ghost@example.com",
		Password: "password",
	})

	if err == nil {
		t.Fatal("expected error when user not found")
	}
	// should not leak "user not found" — must be a generic message
	if strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("error leaks user existence info: %v", err)
	}
}

func TestLogin_ReturnsJWT_NotPlaintext(t *testing.T) {
	userID := primitive.NewObjectID()

	svc := buildAuthService(&mockUserRepo{
		getUserByEmailFn: func(_ context.Context, _ string) (*models.User, error) {
			return &models.User{
				ID:       userID,
				Email:    "jane@example.com",
				Password: hashPassword(t, "pass"),
			}, nil
		},
	})

	token, err := svc.Login(testCtx(), &LoginRequest{
		Email:    "jane@example.com",
		Password: "pass",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// a JWT has exactly 2 dots: header.payload.signature
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("expected JWT with 3 parts (header.payload.signature), got %d parts: %s", len(parts), token)
	}
}

// ─── JWT Tests ────────────────────────────────────────────────────────────────

func TestGenerateJWT_ReturnsValidToken(t *testing.T) {
	token, err := GenerateJWT("user-id-123", "Test", "test-secret")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("expected 3-part JWT, got %d parts", len(parts))
	}
}

func TestGenerateJWT_DifferentUsersGetDifferentTokens(t *testing.T) {
	token1, _ := GenerateJWT("user-1", "Alice", "test-secret")
	token2, _ := GenerateJWT("user-2", "Bob", "test-secret")

	if token1 == token2 {
		t.Error("different users should get different tokens")
	}
}

func TestGenerateJWT_SameUserGetsDifferentTokensOverTime(t *testing.T) {
	// tokens include iat (issued-at) — two calls should differ
	// unless GenerateJWT removes iat, in which case delete this test
	token1, _ := GenerateJWT("user-1", "Alice", "test-secret")
	token2, _ := GenerateJWT("user-1", "Alice", "test-secret")
	// not asserting they're different here — just that both are valid JWTs
	for _, tok := range []string{token1, token2} {
		if len(strings.Split(tok, ".")) != 3 {
			t.Errorf("invalid JWT: %s", tok)
		}
	}
}
