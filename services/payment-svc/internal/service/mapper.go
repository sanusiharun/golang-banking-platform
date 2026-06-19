package service

import (
	"encoding/json"
	"log/slog"

	"github.com/sanusi/banking/services/payment-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dto"
)

// toResponse converts a dao.Transaction to a dto.TransactionResponse.
func toResponse(t *dao.Transaction) *dto.TransactionResponse {
	r := &dto.TransactionResponse{
		ID:                   t.ID,
		IdempotencyKey:       t.IdempotencyKey,
		PaymentType:          t.PaymentType,
		Channel:              t.Channel,
		SourceAccountID:      t.SourceAccountID,
		DestinationAccountID: t.DestinationAccountID,
		Amount:               t.Amount,
		Currency:             t.Currency,
		Status:               t.Status,
		FailureReason:        t.FailureReason,
		RetryCount:           t.RetryCount,
		ExternalReference:    t.ExternalReference,
		CorrelationID:        t.CorrelationID,
		Description:          t.Description,
		InitiatedBy:          t.InitiatedBy,
		CreatedAt:            t.CreatedAt,
		UpdatedAt:            t.UpdatedAt,
		CompletedAt:          t.CompletedAt,
		ReversedAt:           t.ReversedAt,
	}
	if len(t.Metadata) > 0 {
		var m map[string]any
		if err := json.Unmarshal(t.Metadata, &m); err == nil {
			r.Metadata = m
		} else {
			slog.Warn("payment_service: failed to unmarshal transaction metadata", "transaction_id", t.ID, "error", err)
		}
	}
	return r
}
