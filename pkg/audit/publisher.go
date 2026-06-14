package audit

import "context"

// Publisher sends an audit event to the audit pipeline.
//
// Implementations:
//   - NATSPublisher  — primary path, async via JetStream (production)
//   - HTTPPublisher  — sync fallback for compliance-sensitive operations
//   - NoopPublisher  — silent drop for tests and local dev without NATS
//
// Contract: Publish must never block the calling goroutine for longer than a
// brief timeout, and audit failure must never surface as a user-visible error.
// Wrap with Async() at the container layer to guarantee non-blocking behaviour
// without goroutine boilerplate at every call site.
type Publisher interface {
	Publish(ctx context.Context, event AuditEvent) error
}
