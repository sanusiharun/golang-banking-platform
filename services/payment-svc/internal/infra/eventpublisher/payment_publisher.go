// Package eventpublisher wraps pkg/messaging to publish typed payment lifecycle events.
package eventpublisher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/sanusi/banking/pkg/messaging"
)

const (
	subjectCompleted  = "payments.completed"
	subjectFailed     = "payments.failed"
	subjectReversed   = "payments.reversed"
	subjectCancelled  = "payments.cancelled"
	subjectProcessing = "payments.processing"
)

// PaymentEventPublisher publishes payment lifecycle events to NATS.
type PaymentEventPublisher interface {
	PublishCompleted(ctx context.Context, event PaymentEvent) error
	PublishFailed(ctx context.Context, event PaymentEvent) error
	PublishReversed(ctx context.Context, event PaymentEvent) error
	PublishCancelled(ctx context.Context, event PaymentEvent) error
}

// PaymentEvent carries transaction lifecycle data for downstream consumers.
type PaymentEvent struct {
	TransactionID         string    `json:"transaction_id"`
	PaymentType           string    `json:"payment_type"`
	Status                string    `json:"status"`
	SourceAccountID       string    `json:"source_account_id"`
	DestinationAccountID  string    `json:"destination_account_id"`
	Amount                int64     `json:"amount"`
	Currency              string    `json:"currency"`
	FailureReason         string    `json:"failure_reason,omitempty"`
	CorrelationID         string    `json:"correlation_id,omitempty"`
	InitiatedBy           string    `json:"initiated_by"`
	OccurredAt            time.Time `json:"occurred_at"`
}

// natsPaymentPublisher implements PaymentEventPublisher using pkg/messaging.
type natsPaymentPublisher struct {
	pub messaging.Publisher
}

// New creates a PaymentEventPublisher backed by a NATS publisher.
func New(pub messaging.Publisher) PaymentEventPublisher {
	return &natsPaymentPublisher{pub: pub}
}

func (p *natsPaymentPublisher) PublishCompleted(ctx context.Context, event PaymentEvent) error {
	return p.publish(ctx, subjectCompleted, event)
}

func (p *natsPaymentPublisher) PublishFailed(ctx context.Context, event PaymentEvent) error {
	return p.publish(ctx, subjectFailed, event)
}

func (p *natsPaymentPublisher) PublishReversed(ctx context.Context, event PaymentEvent) error {
	return p.publish(ctx, subjectReversed, event)
}

func (p *natsPaymentPublisher) PublishCancelled(ctx context.Context, event PaymentEvent) error {
	return p.publish(ctx, subjectCancelled, event)
}

func (p *natsPaymentPublisher) publish(ctx context.Context, subject string, event PaymentEvent) error {
	event.OccurredAt = time.Now().UTC()
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("event_publisher.publish: marshal: %w", err)
	}
	if err := p.pub.Publish(ctx, subject, payload); err != nil {
		slog.WarnContext(ctx, "event_publisher: publish failed",
			"subject", subject,
			"transaction_id", event.TransactionID,
			"error", err,
		)
		return fmt.Errorf("event_publisher.publish %s: %w", subject, err)
	}
	return nil
}
