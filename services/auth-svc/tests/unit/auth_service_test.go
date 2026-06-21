package unit

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/auth-svc/internal/repository"
	"github.com/sanusi/banking/services/auth-svc/internal/services"
)

// ── Login Tests ────────────────────────────────────────────────────────────────

func TestLogin_Success(t *testing.T) {
	password := "Secret@123"
	user := &dao.User{
		ID:           "usr-001",
		TenantID:     "tenant-1",
		Username:     "admin",
		PasswordHash: HashPassword(t, password),
		Roles:        dao.StringArray{"ADMIN"},
		IsActive:     true,
	}

	userRepo := &MockUserRepo{User: user}
	tokenStore := &MockTokenStore{}
	svc := NewTestAuthService(t, userRepo, tokenStore)

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
	if userRepo.FindByUsernameCalls != 1 {
		t.Errorf("expected 1 FindByUsername call, got %d", userRepo.FindByUsernameCalls)
	}
	if tokenStore.SaveCalls != 1 {
		t.Errorf("expected 1 Save call, got %d", tokenStore.SaveCalls)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	user := &dao.User{
		ID:           "usr-001",
		Username:     "admin",
		PasswordHash: HashPassword(t, "correct-password"),
		IsActive:     true,
	}

	svc := NewTestAuthService(t, &MockUserRepo{User: user}, &MockTokenStore{})

	_, err := svc.Login(context.Background(), &dto.LoginRequest{
		Username: "admin",
		Password: "wrong-password",
	})

	if !errors.Is(err, services.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	userRepo := &MockUserRepo{Err: repository.ErrUserNotFound}
	svc := NewTestAuthService(t, userRepo, &MockTokenStore{})

	_, err := svc.Login(context.Background(), &dto.LoginRequest{
		Username: "nobody",
		Password: "any",
	})

	if !errors.Is(err, services.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
	if userRepo.FindByUsernameCalls != 1 {
		t.Errorf("expected 1 FindByUsername call, got %d", userRepo.FindByUsernameCalls)
	}
}

func TestLogin_InactiveUser(t *testing.T) {
	user := &dao.User{
		ID:           "usr-002",
		Username:     "inactive",
		PasswordHash: HashPassword(t, "password"),
		IsActive:     false,
	}

	svc := NewTestAuthService(t, &MockUserRepo{User: user}, &MockTokenStore{})

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
		PasswordHash: HashPassword(t, "pass"),
		IsActive:     true,
	}

	svc := NewTestAuthService(t,
		&MockUserRepo{User: user},
		&MockTokenStore{SaveErr: errors.New("db error")},
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
	svc := NewTestAuthService(t,
		&MockUserRepo{Err: lookupErr},
		&MockTokenStore{},
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
		PasswordHash: HashPassword(t, "password"),
		IsActive:     true,
	}

	key := NewTestRSAKey(t)
	svc := services.NewAuthService(
		&MockUserRepo{User: user},
		&MockTokenStore{},
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

	userRepo := &MockUserRepo{User: user}
	tokenStore := &MockTokenStore{FindToken: refreshToken}
	svc := NewTestAuthService(t, userRepo, tokenStore)

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
	if userRepo.FindByIDCalls != 1 {
		t.Errorf("expected 1 FindByID call, got %d", userRepo.FindByIDCalls)
	}
	if tokenStore.FindCalls != 1 {
		t.Errorf("expected 1 Find call, got %d", tokenStore.FindCalls)
	}
	if tokenStore.RevokeCalls != 1 {
		t.Errorf("expected 1 Revoke call (for old token), got %d", tokenStore.RevokeCalls)
	}
}

func TestRefresh_TokenNotFound(t *testing.T) {
	tokenStore := &MockTokenStore{FindErr: repository.ErrTokenNotFound}
	svc := NewTestAuthService(t, &MockUserRepo{}, tokenStore)

	_, err := svc.Refresh(context.Background(), &dto.RefreshRequest{
		RefreshToken: "invalid",
	})

	if !errors.Is(err, services.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestRefresh_TokenRevoked(t *testing.T) {
	tokenStore := &MockTokenStore{FindErr: repository.ErrTokenRevoked}
	svc := NewTestAuthService(t, &MockUserRepo{}, tokenStore)

	_, err := svc.Refresh(context.Background(), &dto.RefreshRequest{
		RefreshToken: "revoked",
	})

	if !errors.Is(err, services.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestRefresh_TokenExpired(t *testing.T) {
	tokenStore := &MockTokenStore{FindErr: repository.ErrTokenExpired}
	svc := NewTestAuthService(t, &MockUserRepo{}, tokenStore)

	_, err := svc.Refresh(context.Background(), &dto.RefreshRequest{
		RefreshToken: "expired",
	})

	if !errors.Is(err, services.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
}

func TestRefresh_TokenStoreError(t *testing.T) {
	lookupErr := errors.New("db connection failed")
	tokenStore := &MockTokenStore{FindErr: lookupErr}
	svc := NewTestAuthService(t, &MockUserRepo{}, tokenStore)

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
	tokenStore := &MockTokenStore{
		FindToken: refreshToken,
		RevokeErr: revokeErr,
	}
	svc := NewTestAuthService(t, &MockUserRepo{}, tokenStore)

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

	userRepo := &MockUserRepo{Err: repository.ErrUserNotFound}
	tokenStore := &MockTokenStore{FindToken: refreshToken}
	svc := NewTestAuthService(t, userRepo, tokenStore)

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
	userRepo := &MockUserRepo{Err: reloadErr}
	tokenStore := &MockTokenStore{FindToken: refreshToken}
	svc := NewTestAuthService(t, userRepo, tokenStore)

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
	userRepo := &MockUserRepo{User: user}
	tokenStore := &MockTokenStore{
		FindToken: refreshToken,
		SaveErr:   saveErr,
	}
	svc := NewTestAuthService(t, userRepo, tokenStore)

	_, err := svc.Refresh(context.Background(), &dto.RefreshRequest{
		RefreshToken: "test-token",
	})

	if !errors.Is(err, saveErr) {
		t.Errorf("expected wrapped save error, got %v", err)
	}
}

// ── Logout Tests ───────────────────────────────────────────────────────────────

func TestLogout_Success(t *testing.T) {
	tokenStore := &MockTokenStore{}
	svc := NewTestAuthService(t, &MockUserRepo{}, tokenStore)

	err := svc.Logout(context.Background(), &dto.LogoutRequest{
		RefreshToken: "test-token",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tokenStore.RevokeCalls != 1 {
		t.Errorf("expected 1 Revoke call, got %d", tokenStore.RevokeCalls)
	}
}

func TestLogout_RevokeError(t *testing.T) {
	revokeErr := errors.New("revoke failed")
	tokenStore := &MockTokenStore{RevokeErr: revokeErr}
	svc := NewTestAuthService(t, &MockUserRepo{}, tokenStore)

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
		PasswordHash: HashPassword(t, password),
		Roles:        dao.StringArray{"ADMIN"},
		IsActive:     true,
	}

	key := NewTestRSAKey(t)
	svc := NewTestAuthServiceWithConfig(t, &MockUserRepo{User: user}, &MockTokenStore{}, services.AuthConfig{
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
		PasswordHash: HashPassword(t, password),
		IsActive:     true,
	}

	key := NewTestRSAKey(t)
	svc := NewTestAuthServiceWithConfig(t, &MockUserRepo{User: user}, &MockTokenStore{}, services.AuthConfig{
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
		PasswordHash: HashPassword(t, password),
		IsActive:     true,
	}

	key := NewTestRSAKey(t)
	svc := NewTestAuthServiceWithConfig(t, &MockUserRepo{User: user}, &MockTokenStore{}, services.AuthConfig{
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
		PasswordHash: HashPassword(t, password),
		IsActive:     true,
	}

	key := NewTestRSAKey(t)
	svc := NewTestAuthServiceWithConfig(t, &MockUserRepo{User: user}, &MockTokenStore{}, services.AuthConfig{
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
		PasswordHash: HashPassword(t, password),
		Roles:        dao.StringArray{"USER", "ADMIN"},
		IsActive:     true,
	}

	svc := NewTestAuthService(t, &MockUserRepo{User: user}, &MockTokenStore{})

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
		PasswordHash: HashPassword(t, "pass"),
		IsActive:     true,
	}

	key := NewTestRSAKey(t)
	// With BCryptCost < 0, should use bcrypt.DefaultCost
	svc := services.NewAuthService(
		&MockUserRepo{User: user},
		&MockTokenStore{},
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

