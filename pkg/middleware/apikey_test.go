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

	"github.com/sanusi/banking/pkg/middleware"
)

// ── Mock APIKeyLookup ─────────────────────────────────────────────────────────

type mockAPIKeyLookup struct {
	identity     *middleware.ServiceAccountIdentity
	findErr      error
	updateErr    error
	updateCalled bool
}

func (m *mockAPIKeyLookup) FindActiveByHash(_ context.Context, _ string) (*middleware.ServiceAccountIdentity, error) {
	return m.identity, m.findErr
}

func (m *mockAPIKeyLookup) UpdateLastUsed(_ context.Context, _ string) error {
	m.updateCalled = true
	return m.updateErr
}

var _ middleware.APIKeyLookup = (*mockAPIKeyLookup)(nil)

// ── GenerateAPIKey ────────────────────────────────────────────────────────────

func TestGenerateAPIKey_TestPrefix(t *testing.T) {
	raw, hash, err := middleware.GenerateAPIKey("test")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(raw, "bp_test_"), "expected bp_test_ prefix, got %s", raw)
	assert.Len(t, raw, 40, "expected 40 chars: 8 prefix + 32 random")
	assert.NotEmpty(t, hash)
	// SHA-256 hex is always 64 chars
	assert.Len(t, hash, 64)
}

func TestGenerateAPIKey_LivePrefix(t *testing.T) {
	raw, hash, err := middleware.GenerateAPIKey("live")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(raw, "bp_live_"), "expected bp_live_ prefix, got %s", raw)
	assert.Len(t, raw, 40)
	assert.Len(t, hash, 64)
}

func TestGenerateAPIKey_Uniqueness(t *testing.T) {
	raw1, _, _ := middleware.GenerateAPIKey("test")
	raw2, _, _ := middleware.GenerateAPIKey("test")
	assert.NotEqual(t, raw1, raw2, "two generated keys must not be identical")
}

// ── HashAPIKey ────────────────────────────────────────────────────────────────

func TestHashAPIKey_Deterministic(t *testing.T) {
	raw := "bp_test_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde"
	h1 := middleware.HashAPIKey(raw)
	h2 := middleware.HashAPIKey(raw)
	assert.Equal(t, h1, h2, "HashAPIKey must be deterministic")
	assert.Len(t, h1, 64, "SHA-256 hex is 64 chars")
}

func TestHashAPIKey_DifferentInputs(t *testing.T) {
	assert.NotEqual(t, middleware.HashAPIKey("key-a"), middleware.HashAPIKey("key-b"))
}

func TestHashAPIKey_MatchesGenerateAPIKey(t *testing.T) {
	raw, generatedHash, err := middleware.GenerateAPIKey("test")
	require.NoError(t, err)
	assert.Equal(t, generatedHash, middleware.HashAPIKey(raw))
}

// ── AuthenticateAPIKey ────────────────────────────────────────────────────────

func TestAuthenticateAPIKey(t *testing.T) {
	validIdentity := &middleware.ServiceAccountIdentity{
		ServiceAccountID: "sa-123",
		TenantID:         "tenant-abc",
		Roles:            []string{"payments:write"},
		KeyID:            "key-456",
	}

	tests := []struct {
		name        string
		header      string // Authorization header value
		xAPIKey     string // X-API-Key header value
		env         string // middleware env
		identity    *middleware.ServiceAccountIdentity
		findErr     error
		wantStatus  int
		wantSubject string
	}{
		{
			name:        "valid key via Authorization header",
			header:      "ApiKey bp_test_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde",
			env:         "local",
			identity:    validIdentity,
			wantStatus:  http.StatusOK,
			wantSubject: "sa:sa-123",
		},
		{
			name:        "valid key via X-API-Key header",
			xAPIKey:     "bp_test_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde",
			env:         "local",
			identity:    validIdentity,
			wantStatus:  http.StatusOK,
			wantSubject: "sa:sa-123",
		},
		{
			name:       "missing key",
			env:        "local",
			identity:   validIdentity,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "live key rejected in test env",
			header:     "ApiKey bp_live_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde",
			env:        "local",
			identity:   validIdentity,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "test key accepted in staging env",
			header:     "ApiKey bp_test_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde",
			env:        "staging",
			identity:   validIdentity,
			wantStatus: http.StatusOK,
		},
		{
			name:       "lookup returns error",
			header:     "ApiKey bp_test_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde",
			env:        "local",
			findErr:    errors.New("key not found"),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "live key in production env accepted",
			header:     "ApiKey bp_live_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde",
			env:        "production",
			identity:   validIdentity,
			wantStatus: http.StatusOK,
		},
		{
			name:       "test key rejected in production env",
			header:     "ApiKey bp_test_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde",
			env:        "production",
			identity:   validIdentity,
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookup := &mockAPIKeyLookup{identity: tc.identity, findErr: tc.findErr}
			cfg := middleware.APIKeyConfig{Lookup: lookup, Environment: tc.env}

			var capturedClaims *middleware.Claims
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				claims, _ := middleware.ClaimsFromContext(r.Context())
				capturedClaims = claims
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			if tc.xAPIKey != "" {
				req.Header.Set("X-API-Key", tc.xAPIKey)
			}

			rr := httptest.NewRecorder()
			middleware.AuthenticateAPIKey(cfg)(next).ServeHTTP(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
			if tc.wantSubject != "" {
				require.NotNil(t, capturedClaims)
				assert.Equal(t, tc.wantSubject, capturedClaims.Subject)
				assert.Equal(t, validIdentity.ServiceAccountID, capturedClaims.UserID)
				assert.Equal(t, validIdentity.TenantID, capturedClaims.TenantID)
				assert.Equal(t, validIdentity.Roles, capturedClaims.Roles)
			}
		})
	}
}

func TestAuthenticateAPIKey_ExpiryRejection(t *testing.T) {
	// Key where ExpiresAt is in the past — the lookup should return an error
	// (the repository rejects expired keys). We verify the middleware propagates this.
	lookup := &mockAPIKeyLookup{
		findErr: errors.New("api key has expired"),
	}
	cfg := middleware.APIKeyConfig{Lookup: lookup, Environment: "local"}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "ApiKey bp_test_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde")

	rr := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	middleware.AuthenticateAPIKey(cfg)(next).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

// ── AuthenticateAny ───────────────────────────────────────────────────────────

func TestAuthenticateAny(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		xAPIKey    string
		wantStatus int
	}{
		{
			name:       "no auth header returns 401",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "ApiKey header routes to api key middleware",
			authHeader: "ApiKey bp_test_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde",
			// mock lookup returns error → 401 (API key path was taken)
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "X-API-Key header routes to api key middleware",
			xAPIKey:    "bp_test_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde",
			wantStatus: http.StatusUnauthorized, // lookup fails → 401
		},
		{
			name:       "Bearer header routes to JWT middleware",
			authHeader: "Bearer invalid.jwt.token",
			wantStatus: http.StatusUnauthorized, // JWT validation fails → 401
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookup := &mockAPIKeyLookup{findErr: errors.New("not found")}
			apiKeyCfg := middleware.APIKeyConfig{Lookup: lookup, Environment: "local"}
			// JWT config with nil key — any Bearer token will fail validation, proving JWT path is taken
			jwtCfg := middleware.JWTConfig{PublicKey: nil}

			handler := middleware.AuthenticateAny(jwtCfg, apiKeyCfg)
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			if tc.xAPIKey != "" {
				req.Header.Set("X-API-Key", tc.xAPIKey)
			}

			rr := httptest.NewRecorder()
			handler(next).ServeHTTP(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)
		})
	}
}

// ── IsServiceAccount ──────────────────────────────────────────────────────────

func TestIsServiceAccount(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		want    bool
	}{
		{"service account prefix", "sa:some-uuid", true},
		{"sa prefix only", "sa:", true},
		{"human user", "user-uuid-1234", false},
		{"empty subject", "", false},
		{"sa substring not prefix", "admin:sa:nested", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims := &middleware.Claims{}
			claims.Subject = tc.subject
			assert.Equal(t, tc.want, middleware.IsServiceAccount(claims))
		})
	}
}

// ── ExpiresAt propagation ─────────────────────────────────────────────────────

func TestAuthenticateAPIKey_ExpiresAtPropagated(t *testing.T) {
	// Verify that a key with future ExpiresAt is accepted.
	future := time.Now().Add(1 * time.Hour)
	identity := &middleware.ServiceAccountIdentity{
		ServiceAccountID: "sa-999",
		TenantID:         "t-1",
		Roles:            []string{"read"},
		KeyID:            "key-999",
		ExpiresAt:        &future,
	}
	lookup := &mockAPIKeyLookup{identity: identity}
	cfg := middleware.APIKeyConfig{Lookup: lookup, Environment: "local"}

	var capturedClaims *middleware.Claims
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedClaims, _ = middleware.ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "bp_test_ABCDEFGHIJKLMNOPQRSTUVWXYZabcde")
	rr := httptest.NewRecorder()

	middleware.AuthenticateAPIKey(cfg)(next).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, capturedClaims)
	assert.Equal(t, "sa:sa-999", capturedClaims.Subject)
}
