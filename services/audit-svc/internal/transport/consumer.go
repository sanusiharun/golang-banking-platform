package transport

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	pkgaudit "github.com/sanusi/banking/pkg/audit"
	"github.com/sanusi/banking/services/audit-svc/internal/services"
)

const (
	// maxDeliver is how many times NATS will retry delivering a message before
	// moving it to a dead-letter subject. Mirrors the plan spec.
	maxDeliver = 5
)

// Consumer subscribes to the AUDIT JetStream stream and persists events
// via the AuditService. It runs in a goroutine until the context is cancelled.
type Consumer struct {
	js       nats.JetStreamContext
	consumer string // durable consumer name
	svc      services.AuditService
}

// NewConsumer creates a Consumer and provisions the durable JetStream consumer
// if it does not already exist.
func NewConsumer(js nats.JetStreamContext, consumerName string, svc services.AuditService) (*Consumer, error) {
	c := &Consumer{js: js, consumer: consumerName, svc: svc}
	if err := c.ensureConsumer(); err != nil {
		return nil, err
	}
	return c, nil
}

// Start subscribes to the durable consumer and processes messages until ctx is done.
// It blocks; run it in a goroutine.
func (c *Consumer) Start(ctx context.Context) {
	sub, err := c.js.PullSubscribe("audit.events.>", c.consumer,
		nats.Bind("AUDIT", c.consumer),
	)
	if err != nil {
		slog.Error("audit consumer: subscribe failed", slog.String("error", err.Error()))
		return
	}
	defer sub.Unsubscribe() //nolint:errcheck

	slog.Info("audit consumer: started", slog.String("consumer", c.consumer))

	for {
		select {
		case <-ctx.Done():
			slog.Info("audit consumer: stopping (context cancelled)")
			return
		default:
		}

		msgs, err := sub.Fetch(10, nats.MaxWait(2*time.Second))
		if err != nil {
			if err == nats.ErrTimeout {
				continue // no messages available, loop again
			}
			slog.Error("audit consumer: fetch error", slog.String("error", err.Error()))
			continue
		}

		for _, msg := range msgs {
			if err := c.process(ctx, msg); err != nil {
				slog.Error("audit consumer: process error",
					slog.String("subject", msg.Subject),
					slog.String("error", err.Error()),
				)
				// NAK to trigger redelivery up to maxDeliver times.
				_ = msg.Nak()
			} else {
				_ = msg.Ack()
			}
		}
	}
}

// process unmarshals a NATS message and calls Ingest.
func (c *Consumer) process(ctx context.Context, msg *nats.Msg) error {
	var event pkgaudit.AuditEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		// Malformed message — ack to avoid infinite redelivery.
		slog.Error("audit consumer: unmarshal failed, discarding message",
			slog.String("subject", msg.Subject),
			slog.String("error", err.Error()),
		)
		_ = msg.Ack()
		return nil
	}

	if err := c.svc.Ingest(ctx, event); err != nil {
		return err
	}
	return nil
}

// ensureConsumer creates the durable pull consumer if it does not exist.
func (c *Consumer) ensureConsumer() error {
	_, err := c.js.ConsumerInfo("AUDIT", c.consumer)
	if err == nil {
		return nil // already exists
	}

	_, err = c.js.AddConsumer("AUDIT", &nats.ConsumerConfig{
		Durable:       c.consumer,
		AckPolicy:     nats.AckExplicitPolicy,
		MaxDeliver:    maxDeliver,
		FilterSubject: "audit.events.>",
		AckWait:       30 * time.Second,
	})
	if err != nil {
		return err
	}

	slog.Info("audit consumer: durable consumer created", slog.String("consumer", c.consumer))
	return nil
}
