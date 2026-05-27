package dto

import "time"

// AccountResponse is returned for single-account endpoints.
type AccountResponse struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	IBAN       string    `json:"iban"`
	Currency   string    `json:"currency"`
	Balance    int64     `json:"balance"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// BalanceResponse is returned by GET /v1/accounts/{id}/balance.
type BalanceResponse struct {
	AccountID string `json:"account_id"`
	Balance   int64  `json:"balance"`
	Currency  string `json:"currency"`
	Status    string `json:"status"`
}

// PaginatedAccountsResponse wraps a list of accounts with pagination metadata.
type PaginatedAccountsResponse struct {
	Items      []*AccountResponse `json:"items"`
	Page       int                `json:"page"`
	PageSize   int                `json:"page_size"`
	TotalCount int64              `json:"total_count"`
	TotalPages int                `json:"total_pages"`
}
