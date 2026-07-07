package service

import (
	"context"
	"fmt"
	"log/slog"

	pkgerrors "github.com/sanusi/banking/pkg/errors"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/payment-svc/internal/infra/accountclient"
	"github.com/sanusi/banking/services/payment-svc/internal/infra/eventpublisher"
)

// orchestrator runs the canonical debit(source) → credit(destination) money
// movement with transaction-level idempotency and compensation on partial
// failure. It is the reusable core of every payment flow; the QRIS service
// composes it, and the (currently stubbed) transfer/merchant flows can adopt it
// later without duplicating the state machine.
type orchestrator struct {
	repo      transactionWriter
	account   accountclient.AccountClient
	publisher eventpublisher.PaymentEventPublisher
}

// transactionWriter is the subset of repository.TransactionRepository the
// orchestrator needs. Declaring it here keeps the dependency narrow and makes
// the orchestrator trivially mockable in tests.
type transactionWriter interface {
	Create(ctx context.Context, txn *dao.Transaction) error
	UpdateStatus(ctx context.Context, id, status string, failureReason *string) error
	GetByIdempotencyKey(ctx context.Context, key string) (*dao.Transaction, error)
}

// debitCreditInput describes one money movement to orchestrate.
type debitCreditInput struct {
	IdempotencyKey    string
	SourceAccountID   string
	DestAccountID     string
	Amount            int64 // minor units
	Currency          string
	PaymentType       string
	Channel           string
	Description       string
	ExternalReference string
	InitiatedBy       string
}

// executeDebitCredit performs the money movement and returns the persisted
// transaction in its terminal state.
//
// Idempotency: a prior transaction with the same key is returned unchanged
// (safe replay). The transactions.idempotency_key unique constraint is the
// race backstop.
//
// Failure handling: insufficient balance is a terminal validation error;
// a successful debit followed by a failed credit triggers a compensating
// credit back to the source before the transaction is marked FAILED.
func (o *orchestrator) executeDebitCredit(ctx context.Context, in debitCreditInput) (*dao.Transaction, error) {
	// ── Idempotency replay ────────────────────────────────────────────────────
	if existing, err := o.repo.GetByIdempotencyKey(ctx, in.IdempotencyKey); err == nil {
		return existing, nil
	} else if !pkgerrors.IsNotFound(err) {
		return nil, fmt.Errorf("orchestrator.executeDebitCredit lookup: %w", err)
	}

	// ── Balance & status pre-check ────────────────────────────────────────────
	balance, err := o.account.GetBalance(ctx, in.SourceAccountID)
	if err != nil {
		return nil, fmt.Errorf("orchestrator.executeDebitCredit balance: %w", err)
	}
	if balance.Status != "" && balance.Status != "ACTIVE" {
		return nil, pkgerrors.Validation("source_account_id", "source account is not active")
	}
	if balance.Balance < in.Amount {
		return nil, pkgerrors.Validation("amount", "insufficient balance")
	}

	// ── Persist PENDING ───────────────────────────────────────────────────────
	txn := &dao.Transaction{
		IdempotencyKey:       in.IdempotencyKey,
		PaymentType:          in.PaymentType,
		Channel:              in.Channel,
		SourceAccountID:      in.SourceAccountID,
		DestinationAccountID: in.DestAccountID,
		Amount:               in.Amount,
		Currency:             in.Currency,
		Status:               dto.StatusPending,
		InitiatedBy:          in.InitiatedBy,
	}
	if in.Description != "" {
		txn.Description = &in.Description
	}
	if in.ExternalReference != "" {
		txn.ExternalReference = &in.ExternalReference
	}
	if err := o.repo.Create(ctx, txn); err != nil {
		// A concurrent request with the same key may have won the race.
		if existing, e2 := o.repo.GetByIdempotencyKey(ctx, in.IdempotencyKey); e2 == nil {
			return existing, nil
		}
		return nil, fmt.Errorf("orchestrator.executeDebitCredit create: %w", err)
	}

	// ── Debit source ──────────────────────────────────────────────────────────
	if err := o.repo.UpdateStatus(ctx, txn.ID, dto.StatusProcessing, nil); err != nil {
		return nil, fmt.Errorf("orchestrator.executeDebitCredit mark processing: %w", err)
	}

	ref := txn.ID
	if err := o.account.Debit(ctx, in.SourceAccountID, in.Amount, in.Currency, ref); err != nil {
		o.markFailed(ctx, txn.ID, "debit failed")
		return nil, fmt.Errorf("orchestrator.executeDebitCredit debit: %w", err)
	}

	// ── Credit destination (with compensation) ────────────────────────────────
	if err := o.account.Credit(ctx, in.DestAccountID, in.Amount, in.Currency, ref); err != nil {
		// Compensate: refund the debited amount to the source. If the refund
		// itself fails the transaction is left FAILED for reconciliation.
		if cerr := o.account.Credit(ctx, in.SourceAccountID, in.Amount, in.Currency, ref+"-comp"); cerr != nil {
			slog.ErrorContext(ctx, "orchestrator: compensation credit failed",
				"transaction_id", txn.ID, "error", cerr)
		}
		o.markFailed(ctx, txn.ID, "credit failed")
		return nil, fmt.Errorf("orchestrator.executeDebitCredit credit: %w", err)
	}

	// ── Success ───────────────────────────────────────────────────────────────
	if err := o.repo.UpdateStatus(ctx, txn.ID, dto.StatusSuccess, nil); err != nil {
		return nil, fmt.Errorf("orchestrator.executeDebitCredit mark success: %w", err)
	}
	txn.Status = dto.StatusSuccess

	o.publishCompleted(ctx, txn)
	return txn, nil
}

// markFailed transitions the transaction to FAILED, logging (not returning) any
// error so the original cause is preserved for the caller.
func (o *orchestrator) markFailed(ctx context.Context, id, reason string) {
	if err := o.repo.UpdateStatus(ctx, id, dto.StatusFailed, &reason); err != nil {
		slog.ErrorContext(ctx, "orchestrator: failed to mark transaction FAILED",
			"transaction_id", id, "error", err)
	}
}

// publishCompleted emits the terminal event. Publish failures are logged, never
// fatal — the money has already moved.
func (o *orchestrator) publishCompleted(ctx context.Context, txn *dao.Transaction) {
	event := eventpublisher.PaymentEvent{
		TransactionID:        txn.ID,
		PaymentType:          txn.PaymentType,
		Status:               txn.Status,
		SourceAccountID:      txn.SourceAccountID,
		DestinationAccountID: txn.DestinationAccountID,
		Amount:               txn.Amount,
		Currency:             txn.Currency,
		InitiatedBy:          txn.InitiatedBy,
	}
	if err := o.publisher.PublishCompleted(ctx, event); err != nil {
		slog.WarnContext(ctx, "orchestrator: publish completed event failed",
			"transaction_id", txn.ID, "error", err)
	}
}
