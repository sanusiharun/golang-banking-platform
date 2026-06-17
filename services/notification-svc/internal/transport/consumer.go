package transport

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/sanusi/banking/pkg/messaging"
	"github.com/sanusi/banking/services/notification-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/notification-svc/internal/services"
)

// Consumer subscribes to the NOTIFICATIONS JetStream stream and creates
// notification records via NotificationService for async processing.
type Consumer struct {
	inner *messaging.NATSConsumer
	svc   services.NotificationService
}

// NewConsumer creates a Consumer and provisions the durable JetStream consumer.
func NewConsumer(js nats.JetStreamContext, consumerName string, svc services.NotificationService) (*Consumer, error) {
	inner, err := messaging.NewNATSConsumer(js, messaging.ConsumerConfig{
		StreamName:   "NOTIFICATIONS",
		ConsumerName: consumerName,
		Subject:      "notifications.requests.>",
		BatchSize:    20,
		MaxWait:      2 * time.Second,
		MaxDeliver:   5,
		AckWait:      30 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return &Consumer{inner: inner, svc: svc}, nil
}

// Start processes messages until ctx is done. Blocks; run in a goroutine.
func (c *Consumer) Start(ctx context.Context) {
	c.inner.Start(ctx, c.handle)
}

// handle unmarshals the SendNotificationRequest and creates a notification record.
// Returns nil (→ ACK) for malformed messages to prevent infinite redelivery.
func (c *Consumer) handle(ctx context.Context, subject string, data []byte) error {
	var req dto.SendNotificationRequest
	if err := json.Unmarshal(data, &req); err != nil {
		slog.ErrorContext(ctx, "notification consumer: unmarshal failed, discarding",
			slog.String("subject", subject),
			slog.String("error", err.Error()),
		)
		return nil
	}

	_, err := c.svc.Send(ctx, &req)
	if err != nil {
		slog.ErrorContext(ctx, "notification consumer: send failed",
			slog.String("subject", subject),
			slog.String("channel", req.Channel),
			slog.String("error", err.Error()),
		)
		return err // NAK → NATS retries up to MaxDeliver
	}
	return nil
}
