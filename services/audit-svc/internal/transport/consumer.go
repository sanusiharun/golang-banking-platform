package transport

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	pkgaudit "github.com/sanusi/banking/pkg/audit"
	"github.com/sanusi/banking/pkg/messaging"
	"github.com/sanusi/banking/services/audit-svc/internal/services"
)

// Consumer subscribes to the AUDIT JetStream stream and persists events
// via the AuditService. Transport plumbing is handled by pkg/messaging.
type Consumer struct {
	inner *messaging.NATSConsumer
	svc   services.AuditService
}

// NewConsumer creates a Consumer and provisions the durable JetStream consumer.
func NewConsumer(js nats.JetStreamContext, consumerName string, svc services.AuditService) (*Consumer, error) {
	inner, err := messaging.NewNATSConsumer(js, messaging.ConsumerConfig{
		StreamName:   "AUDIT",
		ConsumerName: consumerName,
		Subject:      "audit.events.>",
		BatchSize:    10,
		MaxWait:      2 * time.Second,
		MaxDeliver:   5,
		AckWait:      30 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return &Consumer{inner: inner, svc: svc}, nil
}

// Start processes messages until ctx is done. Blocks; run it in a goroutine.
func (c *Consumer) Start(ctx context.Context) {
	c.inner.Start(ctx, c.handle)
}

// handle unmarshals an audit event and ingests it.
// Returns nil (→ ack) for poison-pill messages to prevent infinite redelivery.
// Returns error (→ nak) on ingest failure so NATS retries up to MaxDeliver times.
func (c *Consumer) handle(ctx context.Context, subject string, data []byte) error {
	var event pkgaudit.AuditEvent
	if err := json.Unmarshal(data, &event); err != nil {
		slog.Error("audit consumer: unmarshal failed, discarding",
			slog.String("subject", subject),
			slog.String("error", err.Error()),
		)
		return nil
	}
	return c.svc.Ingest(ctx, event)
}
