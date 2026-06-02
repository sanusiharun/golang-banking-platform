// Package authclient provides an HTTP client for calling auth-svc from account-svc.
//
// Uses pkg/httpclient under the hood — inherits retry, backoff, jitter, and
// error classification automatically.
//
// Usage:
//
//	client := authclient.New("http://localhost:8080")
//	info, err := client.Inspect(ctx, token)
//	if err != nil { ... }
//	if !info.HasRole("ADMIN", "TELLER") {
//	    return errors.New("insufficient role")
//	}
package authclient

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sanusi/banking/pkg/httpclient"
)

// Client calls auth-svc endpoints.
type Client struct {
	http *httpclient.Client
}

// New creates an authclient pointed at authSvcURL (e.g. "http://localhost:8080").
// Uses conservative defaults: short timeout, limited retries — auth-svc should be fast.
func New(authSvcURL string) *Client {
	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = authSvcURL
	cfg.ConnectTimeout = 2 * time.Second
	cfg.RequestTimeout = 5 * time.Second
	cfg.MaxRetries = 2
	cfg.RetryOn5xx = true
	cfg.RetryOnTimeout = true

	return &Client{http: httpclient.New(cfg)}
}

// ── Request / Response types ──────────────────────────────────────────────────

type inspectRequest struct {
	Token string `json:"token"`
}

// UserInfo holds the decoded token claims returned by auth-svc /auth/inspect.
type UserInfo struct {
	UserID    string   `json:"user_id"`
	TenantID  string   `json:"tenant_id"`
	Roles     []string `json:"roles"`
	Issuer    string   `json:"issuer"`
	IssuedAt  string   `json:"issued_at"`
	ExpiresAt string   `json:"expires_at"`
	Valid     bool     `json:"valid"`
	Expired   bool     `json:"expired"`
}

// HasRole returns true if the user has at least one of the given roles.
func (u *UserInfo) HasRole(roles ...string) bool {
	for _, userRole := range u.Roles {
		for _, required := range roles {
			if userRole == required {
				return true
			}
		}
	}
	return false
}

// auth-svc wraps all responses in {"success":true,"data":{...}}
type inspectResponse struct {
	Success bool     `json:"success"`
	Data    UserInfo `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ── Methods ───────────────────────────────────────────────────────────────────

// Inspect calls POST /auth/inspect on auth-svc with the given JWT token.
// Returns UserInfo with decoded claims.
//
// Errors:
//   - ErrUnauthorized if the token is invalid or expired
//   - httpclient.ErrNonTransient for 4xx responses
//   - httpclient.ErrRetriesExhausted if auth-svc is temporarily unavailable
func (c *Client) Inspect(ctx context.Context, token string) (*UserInfo, error) {
	var resp inspectResponse

	err := c.http.Do(ctx, http.MethodPost, "/auth/inspect", inspectRequest{Token: token}, &resp)
	if err != nil {
		return nil, fmt.Errorf("authclient.Inspect: %w", err)
	}

	if !resp.Success || resp.Error != nil {
		code := ""
		msg := "unknown error"
		if resp.Error != nil {
			code = resp.Error.Code
			msg = resp.Error.Message
		}
		return nil, fmt.Errorf("authclient.Inspect: auth-svc error [%s]: %s", code, msg)
	}

	if !resp.Data.Valid || resp.Data.Expired {
		return nil, ErrUnauthorized
	}

	return &resp.Data, nil
}

// ErrUnauthorized is returned when the token is invalid or expired.
var ErrUnauthorized = fmt.Errorf("authclient: token is invalid or expired")
