// Package messaging provides generic NATS JetStream publisher and consumer
// primitives for inter-service communication within the banking platform.
// Services build typed wrappers (e.g. pkg/audit) on top of these primitives
// rather than depending on NATS directly.
package messaging

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go"
)

// Publisher sends raw bytes to a NATS subject.
type Publisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
}

// NATSPublisher publishes messages to NATS JetStream asynchronously.
// The caller is responsible for ensuring any required streams exist before
// publishing — stream provisioning belongs in the service layer, not here.
type NATSPublisher struct {
	js nats.JetStreamContext
}

// NewNATSPublisher creates a NATSPublisher from an existing NATS connection.
func NewNATSPublisher(nc *nats.Conn) (*NATSPublisher, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("get jetstream context: %w", err)
	}
	return &NATSPublisher{js: js}, nil
}

// NewNATSPublisherFromJS creates a NATSPublisher from an existing JetStream context.
// Prefer this when the caller already holds a JetStreamContext to avoid a second
// nc.JetStream() call.
func NewNATSPublisherFromJS(js nats.JetStreamContext) *NATSPublisher {
	return &NATSPublisher{js: js}
}

// Publish sends payload to subject via JetStream async publish.
// The context is accepted for interface compatibility; JetStream async publish
// is inherently non-blocking and does not use the context.
func (p *NATSPublisher) Publish(_ context.Context, subject string, payload []byte) error {
	_, err := p.js.PublishAsync(subject, payload)
	return err
}
