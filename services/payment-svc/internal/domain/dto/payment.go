// Package dto contains HTTP request and response Data Transfer Objects for payment-svc.
package dto

import "time"

// ── Payment type and status constants ────────────────────────────────────────

const (
	TypeTransfer        = "TRANSFER"
	TypeMerchantPayment = "MERCHANT_PAYMENT"
	TypeFee             = "FEE"
	TypeRefund          = "REFUND"
	TypeScheduled       = "SCHEDULED"

	StatusPending    = "PENDING"
	StatusProcessing = "PROCESSING"
	StatusSuccess    = "SUCCESS"
	StatusFailed     = "FAILED"
	StatusCancelled  = "CANCELLED"
	StatusReversed   = "REVERSED"

	ChannelMobileApp = "MOBILE_APP"
	ChannelWeb       = "WEB"
	ChannelAPI       = "API"
	ChannelSystem    = "SYSTEM"
)

// ── Initiation requests ───────────────────────────────────────────────────────

// TransferRequest is the payload for POST /v1/payments/transfer.
type TransferRequest struct {
	SourceAccountID      string         `json:"source_account_id"      validate:"required,uuid"`
	DestinationAccountID string         `json:"destination_account_id" validate:"required,uuid"`
	Amount               int64          `json:"amount"                 validate:"required,gt=0"`
	Currency             string         `json:"currency"               validate:"required,len=3,uppercase"`
	Channel              string         `json:"channel"                validate:"required,oneof=MOBILE_APP WEB API SYSTEM"`
	Description          string         `json:"description"            validate:"max=500"`
	ExternalReference    string         `json:"external_reference"     validate:"max=128"`
	Metadata             map[string]any `json:"metadata"`
}

// MerchantPaymentRequest is the payload for POST /v1/payments/merchant.
type MerchantPaymentRequest struct {
	SourceAccountID      string         `json:"source_account_id"      validate:"required,uuid"`
	DestinationAccountID string         `json:"destination_account_id" validate:"required,uuid"`
	Amount               int64          `json:"amount"                 validate:"required,gt=0"`
	Currency             string         `json:"currency"               validate:"required,len=3,uppercase"`
	Channel              string         `json:"channel"                validate:"required,oneof=MOBILE_APP WEB API"`
	Description          string         `json:"description"            validate:"max=500"`
	ExternalReference    string         `json:"external_reference"     validate:"max=128"`
	Metadata             map[string]any `json:"metadata"`
}

// FeeRequest is the payload for POST /v1/payments/fee (service-scoped callers only).
type FeeRequest struct {
	SourceAccountID      string         `json:"source_account_id"      validate:"required,uuid"`
	DestinationAccountID string         `json:"destination_account_id" validate:"required,uuid"`
	Amount               int64          `json:"amount"                 validate:"required,gt=0"`
	Currency             string         `json:"currency"               validate:"required,len=3,uppercase"`
	Description          string         `json:"description"            validate:"max=500"`
	ExternalReference    string         `json:"external_reference"     validate:"max=128"`
	Metadata             map[string]any `json:"metadata"`
}

// RefundRequest is the payload for POST /v1/payments/refund.
type RefundRequest struct {
	SourceAccountID      string         `json:"source_account_id"      validate:"required,uuid"`
	DestinationAccountID string         `json:"destination_account_id" validate:"required,uuid"`
	Amount               int64          `json:"amount"                 validate:"required,gt=0"`
	Currency             string         `json:"currency"               validate:"required,len=3,uppercase"`
	OriginalReference    string         `json:"original_reference"     validate:"max=128"`
	Description          string         `json:"description"            validate:"max=500"`
	Metadata             map[string]any `json:"metadata"`
}

// ── Responses ─────────────────────────────────────────────────────────────────

// TransactionResponse is returned for single-transaction endpoints.
type TransactionResponse struct {
	ID                    string         `json:"id"`
	IdempotencyKey        string         `json:"idempotency_key"`
	PaymentType           string         `json:"payment_type"`
	Channel               string         `json:"channel"`
	SourceAccountID       string         `json:"source_account_id"`
	DestinationAccountID  string         `json:"destination_account_id"`
	Amount                int64          `json:"amount"`
	Currency              string         `json:"currency"`
	Status                string         `json:"status"`
	FailureReason         *string        `json:"failure_reason,omitempty"`
	RetryCount            int            `json:"retry_count"`
	ExternalReference     *string        `json:"external_reference,omitempty"`
	CorrelationID         *string        `json:"correlation_id,omitempty"`
	Description           *string        `json:"description,omitempty"`
	Metadata              map[string]any `json:"metadata,omitempty"`
	InitiatedBy           string         `json:"initiated_by"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	CompletedAt           *time.Time     `json:"completed_at,omitempty"`
	ReversedAt            *time.Time     `json:"reversed_at,omitempty"`
}

// TransactionListResponse wraps a paginated list of transactions.
type TransactionListResponse struct {
	Items      []*TransactionResponse `json:"items"`
	Total      int64                  `json:"total"`
	Limit      int                    `json:"limit"`
	Offset     int                    `json:"offset"`
	HasMore    bool                   `json:"has_more"`
}

// ── List filter ───────────────────────────────────────────────────────────────

// ListFilter holds query parameters for transaction listing.
type ListFilter struct {
	AccountID string
	Status    string
	From      *time.Time
	To        *time.Time
	Limit     int
	Offset    int
}
