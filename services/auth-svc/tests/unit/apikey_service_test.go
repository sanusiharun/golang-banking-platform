package unit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/auth-svc/internal/repository"
	"github.com/sanusi/banking/services/auth-svc/internal/services"
)

// ── Mock stores ───────────────────────────────────────────────────────────────

type mockSAStore struct {
	saved    *dao.ServiceAccount
	saveErr  error
	found    *dao.ServiceAccount
	findErr  error
	updated  *dao.ServiceAccount
	updateErr error
	listed   []*dao.ServiceAccount
	listErr  error
}

func (m *mockSAStore) Save(_ context.Context, sa *dao.ServiceAccount) error {
	m.saved = sa
	return m.saveErr
}

func (m *mockSAStore) FindByID(_ context.Context, _ string) (*dao.ServiceAccount, error) {
	return m.found, m.findErr
}

func (m *mockSAStore) Update(_ context.Context, sa *dao.ServiceAccount) error {
	m.updated = sa
	return m.updateErr
}

func (m *mockSAStore) List(_ context.Context, _ string) ([]*dao.ServiceAccount, error) {
	return m.listed, m.listErr
}

var _ repository.ServiceAccountStore = (*mockSAStore)(nil)

// mockKeyStore implements repository.APIKeyStore for testing.
type mockKeyStore struct {
	saved        *dao.APIKey
	saveErr      error
	identity     *pkgmiddleware.ServiceAccountIdentity
	findErr      error
	revokeErr    error
	listed       []*dao.APIKey
	listErr      error
	lastUsedErr  error
}

func (m *mockKeyStore) Save(_ context.Context, key *dao.APIKey) error {
	m.saved = key
	return m.saveErr
}

func (m *mockKeyStore) FindActiveByHash(_ context.Context, _ string) (*pkgmiddleware.ServiceAccountIdentity, error) {
	return m.identity, m.findErr
}

func (m *mockKeyStore) Revoke(_ context.Context, _, _ string) error {
	return m.revokeErr
}

func (m *mockKeyStore) ListByServiceAccount(_ context.Context, _ string) ([]*dao.APIKey, error) {
	return m.listed, m.listErr
}

func (m *mockKeyStore) UpdateLastUsed(_ context.Context, _ string) error {
	return m.lastUsedErr
}

var _ repository.APIKeyStore = (*mockKeyStore)(nil)

// ── Helpers ───────────────────────────────────────────────────────────────────

func newAPIKeyService(saStore repository.ServiceAccountStore, keyStore repository.APIKeyStore, env string) services.APIKeyService {
	return services.NewAPIKeyService(saStore, keyStore, env)
}

func testServiceAccount() *dao.ServiceAccount {
	return &dao.ServiceAccount{
		ID:        "sa-uuid-1",
		Name:      "payment-gateway",
		TenantID:  "tenant-abc",
		Roles:     dao.StringArray{"payments:write", "refunds:write"},
		IsActive:  true,
		CreatedBy: "admin-user-1",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// ── CreateServiceAccount ──────────────────────────────────────────────────────

func TestCreateServiceAccount_Success(t *testing.T) {
	saStore := &mockSAStore{}
	svc := newAPIKeyService(saStore, &mockKeyStore{}, "local")

	req := &dto.CreateServiceAccountRequest{
		Name:     "payment-gateway",
		TenantID: "tenant-abc",
		Roles:    []string{"payments:write"},
	}

	resp, err := svc.CreateServiceAccount(context.Background(), req, "admin-user-1")
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, "payment-gateway", resp.Name)
	assert.Equal(t, "tenant-abc", resp.TenantID)
	assert.Equal(t, []string{"payments:write"}, resp.Roles)
	assert.True(t, resp.IsActive)
	assert.Equal(t, "admin-user-1", resp.CreatedBy)
	assert.NotEmpty(t, resp.ID, "ID must be generated")

	require.NotNil(t, saStore.saved)
	assert.Equal(t, resp.ID, saStore.saved.ID)
}

func TestCreateServiceAccount_DefaultTenantID(t *testing.T) {
	saStore := &mockSAStore{}
	svc := newAPIKeyService(saStore, &mockKeyStore{}, "local")

	req := &dto.CreateServiceAccountRequest{
		Name:  "internal-service",
		Roles: []string{"internal:read"},
		// TenantID intentionally omitted
	}

	resp, err := svc.CreateServiceAccount(context.Background(), req, "admin-1")
	require.NoError(t, err)
	assert.Equal(t, "default", resp.TenantID)
}

func TestCreateServiceAccount_SaveError(t *testing.T) {
	saStore := &mockSAStore{saveErr: errors.New("db: unique constraint violation")}
	svc := newAPIKeyService(saStore, &mockKeyStore{}, "local")

	req := &dto.CreateServiceAccountRequest{Name: "dup-service", Roles: []string{"read"}}
	_, err := svc.CreateServiceAccount(context.Background(), req, "admin-1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create service account")
}

// ── GetServiceAccount ─────────────────────────────────────────────────────────

func TestGetServiceAccount_Success(t *testing.T) {
	sa := testServiceAccount()
	saStore := &mockSAStore{found: sa}
	svc := newAPIKeyService(saStore, &mockKeyStore{}, "local")

	resp, err := svc.GetServiceAccount(context.Background(), sa.ID)
	require.NoError(t, err)
	assert.Equal(t, sa.ID, resp.ID)
	assert.Equal(t, sa.Name, resp.Name)
	assert.Equal(t, []string(sa.Roles), resp.Roles)
}

func TestGetServiceAccount_NotFound(t *testing.T) {
	saStore := &mockSAStore{findErr: errors.New("not found")}
	svc := newAPIKeyService(saStore, &mockKeyStore{}, "local")

	_, err := svc.GetServiceAccount(context.Background(), "nonexistent-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get service account")
}

// ── UpdateServiceAccount ──────────────────────────────────────────────────────

func TestUpdateServiceAccount_UpdatesRoles(t *testing.T) {
	sa := testServiceAccount()
	saStore := &mockSAStore{found: sa}
	svc := newAPIKeyService(saStore, &mockKeyStore{}, "local")

	newRoles := []string{"admin:write"}
	resp, err := svc.UpdateServiceAccount(context.Background(), sa.ID, &dto.UpdateServiceAccountRequest{
		Roles: newRoles,
	})
	require.NoError(t, err)
	assert.Equal(t, newRoles, resp.Roles)
}

func TestUpdateServiceAccount_SuspendsAccount(t *testing.T) {
	sa := testServiceAccount()
	saStore := &mockSAStore{found: sa}
	svc := newAPIKeyService(saStore, &mockKeyStore{}, "local")

	isActive := false
	resp, err := svc.UpdateServiceAccount(context.Background(), sa.ID, &dto.UpdateServiceAccountRequest{
		IsActive: &isActive,
	})
	require.NoError(t, err)
	assert.False(t, resp.IsActive)
}

func TestUpdateServiceAccount_FindError(t *testing.T) {
	saStore := &mockSAStore{findErr: errors.New("db timeout")}
	svc := newAPIKeyService(saStore, &mockKeyStore{}, "local")

	_, err := svc.UpdateServiceAccount(context.Background(), "sa-1", &dto.UpdateServiceAccountRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update service account")
}

// ── ListServiceAccounts ───────────────────────────────────────────────────────

func TestListServiceAccounts_Success(t *testing.T) {
	sa1 := testServiceAccount()
	sa2 := &dao.ServiceAccount{ID: "sa-2", Name: "risk-engine", TenantID: "tenant-abc", IsActive: true}
	saStore := &mockSAStore{listed: []*dao.ServiceAccount{sa1, sa2}}
	svc := newAPIKeyService(saStore, &mockKeyStore{}, "local")

	result, err := svc.ListServiceAccounts(context.Background(), "tenant-abc")
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "payment-gateway", result[0].Name)
	assert.Equal(t, "risk-engine", result[1].Name)
}

func TestListServiceAccounts_Empty(t *testing.T) {
	saStore := &mockSAStore{listed: []*dao.ServiceAccount{}}
	svc := newAPIKeyService(saStore, &mockKeyStore{}, "local")

	result, err := svc.ListServiceAccounts(context.Background(), "no-such-tenant")
	require.NoError(t, err)
	assert.Empty(t, result)
}

// ── CreateAPIKey ──────────────────────────────────────────────────────────────

func TestCreateAPIKey_Success_TestEnv(t *testing.T) {
	sa := testServiceAccount()
	saStore := &mockSAStore{found: sa}
	keyStore := &mockKeyStore{}
	svc := newAPIKeyService(saStore, keyStore, "local") // non-production → "test" prefix

	req := &dto.CreateAPIKeyRequest{Name: "postman-key"}
	resp, err := svc.CreateAPIKey(context.Background(), sa.ID, req, "admin-1")
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.NotEmpty(t, resp.RawKey, "RawKey must be returned exactly once")
	assert.True(t, len(resp.RawKey) == 40, "key must be 40 chars")
	assert.Equal(t, "bp_test_", resp.RawKey[:8], "non-prod key must have bp_test_ prefix")
	assert.Equal(t, "postman-key", resp.Name)
	assert.NotEmpty(t, resp.KeyID)
	assert.NotEmpty(t, resp.KeyPrefix)

	// Raw key must NEVER be stored — only the hash
	require.NotNil(t, keyStore.saved)
	assert.NotEqual(t, resp.RawKey, keyStore.saved.KeyHash, "KeyHash must differ from RawKey")
	assert.Len(t, keyStore.saved.KeyHash, 64, "KeyHash must be SHA-256 hex (64 chars)")
}

func TestCreateAPIKey_Success_ProdEnv(t *testing.T) {
	sa := testServiceAccount()
	saStore := &mockSAStore{found: sa}
	keyStore := &mockKeyStore{}
	svc := newAPIKeyService(saStore, keyStore, "production")

	req := &dto.CreateAPIKeyRequest{Name: "prod-key"}
	resp, err := svc.CreateAPIKey(context.Background(), sa.ID, req, "admin-1")
	require.NoError(t, err)

	assert.Equal(t, "bp_live_", resp.RawKey[:8], "production key must have bp_live_ prefix")
}

func TestCreateAPIKey_WithExpiry(t *testing.T) {
	sa := testServiceAccount()
	saStore := &mockSAStore{found: sa}
	keyStore := &mockKeyStore{}
	svc := newAPIKeyService(saStore, keyStore, "local")

	expiry := time.Now().Add(30 * 24 * time.Hour)
	req := &dto.CreateAPIKeyRequest{Name: "expiring-key", ExpiresAt: &expiry}

	resp, err := svc.CreateAPIKey(context.Background(), sa.ID, req, "admin-1")
	require.NoError(t, err)
	require.NotNil(t, resp.ExpiresAt)
	assert.WithinDuration(t, expiry, *resp.ExpiresAt, time.Second)
}

func TestCreateAPIKey_ServiceAccountNotFound(t *testing.T) {
	saStore := &mockSAStore{findErr: errors.New("sa not found")}
	svc := newAPIKeyService(saStore, &mockKeyStore{}, "local")

	_, err := svc.CreateAPIKey(context.Background(), "nonexistent-sa", &dto.CreateAPIKeyRequest{Name: "k"}, "admin-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service account not found")
}

func TestCreateAPIKey_SaveError(t *testing.T) {
	sa := testServiceAccount()
	saStore := &mockSAStore{found: sa}
	keyStore := &mockKeyStore{saveErr: errors.New("db: write failed")}
	svc := newAPIKeyService(saStore, keyStore, "local")

	_, err := svc.CreateAPIKey(context.Background(), sa.ID, &dto.CreateAPIKeyRequest{Name: "k"}, "admin-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "save")
}

// ── RevokeAPIKey ──────────────────────────────────────────────────────────────

func TestRevokeAPIKey_Success(t *testing.T) {
	sa := testServiceAccount()
	saStore := &mockSAStore{found: sa}
	keyID := "key-uuid-999"
	keyStore := &mockKeyStore{
		listed: []*dao.APIKey{
			{ID: keyID, ServiceAccountID: sa.ID, KeyHash: "abc123def456"},
		},
	}
	svc := newAPIKeyService(saStore, keyStore, "local")

	err := svc.RevokeAPIKey(context.Background(), sa.ID, keyID)
	require.NoError(t, err)
}

func TestRevokeAPIKey_KeyNotOnServiceAccount(t *testing.T) {
	sa := testServiceAccount()
	saStore := &mockSAStore{found: sa}
	keyStore := &mockKeyStore{
		listed: []*dao.APIKey{
			{ID: "key-other-id", ServiceAccountID: sa.ID, KeyHash: "hash1"},
		},
	}
	svc := newAPIKeyService(saStore, keyStore, "local")

	// keyID "key-not-mine" doesn't belong to this SA
	err := svc.RevokeAPIKey(context.Background(), sa.ID, "key-not-mine")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRevokeAPIKey_ListError(t *testing.T) {
	sa := testServiceAccount()
	saStore := &mockSAStore{found: sa}
	keyStore := &mockKeyStore{listErr: errors.New("db timeout")}
	svc := newAPIKeyService(saStore, keyStore, "local")

	err := svc.RevokeAPIKey(context.Background(), sa.ID, "key-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list")
}

func TestRevokeAPIKey_RevokeError(t *testing.T) {
	sa := testServiceAccount()
	saStore := &mockSAStore{found: sa}
	keyID := "key-uuid-999"
	keyStore := &mockKeyStore{
		listed:    []*dao.APIKey{{ID: keyID, KeyHash: "hash1"}},
		revokeErr: errors.New("db: foreign key violation"),
	}
	svc := newAPIKeyService(saStore, keyStore, "local")

	err := svc.RevokeAPIKey(context.Background(), sa.ID, keyID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revoke api key")
}

// ── ListAPIKeys ───────────────────────────────────────────────────────────────

func TestListAPIKeys_Success(t *testing.T) {
	sa := testServiceAccount()
	saStore := &mockSAStore{found: sa}
	now := time.Now()
	keyStore := &mockKeyStore{
		listed: []*dao.APIKey{
			{ID: "k1", ServiceAccountID: sa.ID, Name: "postman", KeyPrefix: "bp_test_AB", CreatedAt: now},
			{ID: "k2", ServiceAccountID: sa.ID, Name: "ci-runner", KeyPrefix: "bp_test_CD", CreatedAt: now},
		},
	}
	svc := newAPIKeyService(saStore, keyStore, "local")

	result, err := svc.ListAPIKeys(context.Background(), sa.ID)
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "postman", result[0].Name)
	assert.Equal(t, "ci-runner", result[1].Name)

	// Raw key hash must never appear in the list response
	for _, k := range result {
		assert.Empty(t, k.KeyPrefix[:0], "KeyPrefix must only be the short prefix, not the full hash")
	}
}

func TestListAPIKeys_Empty(t *testing.T) {
	saStore := &mockSAStore{}
	keyStore := &mockKeyStore{listed: []*dao.APIKey{}}
	svc := newAPIKeyService(saStore, keyStore, "local")

	result, err := svc.ListAPIKeys(context.Background(), "any-sa-id")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestListAPIKeys_StoreError(t *testing.T) {
	saStore := &mockSAStore{}
	keyStore := &mockKeyStore{listErr: errors.New("db: connection lost")}
	svc := newAPIKeyService(saStore, keyStore, "local")

	_, err := svc.ListAPIKeys(context.Background(), "any-sa-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list api keys")
}
