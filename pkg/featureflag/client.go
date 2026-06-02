// Package featureflag provides a lightweight Flipt client with safe fallback.
//
// Design principles:
//   - If Flipt is unreachable or FLIPT_URL is empty, methods return the default value.
//     Services never crash because of a missing flag server.
//   - Uses Flipt REST API — no extra SDK dependency, pure stdlib HTTP.
//   - Each service opts in by creating a Client and passing it through its container.
//   - Services that don't need feature flags simply don't create a Client.
//
// Usage:
//
//	client := featureflag.New("http://localhost:8082", "default")
//
//	// Boolean flag
//	if client.IsEnabled(ctx, "maintenance_mode", false) {
//	    return errors.New("service unavailable")
//	}
//
//	// String config
//	hours := client.GetString(ctx, "banking_operation_hours", "07:00-15:00")
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

// Client talks to a Flipt server.
// Use New to create one; use a zero Client if Flipt is not configured.
type Client struct {
	baseURL    string        // e.g. "http://localhost:8082"
	namespace  string        // Flipt namespace (default: "default")
	httpClient *http.Client
}

// New creates a Client pointed at the given Flipt base URL.
// namespace is the Flipt namespace — use "default" if you haven't created one.
// If url is empty the client is disabled and all methods return their defaultVal.
func New(url, namespace string) *Client {
	if namespace == "" {
		namespace = "default"
	}
	return &Client{
		baseURL:   url,
		namespace: namespace,
		httpClient: &http.Client{
			Timeout: 500 * time.Millisecond, // fast timeout — flags must never slow down requests
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
//
// Example use cases:
//   - banking_operation_hours = "07:00-15:00"
//   - max_transfer_amount     = "50000"
//   - supported_currencies    = "MYR,USD,SGD"
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

// evaluateBool calls the Flipt REST API to get a boolean flag state.
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
	defer resp.Body.Close()

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

// evaluateVariant calls Flipt's variant evaluation API to get a string value.
// Flipt variant evaluation requires a POST with entity_id and context.
func (c *Client) evaluateVariant(ctx context.Context, flagKey string) (string, error) {
	url := fmt.Sprintf("%s/evaluate/v1/variant", c.baseURL)

	body, _ := json.Marshal(map[string]any{
		"namespace_key": c.namespace,
		"flag_key":      flagKey,
		"entity_id":     "system", // system-level config, not per-user
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
	defer resp.Body.Close()

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
