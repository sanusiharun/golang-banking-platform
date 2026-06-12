package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sanusi/banking/pkg/idempotency"
	"github.com/sanusi/banking/pkg/middleware"
)

// ── fakeIdempotencyStore ──────────────────────────────────────────────────────

// fakeIdempotencyStore is an in-memory implementation of idempotency.Store.
// Not thread-safe — each test case uses its own instance.
type fakeIdempotencyStore struct {
	records       map[string]*idempotency.Record
	acquireErr    error
	completeErr   error
	completeCalls int
}

func newFakeIdempotencyStore() *fakeIdempotencyStore {
	return &fakeIdempotencyStore{records: make(map[string]*idempotency.Record)}
}

func (f *fakeIdempotencyStore) Acquire(_ context.Context, scopeKey string, _ idempotency.Meta) (*idempotency.Record, error) {
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

func (f *fakeIdempotencyStore) Complete(_ context.Context, scopeKey string, rec *idempotency.Record) error {
	f.completeCalls++
	if f.completeErr != nil {
		return f.completeErr
	}
	f.records[scopeKey] = rec
	return nil
}

var _ idempotency.Store = (*fakeIdempotencyStore)(nil)

// ── helpers ───────────────────────────────────────────────────────────────────

// serviceAccountRequest builds a POST request pre-loaded with service account Claims.
// It does this by running the request through AuthenticateAPIKey with a mock lookup,
// which correctly sets "sa:<id>" in Subject — exactly what Idempotency middleware checks.
func serviceAccountRequest(method, path, saID string) *http.Request {
	identity := &middleware.ServiceAccountIdentity{
		ServiceAccountID: saID,
		TenantID:         "tenant-test",
		Roles:            []string{"payments:write"},
		KeyID:            "key-test-001",
	}
	lookup := &mockAPIKeyLookup{identity: identity}
	cfg := middleware.APIKeyConfig{Lookup: lookup, Environment: "local"}

	inner := httptest.NewRequest(method, path, nil)
	inner.Header.Set("X-API-Key", "bp_test_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde")

	var injected *http.Request
	capturer := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		injected = r
	})
	rr := httptest.NewRecorder()
	middleware.AuthenticateAPIKey(cfg)(capturer).ServeHTTP(rr, inner)

	if injected == nil {
		panic("serviceAccountRequest: AuthenticateAPIKey rejected the test request")
	}
	return injected
}

// idempotencyMW builds the Idempotency middleware with the given store.
func idempotencyMW(store idempotency.Store) func(http.Handler) http.Handler {
	return middleware.Idempotency(middleware.IdempotencyConfig{
		Store: store,
		TTL:   time.Minute,
	})
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestIdempotency_SafeMethodsPassThrough(t *testing.T) {
	tests := []string{
		http.MethodGet, http.MethodDelete, http.MethodHead, http.MethodOptions,
	}
	for _, method := range tests {
		t.Run(method, func(t *testing.T) {
			store := newFakeIdempotencyStore()
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

			req := httptest.NewRequest(method, "/payments", nil)
			rr := httptest.NewRecorder()
			idempotencyMW(store)(next).ServeHTTP(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code, "safe method %s should pass through", method)
			assert.Equal(t, 0, store.completeCalls)
		})
	}
}

func TestIdempotency_NoClaimsInContext_PassesThrough(t *testing.T) {
	// Unauthenticated request (no Claims in context) — middleware skips idempotency.
	store := newFakeIdempotencyStore()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/payments", nil)
	rr := httptest.NewRecorder()
	idempotencyMW(store)(next).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, 0, store.completeCalls)
}

func TestIdempotency_MissingIdempotencyKeyHeader(t *testing.T) {
	// Service account caller on a mutating method with no Idempotency-Key → 400.
	store := newFakeIdempotencyStore()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := serviceAccountRequest(http.MethodPost, "/payments", "sa-123")
	rr := httptest.NewRecorder()
	idempotencyMW(store)(next).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "MISSING_IDEMPOTENCY_KEY")
}

func TestIdempotency_IdempotencyKeyTooLong(t *testing.T) {
	store := newFakeIdempotencyStore()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := serviceAccountRequest(http.MethodPost, "/payments", "sa-123")
	req.Header.Set("Idempotency-Key", strings.Repeat("x", 256))
	rr := httptest.NewRecorder()
	idempotencyMW(store)(next).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "INVALID_IDEMPOTENCY_KEY")
}

func TestIdempotency_MaxLengthKeyAccepted(t *testing.T) {
	store := newFakeIdempotencyStore()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) })

	req := serviceAccountRequest(http.MethodPost, "/payments", "sa-123")
	req.Header.Set("Idempotency-Key", strings.Repeat("x", 255))
	rr := httptest.NewRecorder()
	idempotencyMW(store)(next).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
}

func TestIdempotency_FirstRequest_ExecutesHandler(t *testing.T) {
	store := newFakeIdempotencyStore()
	handlerCalled := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled++
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"txn-1"}`))
	})

	req := serviceAccountRequest(http.MethodPost, "/payments", "sa-111")
	req.Header.Set("Idempotency-Key", "key-first-001")
	rr := httptest.NewRecorder()
	idempotencyMW(store)(next).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.Equal(t, 1, handlerCalled)
	assert.Equal(t, 1, store.completeCalls, "Complete must be called exactly once")
	assert.Contains(t, rr.Body.String(), "txn-1")
}

func TestIdempotency_DuplicateRequest_ReplaysWithoutExecuting(t *testing.T) {
	store := newFakeIdempotencyStore()
	handlerCalled := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled++
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"txn-2"}`))
	})

	saID := "sa-222"
	idempKey := "key-dup-002"

	// First request — executes handler, stores result
	req1 := serviceAccountRequest(http.MethodPost, "/payments", saID)
	req1.Header.Set("Idempotency-Key", idempKey)
	rr1 := httptest.NewRecorder()
	idempotencyMW(store)(next).ServeHTTP(rr1, req1)
	require.Equal(t, http.StatusCreated, rr1.Code)

	// Second request — must replay stored response
	req2 := serviceAccountRequest(http.MethodPost, "/payments", saID)
	req2.Header.Set("Idempotency-Key", idempKey)
	rr2 := httptest.NewRecorder()
	idempotencyMW(store)(next).ServeHTTP(rr2, req2)

	assert.Equal(t, http.StatusCreated, rr2.Code)
	assert.Equal(t, 1, handlerCalled, "handler must NOT execute on replay")
	assert.Equal(t, "true", rr2.Header().Get("Idempotency-Replay"))
	assert.NotEmpty(t, rr2.Header().Get("Idempotency-Key-Expires"))
	assert.Contains(t, rr2.Body.String(), "txn-2")
}

func TestIdempotency_InFlight_Returns409(t *testing.T) {
	store := newFakeIdempotencyStore()
	store.acquireErr = idempotency.ErrInFlight

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	req := serviceAccountRequest(http.MethodPost, "/payments", "sa-333")
	req.Header.Set("Idempotency-Key", "key-inflight-003")
	rr := httptest.NewRecorder()
	idempotencyMW(store)(next).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code)
	assert.Contains(t, rr.Body.String(), "IDEMPOTENCY_IN_FLIGHT")
}

func TestIdempotency_StoreError_DegradesGracefully(t *testing.T) {
	// When the store returns an unexpected error, the middleware should degrade
	// gracefully: proceed with the request and NOT block the caller.
	store := newFakeIdempotencyStore()
	store.acquireErr = errors.New("redis: connection refused")

	handlerCalled := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled++
		w.WriteHeader(http.StatusCreated)
	})

	req := serviceAccountRequest(http.MethodPost, "/payments", "sa-444")
	req.Header.Set("Idempotency-Key", "key-degrade-004")
	rr := httptest.NewRecorder()
	idempotencyMW(store)(next).ServeHTTP(rr, req)

	// Handler should execute — degraded mode, no idempotency protection
	assert.Equal(t, http.StatusCreated, rr.Code)
	assert.Equal(t, 1, handlerCalled)
}

func TestIdempotency_ScopeIsolation_DifferentCallers(t *testing.T) {
	// Same Idempotency-Key from two different service accounts MUST NOT share a record.
	store := newFakeIdempotencyStore()
	idempKey := "shared-key"

	makeNext := func(body string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(body))
		})
	}

	req1 := serviceAccountRequest(http.MethodPost, "/payments", "caller-one")
	req1.Header.Set("Idempotency-Key", idempKey)
	rr1 := httptest.NewRecorder()
	idempotencyMW(store)(makeNext(`{"caller":"one"}`)).ServeHTTP(rr1, req1)

	req2 := serviceAccountRequest(http.MethodPost, "/payments", "caller-two")
	req2.Header.Set("Idempotency-Key", idempKey)
	rr2 := httptest.NewRecorder()
	idempotencyMW(store)(makeNext(`{"caller":"two"}`)).ServeHTTP(rr2, req2)

	// Both should have gotten their own response, not each other's
	assert.Contains(t, rr1.Body.String(), "one")
	assert.Contains(t, rr2.Body.String(), "two")
	// No Idempotency-Replay header on either — both were first-time requests
	assert.Empty(t, rr1.Header().Get("Idempotency-Replay"))
	assert.Empty(t, rr2.Header().Get("Idempotency-Replay"))
}

func TestIdempotency_FailedResponse_StoredAndReplayed(t *testing.T) {
	// A 4xx response must be stored (not re-executed) on duplicate requests.
	store := newFakeIdempotencyStore()
	handlerCalled := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"INVALID_AMOUNT"}`))
	})

	saID := "sa-fail"
	idempKey := "key-fail-005"

	req1 := serviceAccountRequest(http.MethodPost, "/payments", saID)
	req1.Header.Set("Idempotency-Key", idempKey)
	rr1 := httptest.NewRecorder()
	idempotencyMW(store)(next).ServeHTTP(rr1, req1)
	require.Equal(t, http.StatusBadRequest, rr1.Code)

	req2 := serviceAccountRequest(http.MethodPost, "/payments", saID)
	req2.Header.Set("Idempotency-Key", idempKey)
	rr2 := httptest.NewRecorder()
	idempotencyMW(store)(next).ServeHTTP(rr2, req2)

	assert.Equal(t, http.StatusBadRequest, rr2.Code)
	assert.Equal(t, 1, handlerCalled, "handler must not re-execute for failed responses")
	assert.Equal(t, "true", rr2.Header().Get("Idempotency-Replay"))
	assert.Contains(t, rr2.Body.String(), "INVALID_AMOUNT")
}

func TestIdempotency_FirstResponse_HasExpiresHeader(t *testing.T) {
	store := newFakeIdempotencyStore()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	})

	req := serviceAccountRequest(http.MethodPost, "/transfers", "sa-expires")
	req.Header.Set("Idempotency-Key", "key-expires-006")
	rr := httptest.NewRecorder()
	idempotencyMW(store)(next).ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	assert.NotEmpty(t, rr.Header().Get("Idempotency-Key-Expires"),
		"first response must include Idempotency-Key-Expires header")
}
