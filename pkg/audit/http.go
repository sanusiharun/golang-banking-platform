package audit

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sanusi/banking/pkg/httpclient"
)

// HTTPPublisher sends AuditEvents synchronously via HTTP POST to audit-svc.
// Use this only for compliance-sensitive operations where the caller needs
// confirmation that the audit record was written before proceeding.
// For all other cases prefer NATSPublisher wrapped with Async().
type HTTPPublisher struct {
	client *httpclient.Client
}

// NewHTTPPublisher creates an HTTPPublisher pointing at auditSvcURL
// (e.g. "http://banking-audit-svc:8083").
func NewHTTPPublisher(auditSvcURL string) *HTTPPublisher {
	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = auditSvcURL
	cfg.RequestTimeout = 5 * time.Second
	cfg.MaxRetries = 2
	return &HTTPPublisher{client: httpclient.New(cfg)}
}

// Publish POSTs the event to POST /v1/audit/events and returns any HTTP error.
// Unlike NATSPublisher, errors ARE returned so callers can decide whether to
// retry or continue.
func (p *HTTPPublisher) Publish(ctx context.Context, event AuditEvent) error {
	if err := p.client.Do(ctx, http.MethodPost, "/v1/audit/events", event, nil); err != nil {
		return fmt.Errorf("audit http publish: %w", err)
	}
	return nil
}
