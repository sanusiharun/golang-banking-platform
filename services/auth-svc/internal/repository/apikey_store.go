package repository

import (
	"context"
	"errors"

	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
)

// ── Sentinel errors ───────────────────────────────────────────────────────────

var (
	ErrAPIKeyNotFound = errors.New("api key not found or inactive")
	ErrAPIKeyRevoked  = errors.New("api key has been revoked")
	ErrAPIKeyExpired  = errors.New("api key has expired")
)

// ── ServiceAccountStore ───────────────────────────────────────────────────────

// ServiceAccountStore handles service account persistence.
type ServiceAccountStore interface {
	Save(ctx context.Context, sa *dao.ServiceAccount) error
	FindByID(ctx context.Context, id string) (*dao.ServiceAccount, error)
	Update(ctx context.Context, sa *dao.ServiceAccount) error
	List(ctx context.Context, tenantID string) ([]*dao.ServiceAccount, error)
}

// ── APIKeyStore ───────────────────────────────────────────────────────────────

// APIKeyStore handles API key persistence and lookup.
// FindActiveByHash is on the hot path — every authenticated request calls it.
// All other methods are management operations, called infrequently.
type APIKeyStore interface {
	// Save persists a new API key.
	Save(ctx context.Context, key *dao.APIKey) error

	// FindActiveByHash resolves an active, non-expired key into a ServiceAccountIdentity.
	// This is the hot-path method called on every API key request.
	// Returns ErrAPIKeyNotFound if no active key matches the hash.
	FindActiveByHash(ctx context.Context, hash string) (*pkgmiddleware.ServiceAccountIdentity, error)

	// Revoke marks a key as revoked. keyID is the api_keys.id, hash is
	// provided so the cache implementation can invalidate Redis immediately.
	Revoke(ctx context.Context, keyID, hash string) error

	// ListByServiceAccount returns all keys (active and revoked) for a service account.
	ListByServiceAccount(ctx context.Context, serviceAccountID string) ([]*dao.APIKey, error)

	// UpdateLastUsed sets last_used_at = NOW() for the given key ID.
	// Called asynchronously — errors are logged but not surfaced.
	UpdateLastUsed(ctx context.Context, keyID string) error
}
