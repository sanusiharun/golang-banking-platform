// Package dto contains HTTP request and response Data Transfer Objects.
// One request file and one response file per entity.
package dto

// CreateAccountRequest is the payload for POST /v1/accounts.
type CreateAccountRequest struct {
	CustomerID string `json:"customer_id" validate:"required,min=1,max=100"`
	Currency   string `json:"currency"    validate:"required,len=3,uppercase"`
	IBAN       string `json:"iban"        validate:"required,min=15,max=34"`
}

// CreditRequest is the payload for POST /v1/accounts/{id}/credit.
type CreditRequest struct {
	// Amount in minor currency units (e.g. 1000 = ₦10.00).
	Amount    int64  `json:"amount"    validate:"required,gt=0"`
	Reference string `json:"reference" validate:"max=255"`
}

// DebitRequest is the payload for POST /v1/accounts/{id}/debit.
type DebitRequest struct {
	Amount    int64  `json:"amount"    validate:"required,gt=0"`
	Reference string `json:"reference" validate:"max=255"`
}
