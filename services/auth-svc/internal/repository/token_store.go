package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
)

// ── Sentinel errors ───────────────────────────────────────────────────────────

var (
	ErrTokenNotFound = errors.New("refresh token not found")
	ErrTokenRevoked  = errors.New("refresh token has been revoked")
	ErrTokenExpired  = errors.New("refresh token has expired")
)

// ── Interface ─────────────────────────────────────────────────────────────────

// TokenStore is the pluggable backend for refresh token persistence.
// Choose an implementation via TOKEN_STORE env var:
//
//	postgres (default) — durable, survives restarts
//	redis              — fast, volatile unless Redis persistence is enabled
//	memory             — local dev / testing only, lost on restart
type TokenStore interface {
	// Save persists a new refresh token.
	Save(ctx context.Context, rt *dao.RefreshToken) error

	// FindByHash returns the token matching the given SHA-256 hash.
	// Returns ErrTokenNotFound if no match, ErrTokenRevoked or ErrTokenExpired
	// if the token exists but cannot be used.
	FindByHash(ctx context.Context, hash string) (*dao.RefreshToken, error)

	// Revoke marks a single token as revoked (used on refresh + logout).
	Revoke(ctx context.Context, hash string) error

	// RevokeAllForUser revokes every active token for a user.
	// Use on password change, account suspension, or suspicious activity.
	RevokeAllForUser(ctx context.Context, userID string) error
}

// HashToken returns the SHA-256 hex digest of a raw refresh token string.
// This is the value stored in the DB / cache — the raw token is never persisted.
func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
