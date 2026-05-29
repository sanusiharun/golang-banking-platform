// Package services contains auth-svc business logic.
//
// Login flow:
//  1. Fetch user by username (active only).
//  2. Verify password against bcrypt hash.
//  3. Encrypt the user ID using AES-256-GCM before embedding it as JWT Subject.
//  4. Sign an RS256 JWT with the service's RSA private key.
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

	"go.opentelemetry.io/otel/attribute"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	pkgcrypto "github.com/sanusi/banking/pkg/crypto"
	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/pkg/observability"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/auth-svc/internal/repository"
)

// ── Sentinel errors ───────────────────────────────────────────────────────────

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidToken       = errors.New("invalid or expired refresh token")
)

// ── Interface ─────────────────────────────────────────────────────────────────

type AuthService interface {
	Login(ctx context.Context, req *dto.LoginRequest) (*dto.LoginResponse, error)
	Refresh(ctx context.Context, req *dto.RefreshRequest) (*dto.LoginResponse, error)
	Logout(ctx context.Context, req *dto.LogoutRequest) error
}

// ── Config ────────────────────────────────────────────────────────────────────

type AuthConfig struct {
	PrivateKey           *rsa.PrivateKey
	Issuer               string
	AccessTokenTTL       time.Duration
	RefreshTokenTTL      time.Duration
	SubjectEncryptionKey []byte
	BCryptCost           int
}

// ── Implementation ────────────────────────────────────────────────────────────

type authService struct {
	tr         *observability.ServiceTracer
	userRepo   repository.UserRepository
	tokenStore repository.TokenStore
	cfg        AuthConfig
	dummyHash  string
}

func NewAuthService(userRepo repository.UserRepository, tokenStore repository.TokenStore, cfg AuthConfig) AuthService {
	cost := cfg.BCryptCost
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte(uuid.NewString()), cost)
	if err != nil {
		dummy = []byte("$2b$12$invalidhashusedfortimingjustincase000000000000000000000")
	}
	return &authService{
		tr:         observability.NewServiceTracer("AuthService"),
		userRepo:   userRepo,
		tokenStore: tokenStore,
		cfg:        cfg,
		dummyHash:  string(dummy),
	}
}

func (s *authService) Login(ctx context.Context, req *dto.LoginRequest) (res *dto.LoginResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Login",
		attribute.String("auth.username", req.Username),
	)
	defer s.tr.Finish(span, &err)

	user, err := s.userRepo.FindByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			_ = bcrypt.CompareHashAndPassword([]byte(s.dummyHash), []byte(req.Password))
			err = ErrInvalidCredentials
			return nil, err
		}
		return nil, fmt.Errorf("auth: lookup user: %w", err)
	}

	if err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		slog.WarnContext(ctx, "auth: invalid password attempt",
			slog.String("username", req.Username),
			slog.String("user_id", user.ID),
		)
		err = ErrInvalidCredentials
		return nil, err
	}

	resp, err := s.issueTokenPair(ctx, user)
	if err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "auth: login successful",
		slog.String("user_id", user.ID),
		slog.String("username", user.Username),
		slog.Any("roles", user.Roles),
	)
	return resp, nil
}

func (s *authService) Refresh(ctx context.Context, req *dto.RefreshRequest) (res *dto.LoginResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Refresh")
	defer s.tr.Finish(span, &err)

	hash := repository.HashToken(req.RefreshToken)

	rt, err := s.tokenStore.FindByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, repository.ErrTokenNotFound) ||
			errors.Is(err, repository.ErrTokenRevoked) ||
			errors.Is(err, repository.ErrTokenExpired) {
			err = ErrInvalidToken
			return nil, err
		}
		return nil, fmt.Errorf("auth: find refresh token: %w", err)
	}

	if err = s.tokenStore.Revoke(ctx, hash); err != nil {
		return nil, fmt.Errorf("auth: revoke old token: %w", err)
	}

	user, err := s.userRepo.FindByID(ctx, rt.UserID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			err = ErrInvalidToken
			return nil, err
		}
		return nil, fmt.Errorf("auth: reload user: %w", err)
	}

	resp, err := s.issueTokenPair(ctx, user)
	if err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "auth: token refreshed", slog.String("user_id", user.ID))
	return resp, nil
}

func (s *authService) Logout(ctx context.Context, req *dto.LogoutRequest) (err error) {
	ctx, span := s.tr.Start(ctx, "Logout")
	defer s.tr.Finish(span, &err)

	hash := repository.HashToken(req.RefreshToken)
	if err = s.tokenStore.Revoke(ctx, hash); err != nil {
		return fmt.Errorf("auth: logout revoke: %w", err)
	}
	slog.InfoContext(ctx, "auth: logout successful")
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (s *authService) issueTokenPair(ctx context.Context, user *dao.User) (*dto.LoginResponse, error) {
	accessTTL := s.cfg.AccessTokenTTL
	if accessTTL <= 0 {
		accessTTL = 15 * time.Minute
	}
	accessExpiresAt := time.Now().UTC().Add(accessTTL)

	subject := user.ID
	if len(s.cfg.SubjectEncryptionKey) > 0 {
		encrypted, err := pkgcrypto.EncryptSubject(s.cfg.SubjectEncryptionKey, user.ID)
		if err != nil {
			return nil, fmt.Errorf("auth: encrypt subject: %w", err)
		}
		subject = encrypted
	}

	claims := pkgmiddleware.Claims{
		TenantID: user.TenantID,
		Roles:    []string(user.Roles),
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			Subject:   subject,
			Issuer:    s.cfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(accessExpiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(s.cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("auth: sign token: %w", err)
	}

	refreshTTL := s.cfg.RefreshTokenTTL
	if refreshTTL <= 0 {
		refreshTTL = 7 * 24 * time.Hour
	}
	refreshExpiresAt := time.Now().UTC().Add(refreshTTL)
	rawRefresh := uuid.NewString()

	rt := &dao.RefreshToken{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		TokenHash: repository.HashToken(rawRefresh),
		ExpiresAt: refreshExpiresAt,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.tokenStore.Save(ctx, rt); err != nil {
		return nil, fmt.Errorf("auth: save refresh token: %w", err)
	}

	return &dto.LoginResponse{
		AccessToken:           signed,
		RefreshToken:          rawRefresh,
		AccessTokenExpiresAt:  accessExpiresAt.Format(time.RFC3339),
		RefreshTokenExpiresAt: refreshExpiresAt.Format(time.RFC3339),
	}, nil
}
