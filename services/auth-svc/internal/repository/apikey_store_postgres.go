package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
)

// ── ServiceAccount (Postgres) ─────────────────────────────────────────────────

type postgresServiceAccountStore struct {
	db *gorm.DB
}

func NewPostgresServiceAccountStore(db *gorm.DB) ServiceAccountStore {
	return &postgresServiceAccountStore{db: db}
}

func (s *postgresServiceAccountStore) Save(ctx context.Context, sa *dao.ServiceAccount) error {
	if err := s.db.WithContext(ctx).Create(sa).Error; err != nil {
		return fmt.Errorf("service_account(postgres): save: %w", err)
	}
	return nil
}

func (s *postgresServiceAccountStore) FindByID(ctx context.Context, id string) (*dao.ServiceAccount, error) {
	var sa dao.ServiceAccount
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&sa).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAPIKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("service_account(postgres): find by id: %w", err)
	}
	return &sa, nil
}

func (s *postgresServiceAccountStore) Update(ctx context.Context, sa *dao.ServiceAccount) error {
	sa.UpdatedAt = time.Now()
	if err := s.db.WithContext(ctx).Save(sa).Error; err != nil {
		return fmt.Errorf("service_account(postgres): update: %w", err)
	}
	return nil
}

func (s *postgresServiceAccountStore) List(ctx context.Context, tenantID string) ([]*dao.ServiceAccount, error) {
	var accounts []*dao.ServiceAccount
	q := s.db.WithContext(ctx)
	if tenantID != "" {
		q = q.Where("tenant_id = ?", tenantID)
	}
	if err := q.Find(&accounts).Error; err != nil {
		return nil, fmt.Errorf("service_account(postgres): list: %w", err)
	}
	return accounts, nil
}

// ── APIKey (Postgres) ─────────────────────────────────────────────────────────

type postgresAPIKeyStore struct {
	db *gorm.DB
}

func NewPostgresAPIKeyStore(db *gorm.DB) APIKeyStore {
	return &postgresAPIKeyStore{db: db}
}

func (s *postgresAPIKeyStore) Save(ctx context.Context, key *dao.APIKey) error {
	if err := s.db.WithContext(ctx).Create(key).Error; err != nil {
		return fmt.Errorf("api_key(postgres): save: %w", err)
	}
	return nil
}

// FindActiveByHash joins api_keys + service_accounts and returns the resolved identity.
// The partial index on api_keys(key_hash) WHERE revoked_at IS NULL makes this O(log n).
func (s *postgresAPIKeyStore) FindActiveByHash(ctx context.Context, hash string) (*pkgmiddleware.ServiceAccountIdentity, error) {
	type row struct {
		KeyID            string
		KeyExpiresAt     *time.Time
		ServiceAccountID string
		TenantID         string
		Roles            dao.StringArray
		IsActive         bool
	}

	var r row
	err := s.db.WithContext(ctx).Raw(`
		SELECT
			k.id              AS key_id,
			k.expires_at      AS key_expires_at,
			sa.id             AS service_account_id,
			sa.tenant_id,
			sa.roles,
			sa.is_active
		FROM api_keys k
		JOIN service_accounts sa ON sa.id = k.service_account_id
		WHERE k.key_hash  = ?
		  AND k.revoked_at IS NULL
		  AND sa.is_active = TRUE
		LIMIT 1
	`, hash).Scan(&r).Error
	if err != nil {
		return nil, fmt.Errorf("api_key(postgres): find by hash: %w", err)
	}
	if r.KeyID == "" {
		return nil, ErrAPIKeyNotFound
	}
	if r.KeyExpiresAt != nil && time.Now().UTC().After(*r.KeyExpiresAt) {
		return nil, ErrAPIKeyExpired
	}

	return &pkgmiddleware.ServiceAccountIdentity{
		ServiceAccountID: r.ServiceAccountID,
		TenantID:         r.TenantID,
		Roles:            []string(r.Roles),
		KeyID:            r.KeyID,
		ExpiresAt:        r.KeyExpiresAt,
	}, nil
}

func (s *postgresAPIKeyStore) Revoke(ctx context.Context, keyID, _ string) error {
	now := time.Now()
	result := s.db.WithContext(ctx).
		Model(&dao.APIKey{}).
		Where("id = ? AND revoked_at IS NULL", keyID).
		Update("revoked_at", now)
	if result.Error != nil {
		return fmt.Errorf("api_key(postgres): revoke: %w", result.Error)
	}
	return nil
}

func (s *postgresAPIKeyStore) ListByServiceAccount(ctx context.Context, serviceAccountID string) ([]*dao.APIKey, error) {
	var keys []*dao.APIKey
	if err := s.db.WithContext(ctx).
		Where("service_account_id = ?", serviceAccountID).
		Order("created_at DESC").
		Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("api_key(postgres): list by sa: %w", err)
	}
	return keys, nil
}

func (s *postgresAPIKeyStore) UpdateLastUsed(ctx context.Context, keyID string) error {
	return s.db.WithContext(ctx).
		Model(&dao.APIKey{}).
		Where("id = ?", keyID).
		Update("last_used_at", time.Now()).Error
}
