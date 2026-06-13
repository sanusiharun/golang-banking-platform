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
// brief timeout. Audit failure must never surface as a user-visible error —
// call sites should fire-and-forget in a goroutine or ignore the returned error.
type Publisher interface {
	Publish(ctx context.Context, event AuditEvent) error
}
