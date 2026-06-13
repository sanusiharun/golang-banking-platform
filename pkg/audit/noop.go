package audit

import "context"

// NoopPublisher silently discards all events.
// Use in unit tests and in local dev when NATS is not running.
type NoopPublisher struct{}

// Publish implements Publisher and always returns nil.
func (NoopPublisher) Publish(_ context.Context, _ AuditEvent) error { return nil }
