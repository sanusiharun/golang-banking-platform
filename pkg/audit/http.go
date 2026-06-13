package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// HTTPPublisher sends AuditEvents synchronously via HTTP POST to audit-svc.
// Use this only for compliance-sensitive operations where the caller needs
// confirmation that the audit record was written before proceeding.
// For all other cases prefer NATSPublisher.
type HTTPPublisher struct {
	auditSvcURL string
	client      *http.Client
}

// NewHTTPPublisher creates an HTTPPublisher pointing at auditSvcURL
// (e.g. "http://banking-audit-svc:8083").
func NewHTTPPublisher(auditSvcURL string) *HTTPPublisher {
	return &HTTPPublisher{
		auditSvcURL: auditSvcURL,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Publish POSTs the event to POST /v1/audit/events and returns any HTTP error.
// Unlike NATSPublisher, errors ARE returned so callers can decide whether to
// retry or continue.
func (p *HTTPPublisher) Publish(ctx context.Context, event AuditEvent) error {
	b, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.auditSvcURL+"/v1/audit/events", bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("create audit request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("send audit event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("audit-svc returned %d", resp.StatusCode)
	}
	return nil
}
