// Package featureflag provides a lightweight Flipt client with safe fallback.
//
// Design principles:
//   - If Flipt is unreachable or FLIPT_URL is empty, IsEnabled returns the default value.
//     Services never crash because of a missing flag server.
//   - Uses Flipt REST API — no extra SDK dependency, pure stdlib HTTP.
//   - Each service opts in by creating a Client and passing it through its container.
//   - Services that don't need feature flags simply don't create a Client.
//
// Usage:
//
//	client := featureflag.New("http://localhost:8082", "default")
//
//	if client.IsEnabled(ctx, "new_account_response_format", false) {
//	    // new behaviour
//	} else {
//	    // old behaviour
//	}
package featureflag

import (
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
// If url is empty the client is disabled and IsEnabled always returns defaultVal.
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
// Returns defaultVal if:
//   - Flipt URL is not configured
//   - Flipt is unreachable (network error, timeout)
//   - Flag does not exist
//   - Any other error
//
// This makes feature flags safe to deploy before Flipt is running.
func (c *Client) IsEnabled(ctx context.Context, flagKey string, defaultVal bool) bool {
	if c == nil || c.baseURL == "" {
		return defaultVal
	}

	enabled, err := c.evaluate(ctx, flagKey)
	if err != nil {
		slog.DebugContext(ctx, "featureflag: fallback to default",
			slog.String("flag", flagKey),
			slog.Bool("default", defaultVal),
			slog.String("reason", err.Error()),
		)
		return defaultVal
	}
	return enabled
}

// evaluate calls the Flipt REST API to evaluate a boolean flag.
func (c *Client) evaluate(ctx context.Context, flagKey string) (bool, error) {
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
		return false, fmt.Errorf("flag %q not found in namespace %q", flagKey, c.namespace)
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
