package httpclient

import "errors"

// Sentinel errors — use errors.Is to branch on these.
//
//	var result MyResponse
//	err := client.Do(ctx, "GET", "/users/1", nil, &result)
//	if errors.Is(err, httpclient.ErrNonTransient) {
//	    // 4xx or TLS — don't retry at call site either
//	}
//	if errors.Is(err, httpclient.ErrRetriesExhausted) {
//	    // gave up after MaxRetries transient failures
//	}
var (
	// ErrNonTransient wraps errors that must never be retried:
	// HTTP 4xx responses, TLS failures, request encoding errors.
	ErrNonTransient = errors.New("non-transient error")

	// ErrRetriesExhausted is returned when the retry budget runs out on transient errors.
	ErrRetriesExhausted = errors.New("retries exhausted")
)
