// Package webhook provides a Webhook channel that HTTP POSTs the notification
// body to the recipient URL. This is the only non-stub channel in v1.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/sanusi/banking/services/notification-svc/internal/channel"
)

// Provider dispatches notifications via HTTP POST to the recipient URL.
type Provider struct {
	client *http.Client
}

// New creates a Webhook provider with a default 10s timeout.
func New() *Provider {
	return &Provider{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *Provider) Type() string { return "WEBHOOK" }

func (p *Provider) Send(ctx context.Context, req *channel.SendRequest) (*channel.SendResult, error) {
	payload := map[string]any{
		"body":     req.Body,
		"subject":  req.Subject,
		"metadata": req.Metadata,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("webhook: marshal payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.Recipient, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("webhook: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("webhook: http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("webhook: recipient returned %d", resp.StatusCode)
	}

	ref := uuid.New().String()
	return &channel.SendResult{
		ProviderRef:  ref,
		ProviderResp: map[string]any{"status_code": resp.StatusCode, "url": req.Recipient},
	}, nil
}
