// Package push provides a Push Notification channel stub.
// Replace with FCM / APNs SDK in production.
package push

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/sanusi/banking/services/notification-svc/internal/channel"
)

// Provider is the Push Notification channel stub.
type Provider struct{}

func New() *Provider { return &Provider{} }

func (p *Provider) Type() string { return "PUSH" }

func (p *Provider) Send(ctx context.Context, req *channel.SendRequest) (*channel.SendResult, error) {
	ref := uuid.New().String()
	slog.InfoContext(ctx, "push stub: send",
		slog.String("device_token", req.Recipient),
		slog.String("ref", ref),
	)
	return &channel.SendResult{
		ProviderRef:  ref,
		ProviderResp: map[string]any{"stub": true, "device_token": req.Recipient},
	}, nil
}
