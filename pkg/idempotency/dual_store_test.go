package idempotency_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sanusi/banking/pkg/idempotency"
)

// ── fakeStore ─────────────────────────────────────────────────────────────────
//
// fakeStore is a minimal, in-memory Store implementation used to drive DualStore
// tests without real Redis or Postgres. It is intentionally simple and not thread-safe.

type fakeStore struct {
	records       map[string]*idempotency.Record
	acquireErr    error
	completeErr   error
	acquireCalls  int
	completeCalls int
}

func newFakeStore() *fakeStore {
	return &fakeStore{records: make(map[string]*idempotency.Record)}
}

func (f *fakeStore) Acquire(_ context.Context, scopeKey string, _ idempotency.Meta) (*idempotency.Record, error) {
	f.acquireCalls++
	if f.acquireErr != nil {
		return nil, f.acquireErr
	}
	if rec, ok := f.records[scopeKey]; ok {
		if rec.Status == idempotency.StatusProcessing {
			return nil, idempotency.ErrInFlight
		}
		return rec, nil
	}
	f.records[scopeKey] = &idempotency.Record{
		Status:    idempotency.StatusProcessing,
		ScopeKey:  scopeKey,
		CreatedAt: time.Now().Unix(),
	}
	return nil, nil
}

func (f *fakeStore) Complete(_ context.Context, scopeKey string, rec *idempotency.Record) error {
	f.completeCalls++
	if f.completeErr != nil {
		return f.completeErr
	}
	f.records[scopeKey] = rec
	return nil
}

var _ idempotency.Store = (*fakeStore)(nil)

// ── fakeStore self-tests ──────────────────────────────────────────────────────

func TestFakeStore_Acquire_NewKey_ReturnsNil(t *testing.T) {
	store := newFakeStore()
	meta := idempotency.Meta{IdempotencyKey: "k1", CallerID: "sa-1", Method: "POST", Path: "/pay"}
	rec, err := store.Acquire(context.Background(), "scope-1", meta)
	require.NoError(t, err)
	assert.Nil(t, rec, "first Acquire should return nil (acquired lock)")
}

func TestFakeStore_Acquire_ExistingProcessing_ReturnsErrInFlight(t *testing.T) {
	store := newFakeStore()
	meta := idempotency.Meta{IdempotencyKey: "k1", CallerID: "sa-1"}
	_, _ = store.Acquire(context.Background(), "scope-1", meta)
	_, err := store.Acquire(context.Background(), "scope-1", meta)
	assert.ErrorIs(t, err, idempotency.ErrInFlight)
}

func TestFakeStore_Complete_ThenAcquire_ReplaysRecord(t *testing.T) {
	store := newFakeStore()
	ctx := context.Background()
	meta := idempotency.Meta{IdempotencyKey: "k1", CallerID: "sa-1"}
	_, _ = store.Acquire(ctx, "scope-1", meta)

	now := time.Now().Unix()
	completed := &idempotency.Record{
		Status:      idempotency.StatusCompleted,
		StatusCode:  201,
		Body:        []byte(`{"id":"t1"}`),
		CreatedAt:   now,
		CompletedAt: &now,
		ScopeKey:    "scope-1",
	}
	require.NoError(t, store.Complete(ctx, "scope-1", completed))

	replayed, err := store.Acquire(ctx, "scope-1", meta)
	require.NoError(t, err)
	require.NotNil(t, replayed)
	assert.Equal(t, idempotency.StatusCompleted, replayed.Status)
	assert.Equal(t, 201, replayed.StatusCode)
}

// ── DualStore tests ───────────────────────────────────────────────────────────
//
// DualStore composes two Stores (redis + postgres) but takes *RedisStore and
// *PostgresStore as concrete types — those require real infra. The tests here
// exercise the routing logic using the fakeStore indirectly via the Store interface,
// documenting the expected DualStore contract as acceptance tests.
//
// For integration tests that exercise the real Redis+Postgres combination, see
// services/auth-svc/internal/repository/apikey_store_integration_test.go.

// dualFakeStore wraps two fakeStores to simulate DualStore behavior in unit tests.
// We define it here rather than using the real DualStore because DualStore takes
// concrete *RedisStore/*PostgresStore which need real connections.
type dualFakeStore struct {
	primary  *fakeStore // simulates Redis
	fallback *fakeStore // simulates Postgres
}

func newDualFakeStore() *dualFakeStore {
	return &dualFakeStore{
		primary:  newFakeStore(),
		fallback: newFakeStore(),
	}
}

func (d *dualFakeStore) Acquire(ctx context.Context, scopeKey string, meta idempotency.Meta) (*idempotency.Record, error) {
	rec, err := d.primary.Acquire(ctx, scopeKey, meta)
	if err == nil {
		return rec, nil
	}
	if errors.Is(err, idempotency.ErrInFlight) {
		return nil, idempotency.ErrInFlight
	}
	// Primary (Redis) unavailable — fall through to fallback (Postgres)
	return d.fallback.Acquire(ctx, scopeKey, meta)
}

func (d *dualFakeStore) Complete(ctx context.Context, scopeKey string, rec *idempotency.Record) error {
	_ = d.primary.Complete(ctx, scopeKey, rec)
	return d.fallback.Complete(ctx, scopeKey, rec)
}

var _ idempotency.Store = (*dualFakeStore)(nil)

func TestDualStore_HappyPath_PrimaryHandlesRequest(t *testing.T) {
	dual := newDualFakeStore()
	ctx := context.Background()
	meta := idempotency.Meta{IdempotencyKey: "k1", CallerID: "sa-1", Method: "POST", Path: "/pay"}

	rec, err := dual.Acquire(ctx, "scope-1", meta)
	require.NoError(t, err)
	assert.Nil(t, rec)

	assert.Equal(t, 1, dual.primary.acquireCalls)
	assert.Equal(t, 0, dual.fallback.acquireCalls, "fallback must not be called when primary succeeds")
}

func TestDualStore_PrimaryUnavailable_FallbackToPostgres(t *testing.T) {
	dual := newDualFakeStore()
	dual.primary.acquireErr = errors.New("redis: connection refused")
	ctx := context.Background()
	meta := idempotency.Meta{IdempotencyKey: "k1", CallerID: "sa-1"}

	rec, err := dual.Acquire(ctx, "scope-1", meta)
	require.NoError(t, err)
	assert.Nil(t, rec, "should successfully acquire via fallback")

	assert.Equal(t, 1, dual.primary.acquireCalls)
	assert.Equal(t, 1, dual.fallback.acquireCalls, "fallback must be called when primary fails")
}

func TestDualStore_ErrInFlight_PropagatedWithoutFallback(t *testing.T) {
	dual := newDualFakeStore()
	dual.primary.acquireErr = idempotency.ErrInFlight
	ctx := context.Background()
	meta := idempotency.Meta{IdempotencyKey: "k1", CallerID: "sa-1"}

	_, err := dual.Acquire(ctx, "scope-1", meta)
	assert.ErrorIs(t, err, idempotency.ErrInFlight)
	assert.Equal(t, 0, dual.fallback.acquireCalls, "ErrInFlight must NOT fall through to Postgres")
}

func TestDualStore_Complete_WritesBothStores(t *testing.T) {
	dual := newDualFakeStore()
	ctx := context.Background()
	meta := idempotency.Meta{IdempotencyKey: "k1", CallerID: "sa-1"}

	_, _ = dual.Acquire(ctx, "scope-1", meta)

	now := time.Now().Unix()
	rec := &idempotency.Record{
		Status:      idempotency.StatusCompleted,
		StatusCode:  201,
		Body:        []byte(`{}`),
		CreatedAt:   now,
		CompletedAt: &now,
	}
	require.NoError(t, dual.Complete(ctx, "scope-1", rec))

	assert.Equal(t, 1, dual.primary.completeCalls, "primary (Redis) must be written")
	assert.Equal(t, 1, dual.fallback.completeCalls, "fallback (Postgres) must be written")
}

func TestDualStore_PrimaryAcquire_ReplayFromPrimary(t *testing.T) {
	dual := newDualFakeStore()
	ctx := context.Background()
	meta := idempotency.Meta{IdempotencyKey: "k1", CallerID: "sa-1"}

	// First acquire
	_, _ = dual.Acquire(ctx, "scope-1", meta)

	// Complete in primary
	now := time.Now().Unix()
	_ = dual.primary.Complete(ctx, "scope-1", &idempotency.Record{
		Status: idempotency.StatusCompleted, StatusCode: 200, CompletedAt: &now,
	})

	// Second acquire should return the stored record from primary
	replayed, err := dual.Acquire(ctx, "scope-1", meta)
	require.NoError(t, err)
	require.NotNil(t, replayed)
	assert.Equal(t, idempotency.StatusCompleted, replayed.Status)
	assert.Equal(t, 0, dual.fallback.acquireCalls, "fallback not needed when primary has the record")
}

func TestDualStore_PrimaryFails_FallbackDetectsInFlight(t *testing.T) {
	// If Redis is down but Postgres already has a PROCESSING record, ErrInFlight
	// must be returned from Postgres.
	dual := newDualFakeStore()
	ctx := context.Background()
	meta := idempotency.Meta{IdempotencyKey: "k1", CallerID: "sa-1"}

	// Plant a PROCESSING record in fallback (Postgres) as if another request started
	dual.fallback.records["scope-1"] = &idempotency.Record{
		Status:    idempotency.StatusProcessing,
		ScopeKey:  "scope-1",
		CreatedAt: time.Now().Unix(),
	}

	// Make primary (Redis) unavailable
	dual.primary.acquireErr = errors.New("redis: dial timeout")

	_, err := dual.Acquire(ctx, "scope-1", meta)
	assert.ErrorIs(t, err, idempotency.ErrInFlight,
		"Postgres unique-constraint mutex must prevent double-execution when Redis is down")
}

// ── Record contract tests ─────────────────────────────────────────────────────

func TestRecord_StatusConstants(t *testing.T) {
	assert.Equal(t, idempotency.Status("processing"), idempotency.StatusProcessing)
	assert.Equal(t, idempotency.Status("completed"), idempotency.StatusCompleted)
	assert.Equal(t, idempotency.Status("failed"), idempotency.StatusFailed)
}

func TestErrInFlight_SentinelIsDistinct(t *testing.T) {
	other := errors.New("other error")
	assert.False(t, errors.Is(other, idempotency.ErrInFlight))
	assert.True(t, errors.Is(idempotency.ErrInFlight, idempotency.ErrInFlight))
}
