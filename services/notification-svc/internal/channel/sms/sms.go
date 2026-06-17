// Package sms provides an SMS channel stub.
// Replace with a real SMS gateway SDK (Twilio, Vonage, etc.) in production.
package sms

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/sanusi/banking/services/notification-svc/internal/channel"
)

// Provider is the SMS channel stub.
type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Type() string { return "SMS" }

func (p *Provider) Send(ctx context.Context, req *channel.SendRequest) (*channel.SendResult, error) {
	ref := uuid.New().String()
	slog.InfoContext(ctx, "sms stub: send",
		slog.String("recipient", req.Recipient),
		slog.String("ref", ref),
	)
	return &channel.SendResult{
		ProviderRef:  ref,
		ProviderResp: map[string]any{"stub": true, "recipient": req.Recipient},
	}, nil
}
