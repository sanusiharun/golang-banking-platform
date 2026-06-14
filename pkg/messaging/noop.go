package messaging

import "context"

// NoopPublisher silently discards all messages.
// Use in unit tests and local dev without NATS.
type NoopPublisher struct{}

func (NoopPublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }
