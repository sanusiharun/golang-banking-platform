// httpclient example — demonstrates real HTTP calls using pkg/httpclient.
//
// Run:
//
//	cd pkg/httpclient/example
//	go run main.go
//
// Requires internet access to httpbin.org (free public HTTP testing API).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/sanusi/banking/pkg/httpclient"
)

func main() {
	fmt.Println("══════════════════════════════════════════════")
	fmt.Println(" httpclient — real sample against httpbin.org")
	fmt.Println("══════════════════════════════════════════════")
	fmt.Println()

	demoGET()
	demoPOST()
	demoRetryOn5xx()
	demoNonTransient4xx()
	demoTimeout()
	demoCustomHeaders()
}

// ── 1. GET ────────────────────────────────────────────────────────────────────

func demoGET() {
	fmt.Println("── 1. GET /get ──────────────────────────────")

	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = "https://httpbin.org"
	client := httpclient.New(cfg)

	var result struct {
		Headers map[string]string `json:"headers"`
		Origin  string            `json:"origin"`
		URL     string            `json:"url"`
	}

	err := client.Do(context.Background(), http.MethodGet, "/get", nil, &result)
	if err != nil {
		log.Printf("  ✗ GET failed: %v\n\n", err)
		return
	}

	fmt.Printf("  ✓ origin : %s\n", result.Origin)
	fmt.Printf("  ✓ url    : %s\n", result.URL)
	fmt.Println()
}

// ── 2. POST with body ─────────────────────────────────────────────────────────

func demoPost() {
	fmt.Println("── 2. POST /post ─────────────────────────────")

	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = "https://httpbin.org"
	client := httpclient.New(cfg)

	payload := map[string]any{
		"account_id": "acc-001",
		"amount":     1000,
		"currency":   "MYR",
	}

	var result struct {
		JSON map[string]any `json:"json"`
		URL  string         `json:"url"`
	}

	err := client.Do(context.Background(), http.MethodPost, "/post", payload, &result)
	if err != nil {
		log.Printf("  ✗ POST failed: %v\n\n", err)
		return
	}

	b, _ := json.MarshalIndent(result.JSON, "  ", "  ")
	fmt.Printf("  ✓ echo'd body: %s\n", b)
	fmt.Println()
}

func demoPOST() { demoPost() }

// ── 3. Retry on 5xx ───────────────────────────────────────────────────────────
// httpbin /status/500 always returns 500. The client retries MaxRetries times,
// then gives up with ErrRetriesExhausted.

func demoRetryOn5xx() {
	fmt.Println("── 3. Retry on 5xx (/status/500) ────────────")

	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = "https://httpbin.org"
	cfg.MaxRetries = 2
	cfg.RetryDelay = 300 * time.Millisecond
	cfg.BackoffEnabled = false // fixed delay so output is predictable
	cfg.JitterEnabled = false
	client := httpclient.New(cfg)

	attempt := 0
	fmt.Printf("  retrying up to %d times...\n", cfg.MaxRetries)
	start := time.Now()

	err := client.Do(context.Background(), http.MethodGet, "/status/500", nil, nil)

	elapsed := time.Since(start).Round(time.Millisecond)
	attempt = cfg.MaxRetries + 1

	if errors.Is(err, httpclient.ErrRetriesExhausted) {
		fmt.Printf("  ✓ ErrRetriesExhausted after %d attempts (%s): %v\n\n", attempt, elapsed, err)
	} else {
		fmt.Printf("  ✗ unexpected error: %v\n\n", err)
	}
}

// ── 4. Non-transient 4xx — never retried ─────────────────────────────────────
// httpbin /status/404 returns 404. Client returns ErrNonTransient immediately,
// no retry regardless of RetryEnabled.

func demoNonTransient4xx() {
	fmt.Println("── 4. Non-transient 4xx (/status/404) ───────")

	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = "https://httpbin.org"
	cfg.MaxRetries = 3 // configured, but should NOT retry
	client := httpclient.New(cfg)

	start := time.Now()
	err := client.Do(context.Background(), http.MethodGet, "/status/404", nil, nil)
	elapsed := time.Since(start).Round(time.Millisecond)

	if errors.Is(err, httpclient.ErrNonTransient) {
		fmt.Printf("  ✓ ErrNonTransient (no retry): %v — took %s\n\n", err, elapsed)
	} else {
		fmt.Printf("  ✗ unexpected error: %v\n\n", err)
	}
}

// ── 5. Per-request timeout override ──────────────────────────────────────────
// httpbin /delay/5 waits 5 seconds before responding.
// We override timeout to 1s — should fail fast with a timeout error.

func demoTimeout() {
	fmt.Println("── 5. Per-request timeout (/delay/5) ────────")

	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = "https://httpbin.org"
	cfg.RetryOnTimeout = false // don't retry — just show the timeout
	client := httpclient.New(cfg)

	start := time.Now()
	err := client.Do(
		context.Background(),
		http.MethodGet,
		"/delay/5",
		nil,
		nil,
		httpclient.WithTimeout(1*time.Second), // override for this call only
	)
	elapsed := time.Since(start).Round(time.Millisecond)

	if err != nil {
		fmt.Printf("  ✓ timed out after %s: %v\n\n", elapsed, err)
	} else {
		fmt.Printf("  ✗ expected timeout but got success\n\n")
	}
}

// ── 6. Custom headers ─────────────────────────────────────────────────────────
// httpbin /headers echoes back all request headers.

func demoCustomHeaders() {
	fmt.Println("── 6. Custom headers (/headers) ─────────────")

	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = "https://httpbin.org"
	client := httpclient.New(cfg)

	var result struct {
		Headers map[string]string `json:"headers"`
	}

	err := client.Do(
		context.Background(),
		http.MethodGet,
		"/headers",
		nil,
		&result,
		httpclient.WithHeader("X-Correlation-ID", "req-abc-123"),
		httpclient.WithHeader("X-Tenant-ID", "banking-platform"),
	)
	if err != nil {
		log.Printf("  ✗ failed: %v\n\n", err)
		return
	}

	fmt.Printf("  ✓ X-Correlation-ID : %s\n", result.Headers["X-Correlation-Id"])
	fmt.Printf("  ✓ X-Tenant-ID      : %s\n", result.Headers["X-Tenant-Id"])
	fmt.Println()
}
