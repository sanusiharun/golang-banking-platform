// Package featureflag provides a lightweight Flipt client with safe fallback.
//
// Design principles:
//   - Initialized once at startup via Init() — no constructor injection needed.
//   - Works like slog: call Init() in main, then use package-level functions anywhere.
//   - If Flipt is unreachable or FLIPT_URL is empty, always returns the default value.
//   - Uses Flipt REST API — no extra SDK dependency, pure stdlib HTTP.
//
// Usage:
//
//	// In container/main — call once at startup:
//	featureflag.Init(cfg.FliptURL, "default")
//
//	// Anywhere in the codebase — no injection needed:
//	if featureflag.IsEnabled(ctx, "maintenance_mode", false) {
//	    return errors.New("service unavailable")
//	}
//
//	hours := featureflag.GetString(ctx, "banking_operation_hours", "")
package featureflag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// ── Global client ─────────────────────────────────────────────────────────────

var global *Client

// Init initializes the global feature flag client.
// Call once at service startup — safe to call with empty url (disables flags).
// If not called, all flag checks return their default values.
func Init(url, namespace string) {
	global = New(url, namespace)
	if url != "" {
		slog.Info("featureflag: initialized", slog.String("url", url), slog.String("namespace", namespace))
	} else {
		slog.Info("featureflag: disabled (FLIPT_URL not set — using defaults)")
	}
}

// IsEnabled evaluates a boolean feature flag using the global client.
// Returns defaultVal if not initialized, Flipt unreachable, or flag missing.
func IsEnabled(ctx context.Context, flagKey string, defaultVal bool) bool {
	return global.IsEnabled(ctx, flagKey, defaultVal)
}

// GetString evaluates a string variant flag using the global client.
// Returns defaultVal if not initialized, Flipt unreachable, or flag missing.
func GetString(ctx context.Context, flagKey string, defaultVal string) string {
	return global.GetString(ctx, flagKey, defaultVal)
}

// ── Client ────────────────────────────────────────────────────────────────────

// Client talks to a Flipt server.
// Prefer the package-level functions (Init/IsEnabled/GetString) over creating
// a Client directly — they use a shared global instance.
type Client struct {
	baseURL    string
	namespace  string
	httpClient *http.Client
}

// New creates a standalone Client. Most callers should use Init + package-level functions instead.
func New(url, namespace string) *Client {
	if namespace == "" {
		namespace = "default"
	}
	return &Client{
		baseURL:   url,
		namespace: namespace,
		httpClient: &http.Client{
			Timeout: 500 * time.Millisecond,
		},
	}
}

// IsEnabled evaluates a boolean feature flag.
// Returns defaultVal if Flipt is unreachable, flag missing, or any error.
func (c *Client) IsEnabled(ctx context.Context, flagKey string, defaultVal bool) bool {
	if c == nil || c.baseURL == "" {
		return defaultVal
	}
	enabled, err := c.evaluateBool(ctx, flagKey)
	if err != nil {
		slog.DebugContext(ctx, "featureflag: bool fallback to default",
			slog.String("flag", flagKey),
			slog.Bool("default", defaultVal),
			slog.String("reason", err.Error()),
		)
		return defaultVal
	}
	return enabled
}

// GetString evaluates a string variant flag.
// Returns defaultVal if Flipt is unreachable, flag missing, or any error.
func (c *Client) GetString(ctx context.Context, flagKey string, defaultVal string) string {
	if c == nil || c.baseURL == "" {
		return defaultVal
	}
	val, err := c.evaluateVariant(ctx, flagKey)
	if err != nil {
		slog.DebugContext(ctx, "featureflag: string fallback to default",
			slog.String("flag", flagKey),
			slog.String("default", defaultVal),
			slog.String("reason", err.Error()),
		)
		return defaultVal
	}
	return val
}

// ── Internal ──────────────────────────────────────────────────────────────────

func (c *Client) evaluateBool(ctx context.Context, flagKey string) (bool, error) {
	url := fmt.Sprintf("%s/api/v1/namespaces/%s/flags/%s", c.baseURL, c.namespace, flagKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("flipt unreachable: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // closing a response body we only read from; nothing to do on failure
	if resp.StatusCode == http.StatusNotFound {
		return false, fmt.Errorf("flag %q not found", flagKey)
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("flipt returned %d", resp.StatusCode)
	}
	var flag struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&flag); err != nil {
		return false, fmt.Errorf("decode response: %w", err)
	}
	return flag.Enabled, nil
}

func (c *Client) evaluateVariant(ctx context.Context, flagKey string) (string, error) {
	url := fmt.Sprintf("%s/evaluate/v1/variant", c.baseURL)
	body, _ := json.Marshal(map[string]any{ //nolint:errcheck // marshaling a static map of known-serializable values cannot fail
		"namespace_key": c.namespace,
		"flag_key":      flagKey,
		"entity_id":     "system",
		"context":       map[string]string{},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("flipt unreachable: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // closing a response body we only read from; nothing to do on failure
	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("flag %q not found", flagKey)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("flipt returned %d", resp.StatusCode)
	}
	var result struct {
		VariantKey string `json:"variantKey"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if result.VariantKey == "" {
		return "", fmt.Errorf("empty variant key for flag %q", flagKey)
	}
	return result.VariantKey, nil
}
