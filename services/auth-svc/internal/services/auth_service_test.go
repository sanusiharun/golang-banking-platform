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
	// For tracking calls in tests
	findByUsernameCalls int
	findByIDCalls       int
}

func (m *mockUserRepo) FindByUsername(_ context.Context, _ string) (*dao.User, error) {
	m.findByUsernameCalls++
	return m.user, m.err
}

func (m *mockUserRepo) FindByID(_ context.Context, _ string) (*dao.User, error) {
	m.findByIDCalls++
	return m.user, m.err
}

type mockTokenStore struct {
	saveErr       error
	findToken     *dao.RefreshToken
	findErr       error
	revokeErr     error
	saveCalls     int
	findCalls     int
	revokeCalls   int
	revokeAllCalls int
}

func (m *mockTokenStore) Save(_ context.Context, _ *dao.RefreshToken) error {
	m.saveCalls++
	return m.saveErr
}

func (m *mockTokenStore) FindByHash(_ context.Context, _ string) (*dao.RefreshToken, error) {
	m.findCalls++
	return m.findToken, m.findErr
}

func (m *mockTokenStore) Revoke(_ context.Context, _ string) error {
	m.revokeCalls++
	return m.revokeErr
}

func (m *mockTokenStore) RevokeAllForUser(_ context.Context, _ string) error {
	m.revokeAllCalls++
	return m.revokeErr
}

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

func newTestServiceWithConfig(t *testing.T, userRepo repository.UserRepository, tokenStore repository.TokenStore, cfg services.AuthConfig) services.AuthService {
	t.Helper()
	if cfg.PrivateKey == nil {
		cfg.PrivateKey = newTestRSAKey(t)
	}
	if cfg.Issuer == "" {
		cfg.Issuer = "test"
	}
	return services.NewAuthService(userRepo, tokenStore, cfg)
}

// ── Login Tests ────────────────────────────────────────────────────────────────

func TestLogin_Success(t *testing.T) {
	password := "Secret@123"
	user := &dao.User{
		ID:           "usr-001",
		TenantID:     "tenant-1",
		Username:     "admin",
		PasswordHash: hashPassword(t, password),
		Roles:        dao.StringArray{"ADMIN"},
		IsActive:     true,
	}

	userRepo := &mockUserRepo{user: user}
	tokenStore := &mockTokenStore{}
	svc := newTestService(t, userRepo, tokenStore)

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
	if resp.UserID != user.ID {
		t.Errorf("expected user ID %q, got %q", user.ID, resp.UserID)
	}
	if userRepo.findByUsernameCalls != 1 {
		t.Errorf("expected 1 FindByUsername call, got %d", userRepo.findByUsernameCalls)
	}
	if tokenStore.saveCalls != 1 {
		t.Errorf("expected 1 Save call, got %d", tokenStore.saveCalls)
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
	userRepo := &mockUserRepo{err: repository.ErrUserNotFound}
	svc := newTestService(t, userRepo, &mockTokenStore{})

	_, err := svc.Login(context.Background(), &dto.LoginRequest{
		Username: "nobody",
		Password: "any",
	})

	if !errors.Is(err, services.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
	if userRepo.findByUsernameCalls != 1 {
		t.Errorf("expected 1 FindByUsername call, got %d", userRepo.findByUsernameCalls)
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
	if !errors.Is(err, errors.New("db error")) && err.Error() != "auth: save refresh token: db error" {
		t.Errorf("expected wrapped db error, got %v", err)
	}
}

func TestLogin_UserRepositoryLookupError(t *testing.T) {
	lookupErr := errors.New("db connection failed")
	svc := newTestService(t,
		&mockUserRepo{err: lookupErr},
		&mockTokenStore{},
	)

	_, err := svc.Login(context.Background(), &dto.LoginRequest{
		Username: "admin",
		Password: "pass",
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
	if !errors.Is(err, lookupErr) {
		t.Errorf("expected wrapped lookup error, got %v", err)
	}
}

func TestLogin_BCryptCostDefault(t *testing.T) {
	user := &dao.User{
		ID:           "usr-001",
		Username:     "admin",
		PasswordHash: hashPassword(t, "password"),
		IsActive:     true,
	}

	key := newTestRSAKey(t)
	svc := services.NewAuthService(
		&mockUserRepo{user: user},
		&mockTokenStore{},
		services.AuthConfig{
			PrivateKey:      key,
			Issuer:          "test",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 24 * time.Hour,
			BCryptCost:      0, // Should default to bcrypt.DefaultCost
		},
	)

	resp, err := svc.Login(context.Background(), &dto.LoginRequest{
		Username: "admin",
		Password: "password",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
}

// ── Refresh Tests ──────────────────────────────────────────────────────────────

func TestRefresh_Success(t *testing.T) {
	user := &dao.User{
		ID:       "usr-001",
		TenantID: "tenant-1",
		Username: "admin",
		Roles:    dao.StringArray{"ADMIN"},
		IsActive: true,
	}

	refreshToken := &dao.RefreshToken{
		ID:        "rt-001",
		UserID:    "usr-001",
		TokenHash: repository.HashToken("test-token"),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
	}

	userRepo := &mockUserRepo{user: user}
	tokenStore := &mockTokenStore{findToken: refreshToken}
	svc := newTestService(t, userRepo, tokenStore)

	resp, err := svc.Refresh(context.Background(), &dto.RefreshRequest{
		RefreshToken: "test-token",
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
	if userRepo.findByIDCalls != 1 {
		t.Errorf("expected 1 FindByID call, got %d", userRepo.findByIDCalls)
	}
	if tokenStore.findCalls != 1 {
		t.Errorf("expected 1 Find call, got %d", tokenStore.findCalls)
	}
	if tokenStore.revokeCalls != 1 {
		t.Errorf("expected 1 Revoke call (for old token), got %d", tokenStore.revokeCalls)
	}
}

func TestRefresh_TokenNotFound(t *testing.T) {
	tokenStore := &mockTokenStore{findErr: repository.ErrTokenNotFound}
	svc := newTestService(t, &mockUserRepo{}, tokenStore)

	_, err := svc.Refresh(context.Background(), &dto.RefreshRequest{
		RefreshToken: "invalid",
	})

	if !errors.Is(err, services.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestRefresh_TokenRevoked(t *testing.T) {
	tokenStore := &mockTokenStore{findErr: repository.ErrTokenRevoked}
	svc := newTestService(t, &mockUserRepo{}, tokenStore)

	_, err := svc.Refresh(context.Background(), &dto.RefreshRequest{
		RefreshToken: "revoked",
	})

	if !errors.Is(err, services.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestRefresh_TokenExpired(t *testing.T) {
	tokenStore := &mockTokenStore{findErr: repository.ErrTokenExpired}
	svc := newTestService(t, &mockUserRepo{}, tokenStore)

	_, err := svc.Refresh(context.Background(), &dto.RefreshRequest{
		RefreshToken: "expired",
	})

	if !errors.Is(err, services.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestRefresh_TokenStoreError(t *testing.T) {
	lookupErr := errors.New("db connection failed")
	tokenStore := &mockTokenStore{findErr: lookupErr}
	svc := newTestService(t, &mockUserRepo{}, tokenStore)

	_, err := svc.Refresh(context.Background(), &dto.RefreshRequest{
		RefreshToken: "test",
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
	if !errors.Is(err, lookupErr) {
		t.Errorf("expected wrapped lookup error, got %v", err)
	}
}

func TestRefresh_RevokeOldTokenError(t *testing.T) {
	refreshToken := &dao.RefreshToken{
		ID:        "rt-001",
		UserID:    "usr-001",
		TokenHash: repository.HashToken("test-token"),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
	}

	revokeErr := errors.New("revoke failed")
	tokenStore := &mockTokenStore{
		findToken: refreshToken,
		revokeErr: revokeErr,
	}
	svc := newTestService(t, &mockUserRepo{}, tokenStore)

	_, err := svc.Refresh(context.Background(), &dto.RefreshRequest{
		RefreshToken: "test-token",
	})

	if !errors.Is(err, revokeErr) {
		t.Errorf("expected wrapped revoke error, got %v", err)
	}
}

func TestRefresh_UserNotFound(t *testing.T) {
	refreshToken := &dao.RefreshToken{
		ID:        "rt-001",
		UserID:    "usr-deleted",
		TokenHash: repository.HashToken("test-token"),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
	}

	userRepo := &mockUserRepo{err: repository.ErrUserNotFound}
	tokenStore := &mockTokenStore{findToken: refreshToken}
	svc := newTestService(t, userRepo, tokenStore)

	_, err := svc.Refresh(context.Background(), &dto.RefreshRequest{
		RefreshToken: "test-token",
	})

	if !errors.Is(err, services.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken when user not found, got %v", err)
	}
}

func TestRefresh_UserRepositoryError(t *testing.T) {
	refreshToken := &dao.RefreshToken{
		ID:        "rt-001",
		UserID:    "usr-001",
		TokenHash: repository.HashToken("test-token"),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
	}

	reloadErr := errors.New("db error")
	userRepo := &mockUserRepo{err: reloadErr}
	tokenStore := &mockTokenStore{findToken: refreshToken}
	svc := newTestService(t, userRepo, tokenStore)

	_, err := svc.Refresh(context.Background(), &dto.RefreshRequest{
		RefreshToken: "test-token",
	})

	if !errors.Is(err, reloadErr) {
		t.Errorf("expected wrapped reload error, got %v", err)
	}
}

func TestRefresh_TokenSaveError(t *testing.T) {
	user := &dao.User{
		ID:       "usr-001",
		TenantID: "tenant-1",
		Username: "admin",
		Roles:    dao.StringArray{"ADMIN"},
		IsActive: true,
	}

	refreshToken := &dao.RefreshToken{
		ID:        "rt-001",
		UserID:    "usr-001",
		TokenHash: repository.HashToken("test-token"),
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour),
		CreatedAt: time.Now(),
	}

	saveErr := errors.New("save failed")
	userRepo := &mockUserRepo{user: user}
	tokenStore := &mockTokenStore{
		findToken: refreshToken,
		saveErr:   saveErr,
	}
	svc := newTestService(t, userRepo, tokenStore)

	_, err := svc.Refresh(context.Background(), &dto.RefreshRequest{
		RefreshToken: "test-token",
	})

	if !errors.Is(err, saveErr) {
		t.Errorf("expected wrapped save error, got %v", err)
	}
}

// ── Logout Tests ───────────────────────────────────────────────────────────────

func TestLogout_Success(t *testing.T) {
	tokenStore := &mockTokenStore{}
	svc := newTestService(t, &mockUserRepo{}, tokenStore)

	err := svc.Logout(context.Background(), &dto.LogoutRequest{
		RefreshToken: "test-token",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokenStore.revokeCalls != 1 {
		t.Errorf("expected 1 Revoke call, got %d", tokenStore.revokeCalls)
	}
}

func TestLogout_RevokeError(t *testing.T) {
	revokeErr := errors.New("revoke failed")
	tokenStore := &mockTokenStore{revokeErr: revokeErr}
	svc := newTestService(t, &mockUserRepo{}, tokenStore)

	err := svc.Logout(context.Background(), &dto.LogoutRequest{
		RefreshToken: "test-token",
	})

	if !errors.Is(err, revokeErr) {
		t.Errorf("expected wrapped revoke error, got %v", err)
	}
}

// ── Token Pair Issuance Tests ──────────────────────────────────────────────────

func TestIssueTokenPair_WithSubjectEncryption(t *testing.T) {
	encryptionKey := make([]byte, 32)
	for i := range encryptionKey {
		encryptionKey[i] = byte(i)
	}

	password := "testpass"
	user := &dao.User{
		ID:           "usr-001",
		TenantID:     "tenant-1",
		Username:     "admin",
		PasswordHash: hashPassword(t, password),
		Roles:        dao.StringArray{"ADMIN"},
		IsActive:     true,
	}

	key := newTestRSAKey(t)
	svc := newTestServiceWithConfig(t, &mockUserRepo{user: user}, &mockTokenStore{}, services.AuthConfig{
		PrivateKey:           key,
		Issuer:               "test",
		AccessTokenTTL:       15 * time.Minute,
		RefreshTokenTTL:      24 * time.Hour,
		SubjectEncryptionKey: encryptionKey,
		BCryptCost:           bcrypt.MinCost,
	})

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
}

func TestIssueTokenPair_AccessTokenTTLDefault(t *testing.T) {
	password := "pass"
	user := &dao.User{
		ID:           "usr-001",
		TenantID:     "tenant-1",
		Username:     "admin",
		PasswordHash: hashPassword(t, password),
		IsActive:     true,
	}

	key := newTestRSAKey(t)
	svc := newTestServiceWithConfig(t, &mockUserRepo{user: user}, &mockTokenStore{}, services.AuthConfig{
		PrivateKey:      key,
		Issuer:          "test",
		AccessTokenTTL:  0, // Should default to 15 minutes
		RefreshTokenTTL: 24 * time.Hour,
		BCryptCost:      bcrypt.MinCost,
	})

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
	// Verify expiration was set
	if resp.AccessTokenExpiresAt == "" {
		t.Error("expected non-empty access token expiration")
	}
}

func TestIssueTokenPair_RefreshTokenTTLDefault(t *testing.T) {
	password := "pass"
	user := &dao.User{
		ID:           "usr-001",
		TenantID:     "tenant-1",
		Username:     "admin",
		PasswordHash: hashPassword(t, password),
		IsActive:     true,
	}

	key := newTestRSAKey(t)
	svc := newTestServiceWithConfig(t, &mockUserRepo{user: user}, &mockTokenStore{}, services.AuthConfig{
		PrivateKey:      key,
		Issuer:          "test",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 0, // Should default to 7 days
		BCryptCost:      bcrypt.MinCost,
	})

	resp, err := svc.Login(context.Background(), &dto.LoginRequest{
		Username: "admin",
		Password: password,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if resp.RefreshTokenExpiresAt == "" {
		t.Error("expected non-empty refresh token expiration")
	}
}

func TestIssueTokenPair_SubjectEncryptionError(t *testing.T) {
	password := "pass"
	user := &dao.User{
		ID:           "usr-001",
		TenantID:     "tenant-1",
		Username:     "admin",
		PasswordHash: hashPassword(t, password),
		IsActive:     true,
	}

	key := newTestRSAKey(t)
	svc := newTestServiceWithConfig(t, &mockUserRepo{user: user}, &mockTokenStore{}, services.AuthConfig{
		PrivateKey:           key,
		Issuer:               "test",
		AccessTokenTTL:       15 * time.Minute,
		RefreshTokenTTL:      24 * time.Hour,
		SubjectEncryptionKey: []byte("short"), // Invalid encryption key (too short for AES-256)
		BCryptCost:           bcrypt.MinCost,
	})

	_, err := svc.Login(context.Background(), &dto.LoginRequest{
		Username: "admin",
		Password: password,
	})

	if err == nil {
		t.Error("expected error for invalid encryption key, got nil")
	}
}

func TestIssueTokenPair_ResponseFieldsSet(t *testing.T) {
	password := "pass"
	user := &dao.User{
		ID:           "usr-001",
		TenantID:     "tenant-1",
		Username:     "admin",
		PasswordHash: hashPassword(t, password),
		Roles:        dao.StringArray{"USER", "ADMIN"},
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

	if resp.UserID != user.ID {
		t.Errorf("expected user ID %q, got %q", user.ID, resp.UserID)
	}
	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if resp.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if resp.AccessTokenExpiresAt == "" {
		t.Error("expected non-empty access token expiration")
	}
	if resp.RefreshTokenExpiresAt == "" {
		t.Error("expected non-empty refresh token expiration")
	}
}

func TestNewAuthService_WithNegativeBCryptCost(t *testing.T) {
	user := &dao.User{
		ID:           "usr-001",
		Username:     "admin",
		PasswordHash: hashPassword(t, "pass"),
		IsActive:     true,
	}

	key := newTestRSAKey(t)
	// With BCryptCost < 0, should use bcrypt.DefaultCost
	svc := services.NewAuthService(
		&mockUserRepo{user: user},
		&mockTokenStore{},
		services.AuthConfig{
			PrivateKey:      key,
			Issuer:          "test",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 24 * time.Hour,
			BCryptCost:      -1,
		},
	)

	if svc == nil {
		t.Error("expected service to be created")
	}
}

