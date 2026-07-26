package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
)

// Handler processes a message received from the consumer.
// Return nil to acknowledge the message (mark as processed).
// Return an error to NAK the message (triggers redelivery up to MaxDeliver times).
type Handler func(ctx context.Context, subject string, data []byte) error

// ConsumerConfig configures a NATSConsumer.
type ConsumerConfig struct {
	StreamName   string        // JetStream stream name (must already exist)
	ConsumerName string        // durable consumer name — survives service restarts
	Subject      string        // filter subject, e.g. "audit.events.>"
	BatchSize    int           // messages per Fetch call. Default: 10
	MaxWait      time.Duration // Fetch timeout before looping. Default: 2s
	MaxDeliver   int           // max redelivery attempts before dead-lettering. Default: 5
	AckWait      time.Duration // time before NATS redelivers an unacked message. Default: 30s
}

func (c *ConsumerConfig) applyDefaults() {
	if c.BatchSize == 0 {
		c.BatchSize = 10
	}
	if c.MaxWait == 0 {
		c.MaxWait = 2 * time.Second
	}
	if c.MaxDeliver == 0 {
		c.MaxDeliver = 5
	}
	if c.AckWait == 0 {
		c.AckWait = 30 * time.Second
	}
}

// NATSConsumer runs a durable JetStream pull consumer and dispatches messages
// to a Handler. It provisions the durable consumer if it does not yet exist.
type NATSConsumer struct {
	js  nats.JetStreamContext
	cfg ConsumerConfig
}

// NewNATSConsumer creates a NATSConsumer and provisions the durable consumer.
// The stream named by cfg.StreamName must already exist.
func NewNATSConsumer(js nats.JetStreamContext, cfg ConsumerConfig) (*NATSConsumer, error) {
	cfg.applyDefaults()
	c := &NATSConsumer{js: js, cfg: cfg}
	if err := c.ensureConsumer(); err != nil {
		return nil, fmt.Errorf("ensure consumer %q: %w", cfg.ConsumerName, err)
	}
	return c, nil
}

// Start subscribes to the durable consumer and calls handler for each message
// until ctx is cancelled. It blocks; run it in a goroutine.
func (c *NATSConsumer) Start(ctx context.Context, handler Handler) {
	sub, err := c.js.PullSubscribe(c.cfg.Subject, c.cfg.ConsumerName,
		nats.Bind(c.cfg.StreamName, c.cfg.ConsumerName),
	)
	if err != nil {
		slog.Error("messaging: subscribe failed",
			slog.String("consumer", c.cfg.ConsumerName),
			slog.String("error", err.Error()),
		)
		return
	}
	defer sub.Unsubscribe() //nolint:errcheck

	slog.Info("messaging: consumer started", slog.String("consumer", c.cfg.ConsumerName))

	for {
		select {
		case <-ctx.Done():
			slog.Info("messaging: consumer stopping", slog.String("consumer", c.cfg.ConsumerName))
			return
		default:
		}

		msgs, err := sub.Fetch(c.cfg.BatchSize, nats.MaxWait(c.cfg.MaxWait))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) {
				continue
			}
			slog.Error("messaging: fetch error",
				slog.String("consumer", c.cfg.ConsumerName),
				slog.String("error", err.Error()),
			)
			continue
		}

		for _, msg := range msgs {
			if err := handler(ctx, msg.Subject, msg.Data); err != nil {
				slog.Error("messaging: handler error",
					slog.String("subject", msg.Subject),
					slog.String("error", err.Error()),
				)
				_ = msg.Nak() //nolint:errcheck // best-effort NAK; message will be redelivered by NATS on timeout regardless
			} else {
				_ = msg.Ack() //nolint:errcheck // best-effort ACK; a failed ACK just triggers a harmless redelivery
			}
		}
	}
}

func (c *NATSConsumer) ensureConsumer() error {
	_, err := c.js.ConsumerInfo(c.cfg.StreamName, c.cfg.ConsumerName)
	if err == nil {
		return nil
	}

	_, err = c.js.AddConsumer(c.cfg.StreamName, &nats.ConsumerConfig{
		Durable:       c.cfg.ConsumerName,
		AckPolicy:     nats.AckExplicitPolicy,
		MaxDeliver:    c.cfg.MaxDeliver,
		FilterSubject: c.cfg.Subject,
		AckWait:       c.cfg.AckWait,
	})
	if err != nil {
		return fmt.Errorf("add consumer: %w", err)
	}

	slog.Info("messaging: durable consumer created", slog.String("consumer", c.cfg.ConsumerName))
	return nil
}
