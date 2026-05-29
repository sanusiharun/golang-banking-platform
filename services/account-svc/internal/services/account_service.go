// Package services contains the business logic for account-svc.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"

	"github.com/sanusi/banking/pkg/observability"
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
	tr   *observability.ServiceTracer
	repo repository.AccountRepository
}

func NewAccountService(repo repository.AccountRepository) AccountService {
	return &accountService{
		tr:   observability.NewServiceTracer("AccountService"),
		repo: repo,
	}
}

func (s *accountService) CreateAccount(ctx context.Context, req *dto.CreateAccountRequest) (res *dto.AccountResponse, err error) {
	ctx, span := s.tr.Start(ctx, "CreateAccount",
		attribute.String("account.customer_id", req.CustomerID),
		attribute.String("account.currency", req.Currency),
	)
	defer s.tr.Finish(span, &err)

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

	if err = s.repo.Create(ctx, account); err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}

	slog.InfoContext(ctx, "account created",
		slog.String("account_id", account.ID),
		slog.String("customer_id", account.CustomerID),
	)
	return toAccountResponse(account), nil
}

func (s *accountService) GetAccount(ctx context.Context, id string) (res *dto.AccountResponse, err error) {
	ctx, span := s.tr.Start(ctx, "GetAccount",
		attribute.String("account.id", id),
	)
	defer s.tr.Finish(span, &err)

	account, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return toAccountResponse(account), nil
}

func (s *accountService) GetBalance(ctx context.Context, id string) (res *dto.BalanceResponse, err error) {
	ctx, span := s.tr.Start(ctx, "GetBalance",
		attribute.String("account.id", id),
	)
	defer s.tr.Finish(span, &err)

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

func (s *accountService) Credit(ctx context.Context, id string, req *dto.CreditRequest) (res *dto.AccountResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Credit",
		attribute.String("account.id", id),
		attribute.Int64("account.credit_amount", req.Amount),
	)
	defer s.tr.Finish(span, &err)

	account, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account.Status != "ACTIVE" {
		err = repository.ErrAccountNotActive
		return nil, err
	}
	if account.Balance > math.MaxInt64-req.Amount {
		err = fmt.Errorf("credit would overflow balance")
		return nil, err
	}

	account.Balance += req.Amount

	if err = s.repo.Update(ctx, account); err != nil {
		return nil, fmt.Errorf("credit update: %w", err)
	}

	slog.InfoContext(ctx, "account credited",
		slog.String("account_id", id),
		slog.Int64("amount", req.Amount),
		slog.Int64("new_balance", account.Balance),
	)
	return toAccountResponse(account), nil
}

func (s *accountService) Debit(ctx context.Context, id string, req *dto.DebitRequest) (res *dto.AccountResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Debit",
		attribute.String("account.id", id),
		attribute.Int64("account.debit_amount", req.Amount),
	)
	defer s.tr.Finish(span, &err)

	account, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if account.Status != "ACTIVE" {
		err = repository.ErrAccountNotActive
		return nil, err
	}
	if account.Balance < req.Amount {
		err = repository.ErrInsufficientFunds
		return nil, err
	}

	account.Balance -= req.Amount

	if err = s.repo.Update(ctx, account); err != nil {
		return nil, fmt.Errorf("debit update: %w", err)
	}

	slog.InfoContext(ctx, "account debited",
		slog.String("account_id", id),
		slog.Int64("amount", req.Amount),
		slog.Int64("new_balance", account.Balance),
	)
	return toAccountResponse(account), nil
}

func (s *accountService) ListAccounts(ctx context.Context, customerID string, page, pageSize int) (res *dto.PaginatedAccountsResponse, err error) {
	ctx, span := s.tr.Start(ctx, "ListAccounts",
		attribute.String("account.customer_id", customerID),
		attribute.Int("pagination.page", page),
		attribute.Int("pagination.page_size", pageSize),
	)
	defer s.tr.Finish(span, &err)

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
