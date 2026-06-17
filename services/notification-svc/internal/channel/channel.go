// Package channel defines the extensible notification channel abstraction.
// Adding a new channel requires implementing the Channel interface and
// registering it with the Registry — no changes to core routing logic needed.
package channel

import "context"

// Channel dispatches a rendered notification to a specific delivery mechanism.
type Channel interface {
	// Type returns the channel identifier (e.g. "EMAIL", "SMS").
	Type() string
	// Send dispatches the notification. Returns a SendResult on success.
	Send(ctx context.Context, req *SendRequest) (*SendResult, error)
}

// SendRequest carries the rendered notification payload to the channel provider.
type SendRequest struct {
	Recipient string
	Subject   string
	Body      string
	Metadata  map[string]any
}

// SendResult holds the provider's response after a successful Send.
type SendResult struct {
	ProviderRef  string
	ProviderResp map[string]any
}
