package unit

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/sanusi/banking/services/auth-svc/internal/repository"
	"github.com/sanusi/banking/services/auth-svc/internal/services"
)

// ── Test Helpers ──────────────────────────────────────────────────────────────

func NewTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

func HashPassword(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return string(h)
}

func NewTestAuthService(t *testing.T, userRepo repository.UserRepository, tokenStore repository.TokenStore) services.AuthService {
	t.Helper()
	key := NewTestRSAKey(t)
	return services.NewAuthService(userRepo, tokenStore, services.AuthConfig{
		PrivateKey:      key,
		Issuer:          "test",
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 24 * time.Hour,
		BCryptCost:      bcrypt.MinCost,
	})
}

func NewTestAuthServiceWithConfig(t *testing.T, userRepo repository.UserRepository, tokenStore repository.TokenStore, cfg services.AuthConfig) services.AuthService {
	t.Helper()
	if cfg.PrivateKey == nil {
		cfg.PrivateKey = NewTestRSAKey(t)
	}
	if cfg.Issuer == "" {
		cfg.Issuer = "test"
	}
	return services.NewAuthService(userRepo, tokenStore, cfg)
}
