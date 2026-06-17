// Package email provides an Email channel stub.
// Replace with a real SMTP / transactional email SDK in production.
package email

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/sanusi/banking/services/notification-svc/internal/channel"
)

// Provider is the Email channel stub.
type Provider struct{}

// New creates an Email channel stub.
func New() *Provider { return &Provider{} }

func (p *Provider) Type() string { return "EMAIL" }

func (p *Provider) Send(ctx context.Context, req *channel.SendRequest) (*channel.SendResult, error) {
	ref := uuid.New().String()
	slog.InfoContext(ctx, "email stub: send",
		slog.String("recipient", req.Recipient),
		slog.String("subject", req.Subject),
		slog.String("ref", ref),
	)
	return &channel.SendResult{
		ProviderRef:  ref,
		ProviderResp: map[string]any{"stub": true, "recipient": req.Recipient},
	}, nil
}
