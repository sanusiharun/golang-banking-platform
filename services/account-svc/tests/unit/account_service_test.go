package unit

import (
	"context"
	"errors"
	"testing"

	pkgerrors "github.com/sanusi/banking/pkg/errors"
	"github.com/sanusi/banking/services/account-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/account-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/account-svc/internal/repository"
	"github.com/sanusi/banking/services/account-svc/internal/services"
)

// ── Mock repository ───────────────────────────────────────────────────────────

type mockAccountRepo struct {
	account   *dao.Account
	accounts  []*dao.Account
	total     int64
	createErr error
	getErr    error
	updateErr error
	listErr   error
}

func (m *mockAccountRepo) Create(_ context.Context, account *dao.Account) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.account = account
	return nil
}

func (m *mockAccountRepo) GetByID(_ context.Context, _ string) (*dao.Account, error) {
	return m.account, m.getErr
}

func (m *mockAccountRepo) Update(_ context.Context, account *dao.Account) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.account = account
	return nil
}

func (m *mockAccountRepo) List(_ context.Context, _ string, _, _ int) ([]*dao.Account, int64, error) {
	return m.accounts, m.total, m.listErr
}

var _ repository.AccountRepository = (*mockAccountRepo)(nil)

// ── Helpers ───────────────────────────────────────────────────────────────────

func activeAccount(balance int64) *dao.Account {
	return &dao.Account{
		ID:         "acc-001",
		CustomerID: "cust-001",
		IBAN:       "GB29NWBK60161331926819",
		Currency:   "MYR",
		Balance:    balance,
		Status:     "ACTIVE",
		Version:    1,
	}
}

// ── CreateAccount ─────────────────────────────────────────────────────────────

func TestCreateAccount_Success(t *testing.T) {
	repo := &mockAccountRepo{}
	svc := services.NewAccountService(repo)

	resp, err := svc.CreateAccount(context.Background(), &dto.CreateAccountRequest{
		CustomerID: "cust-001",
		Currency:   "MYR",
		IBAN:       "GB29NWBK60161331926819",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID == "" {
		t.Error("expected non-empty account ID")
	}
	if resp.Currency != "MYR" {
		t.Errorf("got currency=%q, want MYR", resp.Currency)
	}
}

func TestCreateAccount_RepoError(t *testing.T) {
	repo := &mockAccountRepo{createErr: errors.New("db unavailable")}
	svc := services.NewAccountService(repo)

	_, err := svc.CreateAccount(context.Background(), &dto.CreateAccountRequest{
		CustomerID: "cust-001",
		Currency:   "MYR",
		IBAN:       "GB29NWBK60161331926819",
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
}

// ── GetAccount ────────────────────────────────────────────────────────────────

func TestGetAccount_Success(t *testing.T) {
	repo := &mockAccountRepo{account: activeAccount(5000)}
	svc := services.NewAccountService(repo)

	resp, err := svc.GetAccount(context.Background(), "acc-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Balance != 5000 {
		t.Errorf("got balance=%d, want 5000", resp.Balance)
	}
}

func TestGetAccount_NotFound(t *testing.T) {
	repo := &mockAccountRepo{getErr: repository.ErrNotFound}
	svc := services.NewAccountService(repo)

	_, err := svc.GetAccount(context.Background(), "nonexistent")
	if !pkgerrors.IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ── Credit ────────────────────────────────────────────────────────────────────

func TestCredit_Success(t *testing.T) {
	repo := &mockAccountRepo{account: activeAccount(1000)}
	svc := services.NewAccountService(repo)

	resp, err := svc.Credit(context.Background(), "acc-001", &dto.CreditRequest{
		Amount:    500,
		Reference: "top-up",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Balance != 1500 {
		t.Errorf("got balance=%d, want 1500", resp.Balance)
	}
}

func TestCredit_InactiveAccount(t *testing.T) {
	account := activeAccount(1000)
	account.Status = "SUSPENDED"
	repo := &mockAccountRepo{account: account}
	svc := services.NewAccountService(repo)

	_, err := svc.Credit(context.Background(), "acc-001", &dto.CreditRequest{Amount: 500})
	if !pkgerrors.IsValidation(err) {
		t.Errorf("expected validation error (account not active), got %v", err)
	}
}

// ── Debit ─────────────────────────────────────────────────────────────────────

func TestDebit_Success(t *testing.T) {
	repo := &mockAccountRepo{account: activeAccount(1000)}
	svc := services.NewAccountService(repo)

	resp, err := svc.Debit(context.Background(), "acc-001", &dto.DebitRequest{
		Amount:    300,
		Reference: "withdrawal",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Balance != 700 {
		t.Errorf("got balance=%d, want 700", resp.Balance)
	}
}

func TestDebit_InsufficientFunds(t *testing.T) {
	repo := &mockAccountRepo{account: activeAccount(100)}
	svc := services.NewAccountService(repo)

	_, err := svc.Debit(context.Background(), "acc-001", &dto.DebitRequest{Amount: 500})
	if !pkgerrors.IsValidation(err) {
		t.Errorf("expected validation error (insufficient funds), got %v", err)
	}
}

func TestDebit_InactiveAccount(t *testing.T) {
	account := activeAccount(1000)
	account.Status = "CLOSED"
	repo := &mockAccountRepo{account: account}
	svc := services.NewAccountService(repo)

	_, err := svc.Debit(context.Background(), "acc-001", &dto.DebitRequest{Amount: 100})
	if !pkgerrors.IsValidation(err) {
		t.Errorf("expected validation error (account not active), got %v", err)
	}
}

func TestDebit_ZeroBalance(t *testing.T) {
	repo := &mockAccountRepo{account: activeAccount(0)}
	svc := services.NewAccountService(repo)

	_, err := svc.Debit(context.Background(), "acc-001", &dto.DebitRequest{Amount: 1})
	if !pkgerrors.IsValidation(err) {
		t.Errorf("expected validation error (insufficient funds), got %v", err)
	}
}
