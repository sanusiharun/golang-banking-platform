// Package accountclient provides an HTTP client for calling account-svc APIs.
// All balance mutations must go through this client — never direct DB access.
package accountclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/sanusi/banking/pkg/httpclient"
)

// AccountClient defines the account-svc operations needed by payment-svc.
type AccountClient interface {
	GetAccount(ctx context.Context, accountID string) (*AccountInfo, error)
	GetBalance(ctx context.Context, accountID string) (*BalanceInfo, error)
	Debit(ctx context.Context, accountID string, amount int64, currency, reference string) error
	Credit(ctx context.Context, accountID string, amount int64, currency, reference string) error
}

// AccountInfo is the relevant subset of an account-svc AccountResponse.
type AccountInfo struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Currency string `json:"currency"`
}

// BalanceInfo is the relevant subset of an account-svc BalanceResponse.
type BalanceInfo struct {
	AccountID string `json:"account_id"`
	Balance   int64  `json:"balance"`
	Currency  string `json:"currency"`
	Status    string `json:"status"`
}

// httpAccountClient implements AccountClient using pkg/httpclient.
type httpAccountClient struct {
	client *httpclient.Client
	apiKey string
}

// New creates an AccountClient that calls account-svc at baseURL.
// The apiKey is sent as X-API-Key on every request.
func New(baseURL, apiKey string) AccountClient {
	cfg := httpclient.DefaultConfig()
	cfg.BaseURL = baseURL
	return &httpAccountClient{
		client: httpclient.New(cfg),
		apiKey: apiKey,
	}
}

func (c *httpAccountClient) GetAccount(ctx context.Context, accountID string) (*AccountInfo, error) {
	var resp struct {
		Data AccountInfo `json:"data"`
	}
	if err := c.client.Do(ctx, http.MethodGet,
		fmt.Sprintf("/v1/accounts/%s", accountID),
		nil, &resp,
		httpclient.WithHeader("X-API-Key", c.apiKey),
	); err != nil {
		return nil, fmt.Errorf("account_client.GetAccount: %w", err)
	}
	return &resp.Data, nil
}

func (c *httpAccountClient) GetBalance(ctx context.Context, accountID string) (*BalanceInfo, error) {
	var resp struct {
		Data BalanceInfo `json:"data"`
	}
	if err := c.client.Do(ctx, http.MethodGet,
		fmt.Sprintf("/v1/accounts/%s/balance", accountID),
		nil, &resp,
		httpclient.WithHeader("X-API-Key", c.apiKey),
	); err != nil {
		return nil, fmt.Errorf("account_client.GetBalance: %w", err)
	}
	return &resp.Data, nil
}

func (c *httpAccountClient) Debit(ctx context.Context, accountID string, amount int64, _ string, reference string) error {
	body := map[string]any{
		"amount":    amount,
		"reference": reference,
	}
	if err := c.client.Do(ctx, http.MethodPost,
		fmt.Sprintf("/v1/accounts/%s/debit", accountID),
		body, nil,
		httpclient.WithHeader("X-API-Key", c.apiKey),
	); err != nil {
		return fmt.Errorf("account_client.Debit: %w", err)
	}
	return nil
}

func (c *httpAccountClient) Credit(ctx context.Context, accountID string, amount int64, _ string, reference string) error {
	body := map[string]any{
		"amount":    amount,
		"reference": reference,
	}
	if err := c.client.Do(ctx, http.MethodPost,
		fmt.Sprintf("/v1/accounts/%s/credit", accountID),
		body, nil,
		httpclient.WithHeader("X-API-Key", c.apiKey),
	); err != nil {
		return fmt.Errorf("account_client.Credit: %w", err)
	}
	return nil
}
