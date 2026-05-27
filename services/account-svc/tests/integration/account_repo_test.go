//go:build ignore
// TODO: remove the ignore tag (restore to `//go:build integration`) once
// internal/domain/account and internal/infrastructure/postgres are implemented.

// Package integration contains integration tests that require a running PostgreSQL instance.
// Run with: go test -tags=integration ./tests/integration/... -v
// Ensure TEST_DATABASE_URL or individual DB_ vars are set in the environment.
package integration_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sanusi/banking/services/account-svc/internal/domain/account"
	"github.com/sanusi/banking/services/account-svc/internal/infrastructure/postgres"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://banking:banking@localhost:5432/banking_test?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping integration test: cannot connect to postgres: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping integration test: ping failed: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestAccountRepo_CreateAndGetByID(t *testing.T) {
	pool := testPool(t)
	repo := postgres.NewAccountRepository(pool)
	ctx := context.Background()

	suffix := randomSuffix()
	acc, err := account.New(
		account.AccountID("integ-"+suffix),
		account.CustomerID("cust-integ-001"),
		"GB29"+suffix+"60161331926",
		"GBP",
		1000,
	)
	if err != nil {
		t.Fatalf("domain.New: %v", err)
	}

	if err := repo.Create(ctx, acc); err != nil {
		t.Fatalf("Create: %v", err)
	}

	retrieved, err := repo.GetByID(ctx, acc.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if retrieved.ID != acc.ID {
		t.Errorf("ID mismatch: got %s, want %s", retrieved.ID, acc.ID)
	}
	if retrieved.Balance != acc.Balance {
		t.Errorf("Balance mismatch: got %d, want %d", retrieved.Balance, acc.Balance)
	}
	if retrieved.Currency != acc.Currency {
		t.Errorf("Currency mismatch: got %s, want %s", retrieved.Currency, acc.Currency)
	}
	if retrieved.Status != account.StatusPending {
		t.Errorf("expected PENDING, got %s", retrieved.Status)
	}
}

func TestAccountRepo_UpdateOptimisticLock(t *testing.T) {
	pool := testPool(t)
	repo := postgres.NewAccountRepository(pool)
	ctx := context.Background()

	suffix := randomSuffix()
	acc, _ := account.New(
		account.AccountID("integ-lock-"+suffix),
		account.CustomerID("cust-002"),
		"EU29"+suffix+"00000001",
		"EUR",
		5000,
	)
	if err := repo.Create(ctx, acc); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = acc.Activate()
	if err := repo.Update(ctx, acc); err != nil {
		t.Fatalf("initial Update: %v", err)
	}

	// Simulate concurrent reads returning the same version.
	v1, _ := repo.GetByID(ctx, acc.ID)
	v2, _ := repo.GetByID(ctx, acc.ID)

	// First update should succeed.
	_ = v1.Credit(100)
	if err := repo.Update(ctx, v1); err != nil {
		t.Fatalf("first update should succeed: %v", err)
	}

	// Second update should fail with version conflict.
	_ = v2.Credit(200)
	err := repo.Update(ctx, v2)
	if err == nil {
		t.Fatal("expected version conflict error, got nil")
	}
	if err != account.ErrVersionConflict {
		t.Errorf("expected ErrVersionConflict, got: %v", err)
	}
}

func TestAccountRepo_CreateTransaction(t *testing.T) {
	pool := testPool(t)
	repo := postgres.NewAccountRepository(pool)
	ctx := context.Background()

	suffix := randomSuffix()
	acc, _ := account.New(
		account.AccountID("integ-tx-"+suffix),
		account.CustomerID("cust-003"),
		"US29"+suffix+"00000002",
		"USD",
		10000,
	)
	if err := repo.Create(ctx, acc); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_ = acc.Activate()
	_ = acc.Credit(500)
	_ = repo.Update(ctx, acc)

	tx, err := account.NewCreditTransaction(
		account.TransactionID("tx-"+suffix),
		acc.ID,
		500,
		acc.Balance,
		"USD",
		"integration test credit",
		"idem-"+suffix,
	)
	if err != nil {
		t.Fatalf("NewCreditTransaction: %v", err)
	}
	tx.Complete()

	if err := repo.CreateTransaction(ctx, tx); err != nil {
		t.Fatalf("CreateTransaction: %v", err)
	}

	txns, total, err := repo.GetTransactionsByAccountID(ctx, acc.ID, 0, 10)
	if err != nil {
		t.Fatalf("GetTransactionsByAccountID: %v", err)
	}
	if total == 0 || len(txns) == 0 {
		t.Error("expected at least one transaction")
	}
	if txns[0].Amount != 500 {
		t.Errorf("expected amount 500, got %d", txns[0].Amount)
	}
}

func TestAccountRepo_NotFound(t *testing.T) {
	pool := testPool(t)
	repo := postgres.NewAccountRepository(pool)

	_, err := repo.GetByID(context.Background(), account.AccountID("does-not-exist-xyz"))
	if err == nil {
		t.Fatal("expected not found error, got nil")
	}
}

func TestAccountRepo_List(t *testing.T) {
	pool := testPool(t)
	repo := postgres.NewAccountRepository(pool)
	ctx := context.Background()

	customerID := account.CustomerID("list-cust-" + randomSuffix())
	for i := 0; i < 3; i++ {
		suffix := randomSuffix()
		acc, _ := account.New(
			account.AccountID(fmt.Sprintf("list-acc-%s-%d", suffix, i)),
			customerID,
			fmt.Sprintf("LIST%s%d", suffix, i),
			"GBP",
			0,
		)
		_ = repo.Create(ctx, acc)
	}

	accounts, total, err := repo.List(ctx, customerID, 0, 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total < 3 {
		t.Errorf("expected at least 3 accounts, got %d", total)
	}
	_ = accounts
}

// randomSuffix generates a short random hex string for test data isolation.
func randomSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
