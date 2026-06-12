package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/auth-svc/internal/repository"
)

// ── Interface ─────────────────────────────────────────────────────────────────

type APIKeyService interface {
	// Service Account management
	CreateServiceAccount(ctx context.Context, req *dto.CreateServiceAccountRequest, createdBy string) (*dto.ServiceAccountResponse, error)
	GetServiceAccount(ctx context.Context, id string) (*dto.ServiceAccountResponse, error)
	UpdateServiceAccount(ctx context.Context, id string, req *dto.UpdateServiceAccountRequest) (*dto.ServiceAccountResponse, error)
	ListServiceAccounts(ctx context.Context, tenantID string) ([]*dto.ServiceAccountResponse, error)

	// API Key management
	CreateAPIKey(ctx context.Context, serviceAccountID string, req *dto.CreateAPIKeyRequest, createdBy string) (*dto.CreateAPIKeyResponse, error)
	RevokeAPIKey(ctx context.Context, serviceAccountID, keyID string) error
	ListAPIKeys(ctx context.Context, serviceAccountID string) ([]*dto.APIKeyResponse, error)

	// IntrospectAPIKey resolves a SHA-256 hash to a ServiceAccountIdentity.
	// Called by downstream services (e.g. account-svc) via POST /auth/apikey/introspect.
	// Updates last_used_at asynchronously.
	IntrospectAPIKey(ctx context.Context, hash string) (*pkgmiddleware.ServiceAccountIdentity, error)
}

// ── Implementation ────────────────────────────────────────────────────────────

type apiKeyService struct {
	saStore     repository.ServiceAccountStore
	keyStore    repository.APIKeyStore
	environment string // "live" for production, "test" otherwise
}

func NewAPIKeyService(
	saStore repository.ServiceAccountStore,
	keyStore repository.APIKeyStore,
	environment string,
) APIKeyService {
	return &apiKeyService{
		saStore:     saStore,
		keyStore:    keyStore,
		environment: environment,
	}
}

// ── Service Account ───────────────────────────────────────────────────────────

func (s *apiKeyService) CreateServiceAccount(ctx context.Context, req *dto.CreateServiceAccountRequest, createdBy string) (*dto.ServiceAccountResponse, error) {
	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	sa := &dao.ServiceAccount{
		ID:          uuid.NewString(),
		Name:        req.Name,
		Description: req.Description,
		TenantID:    tenantID,
		Roles:       dao.StringArray(req.Roles),
		IsActive:    true,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.saStore.Save(ctx, sa); err != nil {
		return nil, fmt.Errorf("create service account: %w", err)
	}

	slog.InfoContext(ctx, "service account created",
		slog.String("id", sa.ID),
		slog.String("name", sa.Name),
		slog.String("created_by", createdBy),
	)

	return toServiceAccountResponse(sa), nil
}

func (s *apiKeyService) GetServiceAccount(ctx context.Context, id string) (*dto.ServiceAccountResponse, error) {
	sa, err := s.saStore.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get service account: %w", err)
	}
	return toServiceAccountResponse(sa), nil
}

func (s *apiKeyService) UpdateServiceAccount(ctx context.Context, id string, req *dto.UpdateServiceAccountRequest) (*dto.ServiceAccountResponse, error) {
	sa, err := s.saStore.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("update service account: find: %w", err)
	}

	if len(req.Roles) > 0 {
		sa.Roles = dao.StringArray(req.Roles)
	}
	if req.IsActive != nil {
		sa.IsActive = *req.IsActive
	}

	if err := s.saStore.Update(ctx, sa); err != nil {
		return nil, fmt.Errorf("update service account: save: %w", err)
	}

	slog.InfoContext(ctx, "service account updated", slog.String("id", sa.ID))
	return toServiceAccountResponse(sa), nil
}

func (s *apiKeyService) ListServiceAccounts(ctx context.Context, tenantID string) ([]*dto.ServiceAccountResponse, error) {
	accounts, err := s.saStore.List(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list service accounts: %w", err)
	}
	result := make([]*dto.ServiceAccountResponse, len(accounts))
	for i, sa := range accounts {
		result[i] = toServiceAccountResponse(sa)
	}
	return result, nil
}

// ── API Key ───────────────────────────────────────────────────────────────────

func (s *apiKeyService) CreateAPIKey(ctx context.Context, serviceAccountID string, req *dto.CreateAPIKeyRequest, createdBy string) (*dto.CreateAPIKeyResponse, error) {
	// Confirm service account exists and is active.
	if _, err := s.saStore.FindByID(ctx, serviceAccountID); err != nil {
		return nil, fmt.Errorf("create api key: service account not found: %w", err)
	}

	env := "test"
	if s.environment == "production" || s.environment == "prod" {
		env = "live"
	}

	rawKey, hash, err := pkgmiddleware.GenerateAPIKey(env)
	if err != nil {
		return nil, fmt.Errorf("create api key: generate: %w", err)
	}

	prefix := rawKey
	if len(rawKey) > 10 {
		prefix = rawKey[:10]
	}

	key := &dao.APIKey{
		ID:               uuid.NewString(),
		ServiceAccountID: serviceAccountID,
		Name:             req.Name,
		KeyHash:          hash,
		KeyPrefix:        prefix,
		ExpiresAt:        req.ExpiresAt,
		CreatedBy:        createdBy,
		CreatedAt:        time.Now(),
	}

	if err := s.keyStore.Save(ctx, key); err != nil {
		return nil, fmt.Errorf("create api key: save: %w", err)
	}

	slog.InfoContext(ctx, "api key created",
		slog.String("key_id", key.ID),
		slog.String("service_account_id", serviceAccountID),
		slog.String("prefix", prefix),
		slog.String("created_by", createdBy),
	)

	return &dto.CreateAPIKeyResponse{
		KeyID:     key.ID,
		RawKey:    rawKey, // returned ONCE — never stored, never logged
		KeyPrefix: prefix,
		Name:      key.Name,
		ExpiresAt: key.ExpiresAt,
		CreatedAt: key.CreatedAt,
	}, nil
}

func (s *apiKeyService) RevokeAPIKey(ctx context.Context, serviceAccountID, keyID string) error {
	// Fetch keys to verify ownership and get the hash for cache invalidation.
	keys, err := s.keyStore.ListByServiceAccount(ctx, serviceAccountID)
	if err != nil {
		return fmt.Errorf("revoke api key: list: %w", err)
	}

	var targetHash string
	for _, k := range keys {
		if k.ID == keyID {
			targetHash = k.KeyHash
			break
		}
	}
	if targetHash == "" {
		return fmt.Errorf("revoke api key: key %s not found on service account %s", keyID, serviceAccountID)
	}

	if err := s.keyStore.Revoke(ctx, keyID, targetHash); err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}

	slog.InfoContext(ctx, "api key revoked",
		slog.String("key_id", keyID),
		slog.String("service_account_id", serviceAccountID),
	)
	return nil
}

func (s *apiKeyService) ListAPIKeys(ctx context.Context, serviceAccountID string) ([]*dto.APIKeyResponse, error) {
	keys, err := s.keyStore.ListByServiceAccount(ctx, serviceAccountID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	result := make([]*dto.APIKeyResponse, len(keys))
	for i, k := range keys {
		result[i] = toAPIKeyResponse(k)
	}
	return result, nil
}

// ── Mappers ───────────────────────────────────────────────────────────────────

func toServiceAccountResponse(sa *dao.ServiceAccount) *dto.ServiceAccountResponse {
	return &dto.ServiceAccountResponse{
		ID:          sa.ID,
		Name:        sa.Name,
		Description: sa.Description,
		TenantID:    sa.TenantID,
		Roles:       []string(sa.Roles),
		IsActive:    sa.IsActive,
		CreatedBy:   sa.CreatedBy,
		CreatedAt:   sa.CreatedAt,
		UpdatedAt:   sa.UpdatedAt,
	}
}

func toAPIKeyResponse(k *dao.APIKey) *dto.APIKeyResponse {
	return &dto.APIKeyResponse{
		ID:               k.ID,
		ServiceAccountID: k.ServiceAccountID,
		Name:             k.Name,
		KeyPrefix:        k.KeyPrefix,
		ExpiresAt:        k.ExpiresAt,
		RevokedAt:        k.RevokedAt,
		LastUsedAt:       k.LastUsedAt,
		CreatedAt:        k.CreatedAt,
	}
}

// IntrospectAPIKey resolves a SHA-256 hash into a ServiceAccountIdentity.
// Updates last_used_at asynchronously so it doesn't block the response.
func (s *apiKeyService) IntrospectAPIKey(ctx context.Context, hash string) (*pkgmiddleware.ServiceAccountIdentity, error) {
	identity, err := s.keyStore.FindActiveByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("introspect api key: %w", err)
	}
	go func() {
		ctx2, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.keyStore.UpdateLastUsed(ctx2, identity.KeyID); err != nil {
			slog.Warn("introspect api key: failed to update last_used_at",
				slog.String("key_id", identity.KeyID),
				slog.String("error", err.Error()),
			)
		}
	}()
	return identity, nil
}
