package httpclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newTestClient(server *httptest.Server, overrides ...func(*Config)) *Client {
	cfg := DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.RetryDelay = 10 * time.Millisecond // fast retries in tests
	cfg.MaxDelay = 50 * time.Millisecond
	cfg.BackoffEnabled = false
	cfg.JitterEnabled = false
	for _, o := range overrides {
		o(&cfg)
	}
	return New(cfg)
}

func jsonHandler(status int, body any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(body)
	}
}

// ── GET success ───────────────────────────────────────────────────────────────

func TestDo_GetSuccess(t *testing.T) {
	type response struct {
		Name string `json:"name"`
	}

	srv := httptest.NewServer(jsonHandler(http.StatusOK, response{Name: "banking"}))
	defer srv.Close()

	client := newTestClient(srv)

	var out response
	err := client.Do(context.Background(), http.MethodGet, "/test", nil, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Name != "banking" {
		t.Errorf("got name=%q, want %q", out.Name, "banking")
	}
}

// ── POST with body ────────────────────────────────────────────────────────────

func TestDo_PostWithBody(t *testing.T) {
	type request struct {
		Amount int `json:"amount"`
	}
	type response struct {
		Received int `json:"received"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		json.NewDecoder(r.Body).Decode(&req)
		json.NewEncoder(w).Encode(response{Received: req.Amount})
	}))
	defer srv.Close()

	client := newTestClient(srv)

	var out response
	err := client.Do(context.Background(), http.MethodPost, "/", request{Amount: 1000}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Received != 1000 {
		t.Errorf("got received=%d, want 1000", out.Received)
	}
}

// ── 4xx — ErrNonTransient, never retried ─────────────────────────────────────

func TestDo_4xx_NonTransient(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := newTestClient(srv, func(c *Config) {
		c.MaxRetries = 3
		c.RetryOn5xx = true
	})

	err := client.Do(context.Background(), http.MethodGet, "/", nil, nil)
	if !errors.Is(err, ErrNonTransient) {
		t.Errorf("expected ErrNonTransient, got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected exactly 1 call (no retry), got %d", calls.Load())
	}
}

// ── 5xx with retry — ErrRetriesExhausted ─────────────────────────────────────

func TestDo_5xx_RetryExhausted(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(srv, func(c *Config) {
		c.MaxRetries = 2
		c.RetryEnabled = true
		c.RetryOn5xx = true
	})

	err := client.Do(context.Background(), http.MethodGet, "/", nil, nil)
	if !errors.Is(err, ErrRetriesExhausted) {
		t.Errorf("expected ErrRetriesExhausted, got %v", err)
	}
	// 1 initial + 2 retries = 3 total calls
	if calls.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}
}

// ── 5xx without RetryOn5xx — fails immediately ────────────────────────────────

func TestDo_5xx_NoRetryWhenDisabled(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(srv, func(c *Config) {
		c.MaxRetries = 3
		c.RetryEnabled = true
		c.RetryOn5xx = false // disabled
	})

	err := client.Do(context.Background(), http.MethodGet, "/", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call (no retry), got %d", calls.Load())
	}
}

// ── Retry succeeds on second attempt ─────────────────────────────────────────

func TestDo_5xx_RetrySucceeds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	client := newTestClient(srv, func(c *Config) {
		c.MaxRetries = 3
		c.RetryEnabled = true
		c.RetryOn5xx = true
	})

	var out map[string]string
	err := client.Do(context.Background(), http.MethodGet, "/", nil, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls (1 fail + 1 success), got %d", calls.Load())
	}
}

// ── Context cancellation stops retries immediately ────────────────────────────

func TestDo_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(srv, func(c *Config) {
		c.MaxRetries = 10
		c.RetryEnabled = true
		c.RetryOn5xx = true
		c.RetryDelay = 1 * time.Second // long enough to be cancelled
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := client.Do(ctx, http.MethodGet, "/", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── WithHeader per-request option ────────────────────────────────────────────

func TestDo_WithHeader(t *testing.T) {
	var receivedHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("X-Tenant-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := newTestClient(srv)
	client.Do(context.Background(), http.MethodGet, "/", nil, nil,
		WithHeader("X-Tenant-ID", "banking-platform"),
	)

	if receivedHeader != "banking-platform" {
		t.Errorf("got header=%q, want %q", receivedHeader, "banking-platform")
	}
}

// ── WithTimeout per-request override ─────────────────────────────────────────

func TestDo_WithTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // slow response
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := newTestClient(srv, func(c *Config) {
		c.RequestTimeout = 0   // no client-level timeout
		c.RetryOnTimeout = false
		c.RetryEnabled = false
	})

	err := client.Do(context.Background(), http.MethodGet, "/", nil, nil,
		WithTimeout(50*time.Millisecond), // tight per-request timeout
	)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

// ── DefaultConfig has sensible values ────────────────────────────────────────

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ConnectTimeout != 5*time.Second {
		t.Errorf("ConnectTimeout=%v, want 5s", cfg.ConnectTimeout)
	}
	if cfg.RequestTimeout != 30*time.Second {
		t.Errorf("RequestTimeout=%v, want 30s", cfg.RequestTimeout)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries=%d, want 3", cfg.MaxRetries)
	}
	if !cfg.RetryEnabled {
		t.Error("RetryEnabled should be true by default")
	}
	if !cfg.RetryOn5xx {
		t.Error("RetryOn5xx should be true by default")
	}
	if !cfg.BackoffEnabled {
		t.Error("BackoffEnabled should be true by default")
	}
	if !cfg.JitterEnabled {
		t.Error("JitterEnabled should be true by default")
	}
}

// ── New() applies defaults for zero values ────────────────────────────────────

func TestNew_AppliesDefaults(t *testing.T) {
	cfg := Config{BaseURL: "http://example.com"} // all zero except BaseURL
	client := New(cfg)

	if client.cfg.RetryDelay == 0 {
		t.Error("RetryDelay should have default applied")
	}
	if client.cfg.RetryMultiplier == 0 {
		t.Error("RetryMultiplier should have default applied")
	}
	if client.cfg.MaxRetries == 0 {
		t.Error("MaxRetries should have default applied")
	}
}
