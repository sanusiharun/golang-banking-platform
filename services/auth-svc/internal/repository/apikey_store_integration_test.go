//go:build integration

// Run with: go test -tags integration ./services/auth-svc/internal/repository/...
//
// Required environment variables:
//   TEST_DB_DSN   — Postgres DSN, e.g. "postgres://user:pass@localhost:5432/testdb?sslmode=disable"
//   TEST_REDIS_ADDR — Redis address, e.g. "localhost:6379"
//
// Both are optional: if absent the relevant sub-tests are skipped.

package repository_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
	"github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/auth-svc/internal/repository"
)

// ── Infrastructure helpers ────────────────────────────────────────────────────

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("TEST_DB_DSN not set — skipping postgres integration tests")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "connect to test postgres")

	// Auto-migrate test tables
	err = db.AutoMigrate(
		&dao.ServiceAccount{},
		&dao.APIKey{},
	)
	require.NoError(t, err, "auto-migrate test tables")

	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM api_keys").Error
		_ = db.Exec("DELETE FROM service_accounts").Error
	})

	return db
}

func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set — skipping redis integration tests")
	}

	client := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	require.NoError(t, client.Ping(ctx).Err(), "redis must be reachable")

	t.Cleanup(func() {
		_ = client.FlushDB(context.Background()).Err()
		_ = client.Close()
	})

	return client
}

// newTestSA inserts a service account and returns it.
func newTestSA(t *testing.T, saStore repository.ServiceAccountStore, name string) *dao.ServiceAccount {
	t.Helper()
	sa := &dao.ServiceAccount{
		ID:        fmt.Sprintf("sa-%d", time.Now().UnixNano()),
		Name:      name,
		TenantID:  "tenant-integration",
		Roles:     dao.StringArray{"payments:write"},
		IsActive:  true,
		CreatedBy: "test",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, saStore.Save(context.Background(), sa))
	return sa
}

// newTestKey creates a key and persists it, returning both the DAO and the raw key.
func newTestKey(t *testing.T, keyStore repository.APIKeyStore, saID string, expiresAt *time.Time) (raw string, key *dao.APIKey) {
	t.Helper()
	var hash string
	var err error
	raw, hash, err = pkgmiddleware.GenerateAPIKey("test")
	require.NoError(t, err)

	prefix := raw
	if len(raw) > 10 {
		prefix = raw[:10]
	}

	key = &dao.APIKey{
		ID:               fmt.Sprintf("key-%d", time.Now().UnixNano()),
		ServiceAccountID: saID,
		Name:             "test-key",
		KeyHash:          hash,
		KeyPrefix:        prefix,
		ExpiresAt:        expiresAt,
		CreatedBy:        "test",
		CreatedAt:        time.Now(),
	}
	require.NoError(t, keyStore.Save(context.Background(), key))
	return raw, key
}

// ── PostgresAPIKeyStore integration tests ─────────────────────────────────────

func TestPostgresAPIKeyStore_SaveAndFindActiveByHash(t *testing.T) {
	db := testDB(t)
	saStore := repository.NewPostgresServiceAccountStore(db)
	keyStore := repository.NewPostgresAPIKeyStore(db)

	sa := newTestSA(t, saStore, "integration-sa")
	raw, key := newTestKey(t, keyStore, sa.ID, nil) // no expiry

	hash := pkgmiddleware.HashAPIKey(raw)

	identity, err := keyStore.FindActiveByHash(context.Background(), hash)
	require.NoError(t, err)
	require.NotNil(t, identity)

	assert.Equal(t, sa.ID, identity.ServiceAccountID)
	assert.Equal(t, sa.TenantID, identity.TenantID)
	assert.Equal(t, []string{"payments:write"}, identity.Roles)
	assert.Equal(t, key.ID, identity.KeyID)
	assert.Nil(t, identity.ExpiresAt)
}

func TestPostgresAPIKeyStore_FindByHash_NotFound(t *testing.T) {
	db := testDB(t)
	keyStore := repository.NewPostgresAPIKeyStore(db)

	_, err := keyStore.FindActiveByHash(context.Background(), "nonexistent-hash")
	assert.ErrorIs(t, err, repository.ErrAPIKeyNotFound)
}

func TestPostgresAPIKeyStore_FindByHash_ExpiredKey(t *testing.T) {
	db := testDB(t)
	saStore := repository.NewPostgresServiceAccountStore(db)
	keyStore := repository.NewPostgresAPIKeyStore(db)

	sa := newTestSA(t, saStore, "expired-key-sa")
	past := time.Now().Add(-1 * time.Hour)
	raw, _ := newTestKey(t, keyStore, sa.ID, &past)
	hash := pkgmiddleware.HashAPIKey(raw)

	_, err := keyStore.FindActiveByHash(context.Background(), hash)
	assert.ErrorIs(t, err, repository.ErrAPIKeyExpired,
		"FindActiveByHash must return ErrAPIKeyExpired for keys with expires_at in the past")
}

func TestPostgresAPIKeyStore_Revoke(t *testing.T) {
	db := testDB(t)
	saStore := repository.NewPostgresServiceAccountStore(db)
	keyStore := repository.NewPostgresAPIKeyStore(db)

	sa := newTestSA(t, saStore, "revoke-sa")
	raw, key := newTestKey(t, keyStore, sa.ID, nil)
	hash := pkgmiddleware.HashAPIKey(raw)

	// Key is active before revoke
	identity, err := keyStore.FindActiveByHash(context.Background(), hash)
	require.NoError(t, err)
	require.NotNil(t, identity)

	// Revoke it
	require.NoError(t, keyStore.Revoke(context.Background(), key.ID, hash))

	// Key must no longer be found
	_, err = keyStore.FindActiveByHash(context.Background(), hash)
	assert.ErrorIs(t, err, repository.ErrAPIKeyNotFound,
		"revoked key must not be found by FindActiveByHash")
}

func TestPostgresAPIKeyStore_ListByServiceAccount(t *testing.T) {
	db := testDB(t)
	saStore := repository.NewPostgresServiceAccountStore(db)
	keyStore := repository.NewPostgresAPIKeyStore(db)

	sa := newTestSA(t, saStore, "list-keys-sa")
	_, key1 := newTestKey(t, keyStore, sa.ID, nil)
	_, key2 := newTestKey(t, keyStore, sa.ID, nil)

	keys, err := keyStore.ListByServiceAccount(context.Background(), sa.ID)
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	ids := []string{keys[0].ID, keys[1].ID}
	assert.Contains(t, ids, key1.ID)
	assert.Contains(t, ids, key2.ID)
}

func TestPostgresAPIKeyStore_FindByHash_InactiveSA(t *testing.T) {
	db := testDB(t)
	saStore := repository.NewPostgresServiceAccountStore(db)
	keyStore := repository.NewPostgresAPIKeyStore(db)

	sa := newTestSA(t, saStore, "inactive-sa")
	raw, _ := newTestKey(t, keyStore, sa.ID, nil)

	// Suspend the service account
	sa.IsActive = false
	sa.UpdatedAt = time.Now()
	require.NoError(t, db.Save(sa).Error)

	hash := pkgmiddleware.HashAPIKey(raw)
	_, err := keyStore.FindActiveByHash(context.Background(), hash)
	assert.ErrorIs(t, err, repository.ErrAPIKeyNotFound,
		"keys for inactive service accounts must not be resolved")
}

func TestPostgresAPIKeyStore_UpdateLastUsed(t *testing.T) {
	db := testDB(t)
	saStore := repository.NewPostgresServiceAccountStore(db)
	keyStore := repository.NewPostgresAPIKeyStore(db)

	sa := newTestSA(t, saStore, "last-used-sa")
	_, key := newTestKey(t, keyStore, sa.ID, nil)

	before := time.Now().Add(-time.Second)
	require.NoError(t, keyStore.UpdateLastUsed(context.Background(), key.ID))

	var updated dao.APIKey
	require.NoError(t, db.First(&updated, "id = ?", key.ID).Error)
	require.NotNil(t, updated.LastUsedAt)
	assert.True(t, updated.LastUsedAt.After(before), "last_used_at must be updated")
}

// ── RedisAPIKeyStore integration tests ────────────────────────────────────────

func TestRedisAPIKeyStore_CacheHit(t *testing.T) {
	db := testDB(t)
	redisClient := testRedis(t)

	saStore := repository.NewPostgresServiceAccountStore(db)
	pgKeyStore := repository.NewPostgresAPIKeyStore(db)
	cachedStore := repository.NewRedisAPIKeyStore(pgKeyStore, redisClient)

	sa := newTestSA(t, saStore, "redis-cache-sa")
	raw, _ := newTestKey(t, cachedStore, sa.ID, nil)
	hash := pkgmiddleware.HashAPIKey(raw)

	// First call — cache miss, populates Redis
	identity1, err := cachedStore.FindActiveByHash(context.Background(), hash)
	require.NoError(t, err)
	require.NotNil(t, identity1)

	// Verify Redis now holds the entry
	cacheKey := "apikey:" + hash
	cached, err := redisClient.Get(context.Background(), cacheKey).Bytes()
	require.NoError(t, err, "Redis should have cached the result")
	assert.NotEmpty(t, cached)

	// Second call — must hit cache (we can't easily count DB queries, but
	// we verify the returned identity is identical)
	identity2, err := cachedStore.FindActiveByHash(context.Background(), hash)
	require.NoError(t, err)
	assert.Equal(t, identity1.ServiceAccountID, identity2.ServiceAccountID)
	assert.Equal(t, identity1.KeyID, identity2.KeyID)
}

func TestRedisAPIKeyStore_CacheMiss_FallsBackToPostgres(t *testing.T) {
	db := testDB(t)
	redisClient := testRedis(t)

	saStore := repository.NewPostgresServiceAccountStore(db)
	pgKeyStore := repository.NewPostgresAPIKeyStore(db)
	cachedStore := repository.NewRedisAPIKeyStore(pgKeyStore, redisClient)

	sa := newTestSA(t, saStore, "redis-miss-sa")
	raw, _ := newTestKey(t, pgKeyStore, sa.ID, nil) // save directly to PG, not through cache
	hash := pkgmiddleware.HashAPIKey(raw)

	// Cache is empty — should fall back to Postgres and succeed
	identity, err := cachedStore.FindActiveByHash(context.Background(), hash)
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, sa.ID, identity.ServiceAccountID)
}

func TestRedisAPIKeyStore_Revoke_InvalidatesCache(t *testing.T) {
	db := testDB(t)
	redisClient := testRedis(t)

	saStore := repository.NewPostgresServiceAccountStore(db)
	pgKeyStore := repository.NewPostgresAPIKeyStore(db)
	cachedStore := repository.NewRedisAPIKeyStore(pgKeyStore, redisClient)

	sa := newTestSA(t, saStore, "revoke-cache-sa")
	raw, key := newTestKey(t, cachedStore, sa.ID, nil)
	hash := pkgmiddleware.HashAPIKey(raw)

	// Populate cache
	_, err := cachedStore.FindActiveByHash(context.Background(), hash)
	require.NoError(t, err)

	// Revoke — must flush cache
	require.NoError(t, cachedStore.Revoke(context.Background(), key.ID, hash))

	// Cache entry must be gone
	cacheKey := "apikey:" + hash
	_, err = redisClient.Get(context.Background(), cacheKey).Bytes()
	assert.ErrorIs(t, err, redis.Nil, "Redis cache entry must be deleted on revoke")

	// FindActiveByHash must now return not found
	_, err = cachedStore.FindActiveByHash(context.Background(), hash)
	assert.ErrorIs(t, err, repository.ErrAPIKeyNotFound)
}
