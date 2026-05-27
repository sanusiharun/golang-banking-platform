// Package services contains auth-svc business logic.
//
// Login flow:
//  1. Fetch user by username (active only).
//  2. Verify password against bcrypt hash.
//  3. Sign an RS256 JWT with the service's RSA private key.
//     All other services verify it using only the corresponding PUBLIC key —
//     the private key never leaves this service.
package services

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/auth-svc/internal/repository"
)

// ── Sentinel errors ───────────────────────────────────────────────────────────

// ErrInvalidCredentials is returned for both "user not found" and "wrong password".
// Callers cannot tell which case occurred — prevents username enumeration.
var ErrInvalidCredentials = errors.New("invalid username or password")

// ── Interface ─────────────────────────────────────────────────────────────────

type AuthService interface {
	Login(ctx context.Context, req *dto.LoginRequest) (*dto.LoginResponse, error)
}

// ── Config ────────────────────────────────────────────────────────────────────

// AuthConfig holds the RS256 signing parameters. Injected by container.go.
type AuthConfig struct {
	PrivateKey *rsa.PrivateKey // RSA-2048 private key for signing (auth-svc only)
	Issuer     string
	TokenTTL   time.Duration // default: 24h
}

// ── Implementation ────────────────────────────────────────────────────────────

// dummyHash is a valid bcrypt hash used for constant-time comparisons when
// the username does not exist.  Prevents timing-based username enumeration.
const dummyHash = "$2b$12$qD2KBhzxixHH5inNtiNf9ec33TtHOgMKjy.pP76xjvnm2dCXZumHm"

type authService struct {
	userRepo repository.UserRepository
	cfg      AuthConfig
}

func NewAuthService(userRepo repository.UserRepository, cfg AuthConfig) AuthService {
	return &authService{userRepo: userRepo, cfg: cfg}
}

// Login authenticates a user and returns a signed RS256 JWT on success.
func (s *authService) Login(ctx context.Context, req *dto.LoginRequest) (*dto.LoginResponse, error) {
	// ── 1. Fetch user ──────────────────────────────────────────────────────────
	user, err := s.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			// Constant-time dummy comparison — attacker cannot infer whether
			// the username exists by measuring response latency.
			_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(req.Password))
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("auth: lookup user: %w", err)
	}

	// ── 2. Verify password ────────────────────────────────────────────────────
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		slog.WarnContext(ctx, "auth: invalid password attempt",
			slog.String("username", req.Username),
			slog.String("user_id", user.ID),
		)
		return nil, ErrInvalidCredentials
	}

	// ── 3. Sign RS256 JWT ─────────────────────────────────────────────────────
	ttl := s.cfg.TokenTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	expiresAt := time.Now().UTC().Add(ttl)

	claims := pkgmiddleware.Claims{
		UserID:   user.ID,
		TenantID: user.TenantID,
		Roles:    []string(user.Roles),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			Issuer:    s.cfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(s.cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("auth: sign token: %w", err)
	}

	slog.InfoContext(ctx, "auth: login successful",
		slog.String("user_id", user.ID),
		slog.String("username", user.Username),
		slog.Any("roles", user.Roles),
	)

	return &dto.LoginResponse{
		Token:     signed,
		ExpiresAt: expiresAt.Format(time.RFC3339),
		UserID:    user.ID,
		Roles:     []string(user.Roles),
	}, nil
}
