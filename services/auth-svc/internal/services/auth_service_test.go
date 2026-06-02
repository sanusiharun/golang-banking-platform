package services_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/auth-svc/internal/repository"
	"github.com/sanusi/banking/services/auth-svc/internal/services"
)

// ── Mock repositories ─────────────────────────────────────────────────────────

type mockUserRepo struct {
	user *dao.User
	err  error
}

func (m *mockUserRepo) FindByUsername(_ context.Context, _ string) (*dao.User, error) {
	return m.user, m.err
}

func (m *mockUserRepo) FindByID(_ context.Context, _ string) (*dao.User, error) {
	return m.user, m.err
}

type mockTokenStore struct {
	saveErr   error
	findToken *dao.RefreshToken
	findErr   error
	revokeErr error
}

func (m *mockTokenStore) Save(_ context.Context, _ *dao.RefreshToken) error { return m.saveErr }
func (m *mockTokenStore) FindByHash(_ context.Context, _ string) (*dao.RefreshToken, error) {
	return m.findToken, m.findErr
}
func (m *mockTokenStore) Revoke(_ context.Context, _ string) error         { return m.revokeErr }
func (m *mockTokenStore) RevokeAllForUser(_ context.Context, _ string) error { return m.revokeErr }

var _ repository.UserRepository = (*mockUserRepo)(nil)
var _ repository.TokenStore = (*mockTokenStore)(nil)

// ── Helpers ───────────────────────────────────────────────────────────────────

func newTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

func hashPassword(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return string(h)
}

func newTestService(t *testing.T, userRepo repository.UserRepository, tokenStore repository.TokenStore) services.AuthService {
	t.Helper()
	key := newTestRSAKey(t)
	return services.NewAuthService(userRepo, tokenStore, services.AuthConfig{
		PrivateKey:      key,
		Issuer:          "test",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
		BCryptCost:      bcrypt.MinCost,
	})
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestLogin_Success(t *testing.T) {
	password := "Secret@123"
	user := &dao.User{
		ID:           "usr-001",
		Username:     "admin",
		PasswordHash: hashPassword(t, password),
		Roles:        dao.StringArray{"ADMIN"},
		IsActive:     true,
	}

	svc := newTestService(t, &mockUserRepo{user: user}, &mockTokenStore{})

	resp, err := svc.Login(context.Background(), &dto.LoginRequest{
		Username: "admin",
		Password: password,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if resp.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	user := &dao.User{
		ID:           "usr-001",
		Username:     "admin",
		PasswordHash: hashPassword(t, "correct-password"),
		IsActive:     true,
	}

	svc := newTestService(t, &mockUserRepo{user: user}, &mockTokenStore{})

	_, err := svc.Login(context.Background(), &dto.LoginRequest{
		Username: "admin",
		Password: "wrong-password",
	})

	if !errors.Is(err, services.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	svc := newTestService(t,
		&mockUserRepo{err: repository.ErrUserNotFound},
		&mockTokenStore{},
	)

	_, err := svc.Login(context.Background(), &dto.LoginRequest{
		Username: "nobody",
		Password: "any",
	})

	if !errors.Is(err, services.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_InactiveUser(t *testing.T) {
	user := &dao.User{
		ID:           "usr-002",
		Username:     "inactive",
		PasswordHash: hashPassword(t, "password"),
		IsActive:     false,
	}

	svc := newTestService(t, &mockUserRepo{user: user}, &mockTokenStore{})

	_, err := svc.Login(context.Background(), &dto.LoginRequest{
		Username: "inactive",
		Password: "password",
	})

	if !errors.Is(err, services.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials for inactive user, got %v", err)
	}
}

func TestLogin_TokenStoreSaveError(t *testing.T) {
	user := &dao.User{
		ID:           "usr-001",
		Username:     "admin",
		PasswordHash: hashPassword(t, "pass"),
		IsActive:     true,
	}

	svc := newTestService(t,
		&mockUserRepo{user: user},
		&mockTokenStore{saveErr: errors.New("db error")},
	)

	_, err := svc.Login(context.Background(), &dto.LoginRequest{
		Username: "admin",
		Password: "pass",
	})

	if err == nil {
		t.Error("expected error when token store fails, got nil")
	}
}
