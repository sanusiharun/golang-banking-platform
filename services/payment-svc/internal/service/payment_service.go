// Package service contains business logic and orchestration for payment-svc.
package service

import (
	"context"
	"fmt"

	pkgerrors "github.com/sanusi/banking/pkg/errors"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/repository"
	"github.com/sanusi/banking/services/payment-svc/internal/infra/accountclient"
	"github.com/sanusi/banking/services/payment-svc/internal/infra/eventpublisher"
)

// PaymentService defines all business operations for payment processing.
type PaymentService interface {
	InitiateTransfer(ctx context.Context, idempotencyKey string, req *dto.TransferRequest, initiatedBy string) (*dto.TransactionResponse, error)
	InitiateMerchantPayment(ctx context.Context, idempotencyKey string, req *dto.MerchantPaymentRequest, initiatedBy string) (*dto.TransactionResponse, error)
	InitiateFee(ctx context.Context, idempotencyKey string, req *dto.FeeRequest, initiatedBy string) (*dto.TransactionResponse, error)
	InitiateRefund(ctx context.Context, idempotencyKey string, req *dto.RefundRequest, initiatedBy string) (*dto.TransactionResponse, error)
	GetByID(ctx context.Context, id string) (*dto.TransactionResponse, error)
	List(ctx context.Context, filter dto.ListFilter) (*dto.TransactionListResponse, error)
	Reverse(ctx context.Context, id, initiatedBy string) (*dto.TransactionResponse, error)
	Cancel(ctx context.Context, id, initiatedBy string) (*dto.TransactionResponse, error)
	Retry(ctx context.Context, id, initiatedBy string) (*dto.TransactionResponse, error)
}

// paymentService is the concrete implementation of PaymentService.
type paymentService struct {
	repo      repository.TransactionRepository
	account   accountclient.AccountClient
	publisher eventpublisher.PaymentEventPublisher
	orch      *orchestrator
}

// NewPaymentService creates a new PaymentService.
func NewPaymentService(
	repo repository.TransactionRepository,
	account accountclient.AccountClient,
	publisher eventpublisher.PaymentEventPublisher,
) PaymentService {
	return &paymentService{
		repo:      repo,
		account:   account,
		publisher: publisher,
		orch:      &orchestrator{repo: repo, account: account, publisher: publisher},
	}
}

// GetByID retrieves a transaction by ID.
func (s *paymentService) GetByID(ctx context.Context, id string) (*dto.TransactionResponse, error) {
	txn, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("payment_service.GetByID: %w", err)
	}
	return toResponse(txn), nil
}

// List retrieves paginated transactions matching the filter.
func (s *paymentService) List(ctx context.Context, filter dto.ListFilter) (*dto.TransactionListResponse, error) {
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	items, total, err := s.repo.ListByAccount(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("payment_service.List: %w", err)
	}
	responses := make([]*dto.TransactionResponse, len(items))
	for i, t := range items {
		responses[i] = toResponse(t)
	}
	return &dto.TransactionListResponse{
		Items:   responses,
		Total:   total,
		Limit:   filter.Limit,
		Offset:  filter.Offset,
		HasMore: int64(filter.Offset+filter.Limit) < total,
	}, nil
}

// InitiateTransfer, InitiateMerchantPayment, InitiateFee, InitiateRefund,
// Reverse, Cancel, Retry are stubbed until E4 implementation.

func (s *paymentService) InitiateTransfer(_ context.Context, _ string, _ *dto.TransferRequest, _ string) (*dto.TransactionResponse, error) {
	return nil, errNotImplemented("InitiateTransfer")
}

func (s *paymentService) InitiateMerchantPayment(_ context.Context, _ string, _ *dto.MerchantPaymentRequest, _ string) (*dto.TransactionResponse, error) {
	return nil, errNotImplemented("InitiateMerchantPayment")
}

func (s *paymentService) InitiateFee(_ context.Context, _ string, _ *dto.FeeRequest, _ string) (*dto.TransactionResponse, error) {
	return nil, errNotImplemented("InitiateFee")
}

// InitiateRefund processes a refund against a previously completed transaction.
// Satisfies FR-04 (accept and process refund requests). Idempotency (NFR-01,
// AC-02, AC-12) is provided by orchestrator.executeDebitCredit, which replays
// any existing transaction for the same Idempotency-Key rather than moving
// funds twice.
func (s *paymentService) InitiateRefund(ctx context.Context, idempotencyKey string, req *dto.RefundRequest, initiatedBy string) (*dto.TransactionResponse, error) {
	if req.OriginalReference != "" {
		original, err := s.repo.GetByID(ctx, req.OriginalReference)
		if err != nil {
			if pkgerrors.IsNotFound(err) {
				return nil, pkgerrors.Validation("original_reference", "original transaction not found")
			}
			return nil, fmt.Errorf("payment_service.InitiateRefund lookup original: %w", err)
		}
		if original.Status != dto.StatusSuccess {
			return nil, pkgerrors.Validation("original_reference", "original transaction is not eligible for refund")
		}
	}

	txn, err := s.orch.executeDebitCredit(ctx, debitCreditInput{
		IdempotencyKey:    idempotencyKey,
		SourceAccountID:   req.SourceAccountID,
		DestAccountID:     req.DestinationAccountID,
		Amount:            req.Amount,
		Currency:          req.Currency,
		PaymentType:       dto.TypeRefund,
		Channel:           dto.ChannelAPI,
		Description:       req.Description,
		ExternalReference: req.OriginalReference,
		InitiatedBy:       initiatedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("payment_service.InitiateRefund: %w", err)
	}
	return toResponse(txn), nil
}

func (s *paymentService) Reverse(_ context.Context, _, _ string) (*dto.TransactionResponse, error) {
	return nil, errNotImplemented("Reverse")
}

func (s *paymentService) Cancel(_ context.Context, _, _ string) (*dto.TransactionResponse, error) {
	return nil, errNotImplemented("Cancel")
}

func (s *paymentService) Retry(_ context.Context, _, _ string) (*dto.TransactionResponse, error) {
	return nil, errNotImplemented("Retry")
}

func errNotImplemented(method string) error {
	return fmt.Errorf("%s: not implemented", method)
}
