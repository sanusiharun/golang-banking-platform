package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgerrors "github.com/sanusi/banking/pkg/errors"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/payment-svc/internal/service"
)

// originalTxn returns a SUCCESS transaction from payer-acct to merchant-acct,
// the shape every refund test validates against.
func originalTxn() *dao.Transaction {
	return &dao.Transaction{
		ID:                   "txn-orig",
		Status:               dto.StatusSuccess,
		SourceAccountID:      "payer-acct",
		DestinationAccountID: "merchant-acct",
		Amount:               15_000,
		Currency:             "IDR",
	}
}

// validRefundReq is a refund that correctly reverses originalTxn()'s counterparties.
func validRefundReq() *dto.RefundRequest {
	return &dto.RefundRequest{
		SourceAccountID:      "merchant-acct",
		DestinationAccountID: "payer-acct",
		Amount:               15_000,
		Currency:             "IDR",
		OriginalReference:    "txn-orig",
	}
}

func TestInitiateRefund(t *testing.T) {
	t.Run("refunds against a successful original transaction", func(t *testing.T) {
		repo := newMockTxnRepo()
		repo.byID["txn-orig"] = originalTxn()

		account := &mockAccount{balance: 100_000, status: "ACTIVE"}
		svc := service.NewPaymentService(repo, account, &mockPublisher{})

		resp, err := svc.InitiateRefund(context.Background(), "idem-1", validRefundReq(), "user-1")

		require.NoError(t, err)
		assert.Equal(t, dto.TypeRefund, resp.PaymentType)
		assert.Equal(t, dto.StatusSuccess, resp.Status)
		assert.Equal(t, "txn-orig", *resp.ExternalReference)
	})

	t.Run("rejects refund when original_reference is missing", func(t *testing.T) {
		repo := newMockTxnRepo()
		account := &mockAccount{balance: 100_000, status: "ACTIVE"}
		svc := service.NewPaymentService(repo, account, &mockPublisher{})

		req := validRefundReq()
		req.OriginalReference = ""

		_, err := svc.InitiateRefund(context.Background(), "idem-0", req, "user-1")

		require.Error(t, err)
		assert.True(t, pkgerrors.IsValidation(err))
	})

	t.Run("rejects refund when original transaction is missing", func(t *testing.T) {
		repo := newMockTxnRepo()
		account := &mockAccount{balance: 100_000, status: "ACTIVE"}
		svc := service.NewPaymentService(repo, account, &mockPublisher{})

		req := validRefundReq()
		req.OriginalReference = "missing-txn"

		_, err := svc.InitiateRefund(context.Background(), "idem-2", req, "user-1")

		require.Error(t, err)
		assert.True(t, pkgerrors.IsValidation(err))
	})

	t.Run("rejects refund when original transaction did not succeed", func(t *testing.T) {
		repo := newMockTxnRepo()
		failed := originalTxn()
		failed.ID = "txn-failed"
		failed.Status = dto.StatusFailed
		repo.byID["txn-failed"] = failed

		account := &mockAccount{balance: 100_000, status: "ACTIVE"}
		svc := service.NewPaymentService(repo, account, &mockPublisher{})

		req := validRefundReq()
		req.OriginalReference = "txn-failed"

		_, err := svc.InitiateRefund(context.Background(), "idem-3", req, "user-1")

		require.Error(t, err)
		assert.True(t, pkgerrors.IsValidation(err))
	})

	t.Run("rejects refund whose accounts don't match the original counterparties", func(t *testing.T) {
		repo := newMockTxnRepo()
		repo.byID["txn-orig"] = originalTxn()

		account := &mockAccount{balance: 100_000, status: "ACTIVE"}
		svc := service.NewPaymentService(repo, account, &mockPublisher{})

		req := validRefundReq()
		req.DestinationAccountID = "attacker-acct" // not the original payer

		_, err := svc.InitiateRefund(context.Background(), "idem-4", req, "user-1")

		require.Error(t, err)
		assert.True(t, pkgerrors.IsValidation(err))
		assert.Empty(t, account.debits, "no funds should move when accounts are rejected")
	})

	t.Run("rejects refund amount exceeding the original amount", func(t *testing.T) {
		repo := newMockTxnRepo()
		repo.byID["txn-orig"] = originalTxn()

		account := &mockAccount{balance: 1_000_000, status: "ACTIVE"}
		svc := service.NewPaymentService(repo, account, &mockPublisher{})

		req := validRefundReq()
		req.Amount = 1_000_000 // original was only 15,000

		_, err := svc.InitiateRefund(context.Background(), "idem-5", req, "user-1")

		require.Error(t, err)
		assert.True(t, pkgerrors.IsValidation(err))
		assert.Empty(t, account.debits, "no funds should move when amount exceeds original")
	})

	t.Run("rejects refund currency mismatch with the original transaction", func(t *testing.T) {
		repo := newMockTxnRepo()
		repo.byID["txn-orig"] = originalTxn()

		account := &mockAccount{balance: 100_000, status: "ACTIVE"}
		svc := service.NewPaymentService(repo, account, &mockPublisher{})

		req := validRefundReq()
		req.Currency = "USD"

		_, err := svc.InitiateRefund(context.Background(), "idem-6", req, "user-1")

		require.Error(t, err)
		assert.True(t, pkgerrors.IsValidation(err))
	})

	t.Run("rejects a second refund of the same original under a different idempotency key", func(t *testing.T) {
		repo := newMockTxnRepo()
		repo.byID["txn-orig"] = originalTxn()

		account := &mockAccount{balance: 100_000, status: "ACTIVE"}
		svc := service.NewPaymentService(repo, account, &mockPublisher{})

		_, err := svc.InitiateRefund(context.Background(), "idem-first", validRefundReq(), "user-1")
		require.NoError(t, err)

		_, err = svc.InitiateRefund(context.Background(), "idem-second", validRefundReq(), "user-1")

		require.Error(t, err)
		assert.True(t, pkgerrors.IsConflict(err))
		assert.Len(t, account.debits, 1, "second refund attempt must not move funds")
	})

	t.Run("replays the same transaction for a duplicate idempotency key", func(t *testing.T) {
		repo := newMockTxnRepo()
		repo.byID["txn-orig"] = originalTxn()

		account := &mockAccount{balance: 100_000, status: "ACTIVE"}
		svc := service.NewPaymentService(repo, account, &mockPublisher{})

		req := validRefundReq()

		first, err := svc.InitiateRefund(context.Background(), "idem-dup", req, "user-1")
		require.NoError(t, err)

		second, err := svc.InitiateRefund(context.Background(), "idem-dup", req, "user-1")
		require.NoError(t, err)

		assert.Equal(t, first.ID, second.ID)
		assert.Len(t, account.debits, 1, "second call must not move funds again")
	})
}
