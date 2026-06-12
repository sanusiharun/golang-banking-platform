// Package httpclient provides a reusable, resilient HTTP client for calling external APIs.
//
// Features:
//   - Generic Do(ctx, method, path, body, out, opts...) for any JSON endpoint
//   - Configurable retry with exponential backoff + jitter
//   - Error classification: non-transient (4xx, TLS) vs transient (5xx, timeout)
//   - Separate connect and request timeouts via http.Transport + http.Client.Timeout
//   - Per-request customization via functional options (WithHeader, WithTimeout)
//   - Sentinel errors: ErrNonTransient, ErrRetriesExhausted
//
// Usage:
//
//	client := httpclient.New(httpclient.Config{
//	    BaseURL:         "https://api.example.com",
//	    ConnectTimeout:  5 * time.Second,
//	    RequestTimeout:  10 * time.Second,
//	    MaxRetries:      3,
//	    RetryDelay:      200 * time.Millisecond,
//	    RetryMultiplier: 2.0,
//	    MaxDelay:        5 * time.Second,
//	    RetryEnabled:    true,
//	    RetryOn5xx:      true,
//	    RetryOnTimeout:  true,
//	    BackoffEnabled:  true,
//	    JitterEnabled:   true,
//	})
//
//	var result MyResponse
//	err := client.Do(ctx, http.MethodGet, "/users/123", nil, &result,
//	    httpclient.WithHeader("X-Request-ID", requestID),
//	)
//	if errors.Is(err, httpclient.ErrNonTransient) { ... }
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Config holds all configuration for the HTTP client.
// Only BaseURL is required — all other fields fall back to DefaultConfig() values
// when zero. Call DefaultConfig() to start from sensible defaults, then override
// only what you need.
type Config struct {
	// BaseURL is prepended to every path in Do(). Required.
	BaseURL string

	// ConnectTimeout limits the TCP handshake.
	// Default: 5s
	ConnectTimeout time.Duration

	// RequestTimeout limits the full round-trip (connect + send + read response).
	// Applied via http.Client.Timeout.
	// Default: 30s
	RequestTimeout time.Duration

	// Retry configuration
	MaxRetries      int           // max retry attempts. Default: 3
	RetryDelay      time.Duration // base delay between retries. Default: 200ms
	RetryMultiplier float64       // exponential multiplier. Default: 2.0
	MaxDelay        time.Duration // cap on backoff delay. Default: 10s

	// Resilience toggles — all enabled by default
	RetryEnabled   bool // master switch. Default: true
	RetryOn5xx     bool // retry HTTP 5xx. Default: true
	RetryOnTimeout bool // retry network timeouts. Default: true
	BackoffEnabled bool // exponential backoff. Default: true
	JitterEnabled  bool // add jitter to backoff. Default: true
}

// DefaultConfig returns a Config with sensible production-ready defaults.
// Override only the fields you need:
//
//	cfg := httpclient.DefaultConfig()
//	cfg.BaseURL = "https://api.example.com"
//	cfg.MaxRetries = 5      // more retries for critical path
//	cfg.RetryOn5xx = false  // no retry for this particular client
//	client := httpclient.New(cfg)
func DefaultConfig() Config {
	return Config{
		ConnectTimeout:  5 * time.Second,
		RequestTimeout:  30 * time.Second,
		MaxRetries:      3,
		RetryDelay:      200 * time.Millisecond,
		RetryMultiplier: 2.0,
		MaxDelay:        10 * time.Second,
		RetryEnabled:    true,
		RetryOn5xx:      true,
		RetryOnTimeout:  true,
		BackoffEnabled:  true,
		JitterEnabled:   true,
	}
}

// Client is a reusable HTTP client. Safe for concurrent use.
// Create with New and reuse across requests — do not copy.
type Client struct {
	cfg  Config
	http *http.Client
}

// New creates a Client from the given Config.
// Zero values are replaced with defaults from DefaultConfig().
// The underlying http.Client and Transport are configured once and reused.
func New(cfg Config) *Client {
	defaults := DefaultConfig()

	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = defaults.ConnectTimeout
	}
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = defaults.RequestTimeout
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = defaults.MaxRetries
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = defaults.RetryDelay
	}
	if cfg.RetryMultiplier == 0 {
		cfg.RetryMultiplier = defaults.RetryMultiplier
	}
	if cfg.MaxDelay == 0 {
		cfg.MaxDelay = defaults.MaxDelay
	}

	dialer := &net.Dialer{
		Timeout: cfg.ConnectTimeout,
	}

	transport := &http.Transport{
		DialContext:         dialer.DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   cfg.RequestTimeout,
	}

	return &Client{cfg: cfg, http: httpClient}
}

// Do executes an HTTP request and decodes the JSON response into out.
//
//   - method: HTTP method (GET, POST, PUT, DELETE, PATCH)
//   - path:   appended to BaseURL (e.g. "/users/123")
//   - body:   request body, JSON-marshaled. Pass nil for GET/DELETE.
//   - out:    pointer to decode response into. Pass nil to ignore response body.
//   - opts:   per-request functional options (WithHeader, WithTimeout)
//
// Error behavior:
//   - HTTP 4xx → returns ErrNonTransient (never retried)
//   - HTTP 5xx → retried if RetryOn5xx && RetryEnabled, else returns error
//   - Network timeout → retried if RetryOnTimeout && RetryEnabled
//   - TLS/DNS failure → returns ErrNonTransient (never retried)
//   - ctx cancelled → stops immediately, returns ctx.Err()
//   - All retries exhausted → returns ErrRetriesExhausted
func (c *Client) Do(ctx context.Context, method, path string, body any, out any, opts ...Option) error {
	ro := applyOptions(opts)

	// Determine effective timeout for this request
	timeout := c.cfg.RequestTimeout
	if ro.timeout > 0 {
		timeout = ro.timeout
	}

	// Build base URL
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/" + strings.TrimLeft(path, "/")

	maxAttempts := 1
	if c.cfg.RetryEnabled && c.cfg.MaxRetries > 0 {
		maxAttempts = 1 + c.cfg.MaxRetries
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Respect context before each attempt
		if err := ctx.Err(); err != nil {
			return err
		}

		// Build a fresh request each attempt (body reader is consumed)
		req, cancel, err := c.buildRequest(ctx, method, url, body, ro.headers, timeout)
		if err != nil {
			// Request construction failure is always non-transient
			return fmt.Errorf("%w: build request: %w", ErrNonTransient, err)
		}

		resp, err := c.http.Do(req)
		cancel() // release per-request context resources
		if err != nil {
			// Network-level error
			if isNonTransientNetworkError(err) {
				return fmt.Errorf("%w: %w", ErrNonTransient, err)
			}
			if c.shouldRetryError(&c.cfg, err) && attempt < maxAttempts-1 {
				lastErr = err
				if sleepErr := sleep(ctx, backoff(&c.cfg, attempt)); sleepErr != nil {
					return sleepErr
				}
				continue
			}
			return err
		}

		// Read and close body
		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		// HTTP 409 with IDEMPOTENCY_IN_FLIGHT — transient; retry with fixed 100ms delay.
		// All other 409s remain non-retryable (e.g. duplicate username conflict).
		if resp.StatusCode == http.StatusConflict && strings.Contains(string(respBody), "IDEMPOTENCY_IN_FLIGHT") {
			if attempt < maxAttempts-1 {
				if sleepErr := sleep(ctx, 100*time.Millisecond); sleepErr != nil {
					return sleepErr
				}
				continue
			}
			return fmt.Errorf("%w: HTTP 409 IDEMPOTENCY_IN_FLIGHT after %d attempts", ErrRetriesExhausted, maxAttempts)
		}

		// HTTP 4xx — non-transient, fail immediately
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return fmt.Errorf("%w: HTTP %d: %s", ErrNonTransient, resp.StatusCode, summarize(respBody))
		}

		// HTTP 5xx — retry if configured
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, summarize(respBody))
			if c.shouldRetryStatus(&c.cfg, resp.StatusCode) {
				if attempt < maxAttempts-1 {
					if sleepErr := sleep(ctx, backoff(&c.cfg, attempt)); sleepErr != nil {
						return sleepErr
					}
					continue
				}
				// Retries enabled but budget exhausted
				return fmt.Errorf("%w: %w", ErrRetriesExhausted, lastErr)
			}
			return lastErr
		}

		// Success — decode response
		if readErr != nil {
			return fmt.Errorf("read response body: %w", readErr)
		}
		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("%w: decode response: %w", ErrNonTransient, err)
			}
		}
		return nil
	}

	return fmt.Errorf("%w: %w", ErrRetriesExhausted, lastErr)
}

// buildRequest constructs an *http.Request with optional body, headers, and per-request timeout.
// The cancel function (if non-nil) must be called by the caller after the request completes.
func (c *Client) buildRequest(
	ctx context.Context,
	method, url string,
	body any,
	headers map[string]string,
	timeout time.Duration,
) (*http.Request, context.CancelFunc, error) {
	// Apply per-request timeout via a child context.
	// Cancel is returned to the caller — must be called after http.Client.Do returns.
	reqCtx := ctx
	var cancel context.CancelFunc = func() {} // no-op default
	if timeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, timeout)
	}

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			cancel()
		return nil, nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(reqCtx, method, url, bodyReader)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return req, cancel, nil
}

// summarize returns the first 200 bytes of a response body for error messages.
func summarize(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "..."
	}
	return string(b)
}
