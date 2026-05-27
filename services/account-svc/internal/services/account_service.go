// Package services contains the business logic for account-svc.
// Uses the global slog logger — no *slog.Logger constructor injection.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/sanusi/banking/services/account-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/account-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/account-svc/internal/repository"
)

// ── Interface ─────────────────────────────────────────────────────────────────

type AccountService interface {
	CreateAccount(ctx context.Context, req *dto.CreateAccountRequest) (*dto.AccountResponse, error)
	GetAccount(ctx context.Context, id string) (*dto.AccountResponse, error)
	GetBalance(ctx context.Context, id string) (*dto.BalanceResponse, error)
	Credit(ctx context.Context, id string, req *dto.CreditRequest) (*dto.AccountResponse, error)
	Debit(ctx context.Context, id string, req *dto.DebitRequest) (*dto.AccountResponse, error)
	ListAccounts(ctx context.Context, customerID string, page, pageSize int) (*dto.PaginatedAccountsResponse, error)
}

// ── Implementation ────────────────────────────────────────────────────────────

type accountService struct {
	repo repository.AccountRepository
}

// NewAccountService creates a new AccountService.
// No logger parameter — uses slog global configured in main.go.
func NewAccountService(repo repository.AccountRepository) AccountService {
	return &accountService{repo: repo}
}

func (s *accountService) CreateAccount(ctx context.Context, req *dto.CreateAccountRequest) (*dto.AccountResponse, error) {
	account := &dao.Account{
		ID:         uuid.New().String(),
		CustomerID: req.CustomerID,
		IBAN:       req.IBAN,
		Currency:   req.Currency,
		Balance:    0,
		Status:     "PENDING",
		Version:    1,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, account); err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	slog.InfoContext(ctx, "account created",
		slog.String("account_id", account.ID),
		slog.String("customer_id", account.CustomerID),
	)
	return toAccountResponse(account), nil
}

func (s *accountService) GetAccount(ctx context.Context, id string) (*dto.AccountResponse, error) {
	account, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toAccountResponse(account), nil
}

func (s *accountService) GetBalance(ctx context.Context, id string) (*dto.BalanceResponse, error) {
	account, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &dto.BalanceResponse{
		AccountID: account.ID,
		Balance:   account.Balance,
		Currency:  account.Currency,
		Status:    account.Status,
	}, nil
}

func (s *accountService) Credit(ctx context.Context, id string, req *dto.CreditRequest) (*dto.AccountResponse, error) {
	account, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account.Status != "ACTIVE" {
		return nil, repository.ErrAccountNotActive
	}
	if account.Balance > math.MaxInt64-req.Amount {
		return nil, fmt.Errorf("credit would overflow balance")
	}

	account.Balance += req.Amount

	if err := s.repo.Update(ctx, account); err != nil {
		return nil, fmt.Errorf("credit update: %w", err)
	}

	slog.InfoContext(ctx, "account credited",
		slog.String("account_id", id),
		slog.Int64("amount", req.Amount),
		slog.Int64("new_balance", account.Balance),
	)
	return toAccountResponse(account), nil
}

func (s *accountService) Debit(ctx context.Context, id string, req *dto.DebitRequest) (*dto.AccountResponse, error) {
	account, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account.Status != "ACTIVE" {
		return nil, repository.ErrAccountNotActive
	}
	if account.Balance < req.Amount {
		return nil, repository.ErrInsufficientFunds
	}

	account.Balance -= req.Amount

	if err := s.repo.Update(ctx, account); err != nil {
		return nil, fmt.Errorf("debit update: %w", err)
	}

	slog.InfoContext(ctx, "account debited",
		slog.String("account_id", id),
		slog.Int64("amount", req.Amount),
		slog.Int64("new_balance", account.Balance),
	)
	return toAccountResponse(account), nil
}

func (s *accountService) ListAccounts(ctx context.Context, customerID string, page, pageSize int) (*dto.PaginatedAccountsResponse, error) {
	accounts, total, err := s.repo.List(ctx, customerID, page, pageSize)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	items := make([]*dto.AccountResponse, len(accounts))
	for i, a := range accounts {
		items[i] = toAccountResponse(a)
	}

	totalPages := int(math.Ceil(float64(total) / float64(pageSize)))

	return &dto.PaginatedAccountsResponse{
		Items:      items,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: total,
		TotalPages: totalPages,
	}, nil
}

// toAccountResponse maps a DAO model to a response DTO.
// Explicit mapping prevents GORM internals (Version, etc.) leaking into HTTP responses.
func toAccountResponse(a *dao.Account) *dto.AccountResponse {
	return &dto.AccountResponse{
		ID:         a.ID,
		CustomerID: a.CustomerID,
		IBAN:       a.IBAN,
		Currency:   a.Currency,
		Balance:    a.Balance,
		Status:     a.Status,
		CreatedAt:  a.CreatedAt,
		UpdatedAt:  a.UpdatedAt,
	}
}
