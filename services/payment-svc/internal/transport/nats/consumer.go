// Package nats contains the NATS JetStream consumer for async payment processing.
// E6 implementation: scheduled payments, retry queue, external integrations.
package nats

import (
	"context"
	"log/slog"

	"github.com/nats-io/nats.go"
)

// Consumer subscribes to the PAYMENTS stream for async payment messages.
type Consumer struct {
	js       nats.JetStreamContext
	consumer string
}

// NewConsumer creates a Consumer. The consumer group name prevents duplicate
// delivery when multiple payment-svc instances are running.
func NewConsumer(js nats.JetStreamContext, consumerName string) *Consumer {
	return &Consumer{js: js, consumer: consumerName}
}

// Start begins consuming messages from the PAYMENTS.schedule.> subject.
// Blocks until ctx is cancelled.
func (c *Consumer) Start(ctx context.Context) {
	slog.Info("nats consumer starting", "consumer", c.consumer)

	sub, err := c.js.QueueSubscribeSync("payments.schedule.>", c.consumer)
	if err != nil {
		slog.Error("nats consumer: subscribe failed", "error", err)
		return
	}
	defer sub.Unsubscribe() //nolint:errcheck

	for {
		select {
		case <-ctx.Done():
			slog.Info("nats consumer: shutting down")
			return
		default:
			msg, err := sub.NextMsgWithContext(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				slog.Warn("nats consumer: next message error", "error", err)
				continue
			}
			c.handle(ctx, msg)
		}
	}
}

// handle processes a single inbound message.
// TODO (E6-T02): route to the scheduled payment handler.
func (c *Consumer) handle(ctx context.Context, msg *nats.Msg) {
	slog.InfoContext(ctx, "nats consumer: received message",
		"subject", msg.Subject,
		"size", len(msg.Data),
	)
	// Acknowledge so the message is not redelivered.
	if err := msg.Ack(); err != nil {
		slog.WarnContext(ctx, "nats consumer: ack failed", "error", err)
	}
}
