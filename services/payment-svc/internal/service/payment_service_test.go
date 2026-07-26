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

func TestInitiateRefund(t *testing.T) {
	t.Run("refunds against a successful original transaction", func(t *testing.T) {
		repo := newMockTxnRepo()
		repo.byID["txn-orig"] = &dao.Transaction{ID: "txn-orig", Status: dto.StatusSuccess}

		account := &mockAccount{balance: 100_000, status: "ACTIVE"}
		svc := service.NewPaymentService(repo, account, &mockPublisher{})

		resp, err := svc.InitiateRefund(context.Background(), "idem-1", &dto.RefundRequest{
			SourceAccountID:      "merchant-acct",
			DestinationAccountID: "payer-acct",
			Amount:               15_000,
			Currency:             "IDR",
			OriginalReference:    "txn-orig",
		}, "user-1")

		require.NoError(t, err)
		assert.Equal(t, dto.TypeRefund, resp.PaymentType)
		assert.Equal(t, dto.StatusSuccess, resp.Status)
		assert.Equal(t, "txn-orig", *resp.ExternalReference)
	})

	t.Run("rejects refund when original transaction is missing", func(t *testing.T) {
		repo := newMockTxnRepo()
		account := &mockAccount{balance: 100_000, status: "ACTIVE"}
		svc := service.NewPaymentService(repo, account, &mockPublisher{})

		_, err := svc.InitiateRefund(context.Background(), "idem-2", &dto.RefundRequest{
			SourceAccountID:      "merchant-acct",
			DestinationAccountID: "payer-acct",
			Amount:               15_000,
			Currency:             "IDR",
			OriginalReference:    "missing-txn",
		}, "user-1")

		require.Error(t, err)
		assert.True(t, pkgerrors.IsValidation(err))
	})

	t.Run("rejects refund when original transaction did not succeed", func(t *testing.T) {
		repo := newMockTxnRepo()
		repo.byID["txn-failed"] = &dao.Transaction{ID: "txn-failed", Status: dto.StatusFailed}

		account := &mockAccount{balance: 100_000, status: "ACTIVE"}
		svc := service.NewPaymentService(repo, account, &mockPublisher{})

		_, err := svc.InitiateRefund(context.Background(), "idem-3", &dto.RefundRequest{
			SourceAccountID:      "merchant-acct",
			DestinationAccountID: "payer-acct",
			Amount:               15_000,
			Currency:             "IDR",
			OriginalReference:    "txn-failed",
		}, "user-1")

		require.Error(t, err)
		assert.True(t, pkgerrors.IsValidation(err))
	})

	t.Run("replays the same transaction for a duplicate idempotency key", func(t *testing.T) {
		repo := newMockTxnRepo()
		repo.byID["txn-orig"] = &dao.Transaction{ID: "txn-orig", Status: dto.StatusSuccess}

		account := &mockAccount{balance: 100_000, status: "ACTIVE"}
		svc := service.NewPaymentService(repo, account, &mockPublisher{})

		req := &dto.RefundRequest{
			SourceAccountID:      "merchant-acct",
			DestinationAccountID: "payer-acct",
			Amount:               15_000,
			Currency:             "IDR",
			OriginalReference:    "txn-orig",
		}

		first, err := svc.InitiateRefund(context.Background(), "idem-dup", req, "user-1")
		require.NoError(t, err)

		second, err := svc.InitiateRefund(context.Background(), "idem-dup", req, "user-1")
		require.NoError(t, err)

		assert.Equal(t, first.ID, second.ID)
		assert.Len(t, account.debits, 1, "second call must not move funds again")
	})
}
