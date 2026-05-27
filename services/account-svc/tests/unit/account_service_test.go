//go:build ignore
// TODO: remove the ignore tag once internal/domain/account and
// internal/application/account packages are implemented.

package unit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sanusi/banking/services/account-svc/internal/application/account"
	domain "github.com/sanusi/banking/services/account-svc/internal/domain/account"
	"github.com/sanusi/banking/pkg/logger"
)

// --- Mock Repository ---

// mockRepo is an in-memory implementation of domain.Repository for unit tests.
type mockRepo struct {
	accounts     map[string]*domain.Account
	transactions []*domain.Transaction
	createErr    error
	getErr       error
	updateErr    error
}

func newMockRepo() *mockRepo {
	return &mockRepo{accounts: make(map[string]*domain.Account)}
}

func (m *mockRepo) Create(_ context.Context, acc *domain.Account) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.accounts[acc.ID.String()] = acc
	return nil
}

func (m *mockRepo) GetByID(_ context.Context, id domain.AccountID) (*domain.Account, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	acc, ok := m.accounts[id.String()]
	if !ok {
		return nil, errors.New("not found")
	}
	// Return a copy to simulate DB round-trip.
	copy := *acc
	return &copy, nil
}

func (m *mockRepo) GetByIBAN(_ context.Context, iban string) (*domain.Account, error) {
	for _, a := range m.accounts {
		if a.IBAN == iban {
			copy := *a
			return &copy, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockRepo) Update(_ context.Context, acc *domain.Account) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.accounts[acc.ID.String()] = acc
	return nil
}

func (m *mockRepo) Delete(_ context.Context, id domain.AccountID) error {
	delete(m.accounts, id.String())
	return nil
}

func (m *mockRepo) List(_ context.Context, _ domain.CustomerID, _, _ int) ([]*domain.Account, int64, error) {
	var out []*domain.Account
	for _, a := range m.accounts {
		copy := *a
		out = append(out, &copy)
	}
	return out, int64(len(out)), nil
}

func (m *mockRepo) ListAll(_ context.Context, _, _ int) ([]*domain.Account, int64, error) {
	return m.List(context.Background(), "", 0, 100)
}

func (m *mockRepo) CreateTransaction(_ context.Context, tx *domain.Transaction) error {
	m.transactions = append(m.transactions, tx)
	return nil
}

func (m *mockRepo) GetTransactionsByAccountID(_ context.Context, _ domain.AccountID, _, _ int) ([]*domain.Transaction, int64, error) {
	return m.transactions, int64(len(m.transactions)), nil
}

// --- Helpers ---

func newTestService(repo domain.Repository) account.Service {
	return account.NewService(repo, logger.NewNop())
}

func makeAccount(t *testing.T, repo *mockRepo, currency string, balance int64) *domain.Account {
	t.Helper()
	acc, err := domain.New(
		domain.AccountID("acc-"+t.Name()),
		domain.CustomerID("cust-001"),
		"GB29NWBK60161331926819",
		currency,
		balance,
	)
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}
	if err := acc.Activate(); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	repo.accounts[acc.ID.String()] = acc
	return acc
}

// --- Tests ---

func TestCreateAccount_Success(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)

	cmd := account.CreateAccountCommand{
		CustomerID:     "cust-001",
		IBAN:           "GB29NWBK60161331926819",
		Currency:       "GBP",
		InitialBalance: 0,
		IdempotencyKey: "idem-001",
	}

	acc, err := svc.CreateAccount(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc.Currency != "GBP" {
		t.Errorf("expected currency GBP, got %s", acc.Currency)
	}
	if acc.Status != domain.StatusPending {
		t.Errorf("expected PENDING status, got %s", acc.Status)
	}
	if acc.Balance != 0 {
		t.Errorf("expected 0 balance, got %d", acc.Balance)
	}
}

func TestCreateAccount_InvalidCurrency(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)

	_, err := svc.CreateAccount(context.Background(), account.CreateAccountCommand{
		CustomerID: "cust-001",
		IBAN:       "GB29NWBK60161331926819",
		Currency:   "GBPP", // invalid: too long
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestCredit_Success(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)
	acc := makeAccount(t, repo, "GBP", 0)

	tx, err := svc.Credit(context.Background(), account.CreditCommand{
		AccountID: acc.ID.String(),
		Amount:    5000, // £50.00
		Currency:  "GBP",
		Reference: "salary",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx.Amount != 5000 {
		t.Errorf("expected transaction amount 5000, got %d", tx.Amount)
	}
	if tx.BalanceAfter != 5000 {
		t.Errorf("expected balance_after 5000, got %d", tx.BalanceAfter)
	}
	if tx.Type != domain.TransactionTypeCredit {
		t.Errorf("expected CREDIT type, got %s", tx.Type)
	}
}

func TestDebit_InsufficientFunds(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)
	acc := makeAccount(t, repo, "GBP", 100) // £1.00

	_, err := svc.Debit(context.Background(), account.DebitCommand{
		AccountID: acc.ID.String(),
		Amount:    500, // £5.00 — more than balance
		Currency:  "GBP",
		Reference: "payment",
	})
	if err == nil {
		t.Fatal("expected insufficient funds error, got nil")
	}
}

func TestDebit_Success(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)
	acc := makeAccount(t, repo, "EUR", 10000) // €100.00

	tx, err := svc.Debit(context.Background(), account.DebitCommand{
		AccountID: acc.ID.String(),
		Amount:    3000, // €30.00
		Currency:  "EUR",
		Reference: "purchase",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx.BalanceAfter != 7000 {
		t.Errorf("expected balance_after 7000, got %d", tx.BalanceAfter)
	}
}

func TestTransfer_Success(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)

	src, err := domain.New(domain.AccountID("src-001"), domain.CustomerID("c1"), "GB29NWBK60161331926819", "GBP", 20000)
	if err != nil {
		t.Fatal(err)
	}
	_ = src.Activate()

	dst, err := domain.New(domain.AccountID("dst-001"), domain.CustomerID("c2"), "GB82WEST12345698765432", "GBP", 0)
	if err != nil {
		t.Fatal(err)
	}
	_ = dst.Activate()

	repo.accounts[src.ID.String()] = src
	repo.accounts[dst.ID.String()] = dst

	_, err = svc.Transfer(context.Background(), account.TransferCommand{
		SourceAccountID:      "src-001",
		DestinationAccountID: "dst-001",
		Amount:               5000,
		Currency:             "GBP",
		Reference:            "rent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Source should be debited.
	updatedSrc := repo.accounts["src-001"]
	if updatedSrc.Balance != 15000 {
		t.Errorf("src balance: expected 15000, got %d", updatedSrc.Balance)
	}

	// Destination should be credited.
	updatedDst := repo.accounts["dst-001"]
	if updatedDst.Balance != 5000 {
		t.Errorf("dst balance: expected 5000, got %d", updatedDst.Balance)
	}
}

func TestGetBalance(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)
	acc := makeAccount(t, repo, "GBP", 10050) // £100.50

	result, err := svc.GetBalance(context.Background(), account.GetBalanceQuery{AccountID: acc.ID.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.BalanceMinorUnits != 10050 {
		t.Errorf("expected 10050 minor units, got %d", result.BalanceMinorUnits)
	}
	if result.BalanceMajorStr != "100.50" {
		t.Errorf("expected '100.50', got %s", result.BalanceMajorStr)
	}
}

func TestActivateAccount(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)

	acc, _ := domain.New(domain.AccountID("acc-activate"), domain.CustomerID("c1"), "GB29NWBK60161331926819", "GBP", 0)
	repo.accounts[acc.ID.String()] = acc

	result, err := svc.ActivateAccount(context.Background(), account.ActivateAccountCommand{AccountID: "acc-activate"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != domain.StatusActive {
		t.Errorf("expected ACTIVE, got %s", result.Status)
	}
}

func TestSuspendAccount_AlreadyClosed(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)

	acc, _ := domain.New(domain.AccountID("acc-closed"), domain.CustomerID("c1"), "GB29NWBK60161331926819", "GBP", 0)
	_ = acc.Close()
	repo.accounts[acc.ID.String()] = acc

	_, err := svc.SuspendAccount(context.Background(), account.SuspendAccountCommand{AccountID: "acc-closed"})
	if err == nil {
		t.Fatal("expected error when suspending a closed account")
	}
}

func TestDebit_ZeroAmount(t *testing.T) {
	repo := newMockRepo()
	svc := newTestService(repo)
	acc := makeAccount(t, repo, "GBP", 10000)

	_, err := svc.Debit(context.Background(), account.DebitCommand{
		AccountID: acc.ID.String(),
		Amount:    0,
		Currency:  "GBP",
		Reference: "test",
	})
	if err == nil {
		t.Fatal("expected error for zero amount debit")
	}
}
