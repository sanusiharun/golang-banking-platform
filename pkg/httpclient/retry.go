package httpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"math"
	"math/rand"
	"net"
	"time"
)

// shouldRetryError returns true if a network-level error is considered transient
// and retry is configured for it.
func (c *Client) shouldRetryError(cfg *Config, err error) bool {
	if !cfg.RetryEnabled {
		return false
	}

	// Never retry non-transient errors (TLS, DNS resolution failures, etc.)
	if isNonTransientNetworkError(err) {
		return false
	}

	// Context cancelled/deadline exceeded — stop immediately
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Network timeout
	if cfg.RetryOnTimeout && isTimeoutError(err) {
		return true
	}

	return false
}

// shouldRetryStatus returns true if an HTTP status code should trigger a retry.
func (c *Client) shouldRetryStatus(cfg *Config, status int) bool {
	if !cfg.RetryEnabled {
		return false
	}
	// 4xx — never retry (client error, non-transient by definition)
	if status >= 400 && status < 500 {
		return false
	}
	// 5xx — retry only when opted in
	if status >= 500 && cfg.RetryOn5xx {
		return true
	}
	return false
}

// backoff returns the delay duration for the given attempt number (0-indexed).
// attempt=0 → first retry delay, attempt=1 → second, etc.
func backoff(cfg *Config, attempt int) time.Duration {
	if !cfg.BackoffEnabled {
		return cfg.RetryDelay
	}

	// Exponential: delay * multiplier^attempt
	d := float64(cfg.RetryDelay) * math.Pow(cfg.RetryMultiplier, float64(attempt))

	// Cap at MaxDelay
	if cfg.MaxDelay > 0 && d > float64(cfg.MaxDelay) {
		d = float64(cfg.MaxDelay)
	}

	// Full jitter: random value in [d/2, d]
	// Keeps delay bounded but non-deterministic — prevents thundering herd.
	if cfg.JitterEnabled {
		half := d / 2
		d = half + rand.Float64()*half
	}

	return time.Duration(d)
}

// sleep waits for the given duration, returning early if ctx is cancelled.
func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// isTimeoutError returns true if err represents a network timeout.
func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// isNonTransientNetworkError returns true for errors that should never be retried
// regardless of configuration — TLS failures, permanent DNS errors, etc.
func isNonTransientNetworkError(err error) bool {
	// TLS certificate/handshake failures — retrying won't help
	var tlsErr *tls.CertificateVerificationError
	if errors.As(err, &tlsErr) {
		return true
	}

	// DNS: no such host — permanent failure
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return true
	}

	return false
}
