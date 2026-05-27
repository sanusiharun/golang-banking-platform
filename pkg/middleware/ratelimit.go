package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/sanusi/banking/pkg/httpx"
)

// tokenBucket implements a per-key token bucket rate limiter.
type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

func (tb *tokenBucket) allow() bool {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	if tb.tokens >= 1.0 {
		tb.tokens--
		return true
	}
	return false
}

// RateLimiter holds per-key token buckets.
type RateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]*tokenBucket
	maxTokens  float64
	refillRate float64
	cleanupAge time.Duration
	lastCleanup time.Time
}

// RateLimitConfig configures the token bucket rate limiter.
type RateLimitConfig struct {
	// RequestsPerSecond is the steady-state rate (refill rate).
	RequestsPerSecond float64
	// Burst is the maximum number of requests allowed in a burst.
	Burst float64
	// KeyFunc extracts the rate limit key from a request (e.g. IP address or user ID).
	KeyFunc func(*http.Request) string
}

// NewRateLimiter creates a RateLimiter from the given config.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		buckets:     make(map[string]*tokenBucket),
		maxTokens:   cfg.Burst,
		refillRate:  cfg.RequestsPerSecond,
		cleanupAge:  5 * time.Minute,
		lastCleanup: time.Now(),
	}
}

// RateLimit returns a middleware that enforces rate limits.
// Requests exceeding the limit receive HTTP 429 Too Many Requests.
func RateLimit(cfg RateLimitConfig) func(http.Handler) http.Handler {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = remoteIP
	}
	limiter := NewRateLimiter(cfg)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := cfg.KeyFunc(r)
			if !limiter.Allow(key) {
				w.Header().Set("Retry-After", "1")
				httpx.WriteHTTPError(w, r, httpx.ErrTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Allow checks whether the given key is within the rate limit.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Periodically evict idle buckets to prevent unbounded memory growth.
	if time.Since(rl.lastCleanup) > rl.cleanupAge {
		rl.cleanup()
	}

	bucket, ok := rl.buckets[key]
	if !ok {
		bucket = &tokenBucket{
			tokens:     rl.maxTokens,
			maxTokens:  rl.maxTokens,
			refillRate: rl.refillRate,
			lastRefill: time.Now(),
		}
		rl.buckets[key] = bucket
	}
	return bucket.allow()
}

// cleanup removes buckets that have been idle for cleanupAge.
func (rl *RateLimiter) cleanup() {
	threshold := time.Now().Add(-rl.cleanupAge)
	for key, bucket := range rl.buckets {
		if bucket.lastRefill.Before(threshold) {
			delete(rl.buckets, key)
		}
	}
	rl.lastCleanup = time.Now()
}

// remoteIP extracts the client IP from the request, checking X-Forwarded-For first.
func remoteIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// Take the first (client) IP from the chain.
		for i := 0; i < len(fwd); i++ {
			if fwd[i] == ',' {
				return fwd[:i]
			}
		}
		return fwd
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	// Strip port from RemoteAddr.
	addr := r.RemoteAddr
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}
