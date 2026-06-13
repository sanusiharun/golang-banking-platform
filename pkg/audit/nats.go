package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	streamName    = "AUDIT"
	streamSubject = "audit.events.>"
	streamMaxAge  = 7 * 24 * time.Hour
)

// NATSPublisher publishes AuditEvents to NATS JetStream.
// It is the primary transport path: callers publish fire-and-forget and the
// message is durably delivered to audit-svc via the AUDIT stream.
//
// Errors from Publish are logged but never returned — audit failure must not
// block user-facing operations.
type NATSPublisher struct {
	js nats.JetStreamContext
}

// NewNATSPublisher creates a NATSPublisher and ensures the AUDIT stream exists.
// It is safe to call on every service startup — AddStream is idempotent.
func NewNATSPublisher(nc *nats.Conn) (*NATSPublisher, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("get jetstream context: %w", err)
	}

	if err := ensureStream(js); err != nil {
		return nil, fmt.Errorf("ensure AUDIT stream: %w", err)
	}

	return &NATSPublisher{js: js}, nil
}

// Publish marshals the event to JSON and publishes it asynchronously to
// "audit.events.<action>". The caller must not block waiting for the ack.
func (p *NATSPublisher) Publish(_ context.Context, event AuditEvent) error {
	b, err := json.Marshal(event)
	if err != nil {
		slog.Error("audit: marshal event", slog.String("action", event.Action), slog.String("error", err.Error()))
		return nil // don't propagate — audit failure must not block callers
	}

	if _, err := p.js.PublishAsync(event.NATSSubject(), b); err != nil {
		slog.Error("audit: publish to nats",
			slog.String("subject", event.NATSSubject()),
			slog.String("error", err.Error()),
		)
		// Intentionally swallowed — callers treat audit as best-effort.
	}
	return nil
}

// EnsureStream is exported so audit-svc container.go can call it directly
// on startup to guarantee the stream exists before the consumer subscribes.
func EnsureStream(js nats.JetStreamContext) error {
	return ensureStream(js)
}

// ensureStream creates the AUDIT JetStream stream if it does not already exist.
// Configuration mirrors the plan: file storage, 7-day max age, limits retention.
func ensureStream(js nats.JetStreamContext) error {
	_, err := js.StreamInfo(streamName)
	if err == nil {
		return nil // already exists
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return fmt.Errorf("check stream info: %w", err)
	}

	_, err = js.AddStream(&nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{streamSubject},
		Retention: nats.LimitsPolicy,
		MaxAge:    streamMaxAge,
		Storage:   nats.FileStorage,
		Replicas:  1,
	})
	if err != nil {
		return fmt.Errorf("add stream: %w", err)
	}

	slog.Info("audit: NATS stream created", slog.String("stream", streamName))
	return nil
}
