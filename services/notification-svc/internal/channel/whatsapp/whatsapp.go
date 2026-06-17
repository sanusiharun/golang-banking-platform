// Package whatsapp provides a WhatsApp channel stub.
// Replace with WhatsApp Business API SDK in production.
package whatsapp

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/sanusi/banking/services/notification-svc/internal/channel"
)

// Provider is the WhatsApp channel stub.
type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Type() string { return "WHATSAPP" }

func (p *Provider) Send(ctx context.Context, req *channel.SendRequest) (*channel.SendResult, error) {
	ref := uuid.New().String()
	slog.InfoContext(ctx, "whatsapp stub: send",
		slog.String("recipient", req.Recipient),
		slog.String("ref", ref),
	)
	return &channel.SendResult{
		ProviderRef:  ref,
		ProviderResp: map[string]any{"stub": true, "recipient": req.Recipient},
	}, nil
}
