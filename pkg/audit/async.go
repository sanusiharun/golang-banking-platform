package audit

import "context"

// AsyncPublisher wraps any Publisher and makes Publish non-blocking.
// The event is dispatched in a background goroutine; the caller always gets nil.
//
// Wrap at the container layer (buildAuditPublisher) so handler call sites need
// no goroutine or error-ignore boilerplate — just call h.audit.Publish(ctx, event).
type AsyncPublisher struct {
	inner Publisher
}

// Async returns a Publisher that dispatches events in a background goroutine.
func Async(p Publisher) Publisher {
	return &AsyncPublisher{inner: p}
}

// Publish dispatches the event asynchronously and always returns nil.
func (a *AsyncPublisher) Publish(_ context.Context, event AuditEvent) error {
	go func() {
		_ = a.inner.Publish(context.Background(), event) //nolint:errcheck // fire-and-forget audit publish; failure is unrecoverable here and must not block the caller
	}()
	return nil
}
