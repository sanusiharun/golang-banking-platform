package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	pkgerrors "github.com/sanusi/banking/pkg/errors"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/repository"
	"github.com/sanusi/banking/services/payment-svc/internal/infra/accountclient"
	"github.com/sanusi/banking/services/payment-svc/internal/infra/eventpublisher"
	"github.com/sanusi/banking/services/payment-svc/internal/qris"
)

// QRISConfig holds the codec defaults used when building QR payloads.
type QRISConfig struct {
	AcquirerGUID string        // EMVCo tag 26/00, e.g. "ID.CO.QRIS.WWW"
	DefaultMCC   string        // fallback Merchant Category Code
	Currency     string        // ISO 4217 alpha code, e.g. "IDR"
	ChargeTTL    time.Duration // default expiry for dynamic charges
}

// QRISService defines all QRIS business operations.
type QRISService interface {
	RegisterMerchant(ctx context.Context, req *dto.MerchantRegisterRequest) (*dto.MerchantResponse, error)
	GetMerchant(ctx context.Context, id string) (*dto.MerchantResponse, error)
	GenerateCharge(ctx context.Context, req *dto.QRISGenerateRequest) (*dto.QRISChargeResponse, error)
	Decode(ctx context.Context, qrString string) (*dto.QRISDecodedResponse, error)
	Pay(ctx context.Context, idempotencyKey string, req *dto.QRISPayRequest, initiatedBy string) (*dto.TransactionResponse, error)
}

type qrisService struct {
	merchants repository.MerchantRepository
	charges   repository.QRISChargeRepository
	txns      repository.TransactionRepository
	account   accountclient.AccountClient
	orch      *orchestrator
	cfg       QRISConfig
}

// NewQRISService creates a QRISService.
func NewQRISService(
	merchants repository.MerchantRepository,
	charges repository.QRISChargeRepository,
	txns repository.TransactionRepository,
	account accountclient.AccountClient,
	publisher eventpublisher.PaymentEventPublisher,
	cfg QRISConfig,
) QRISService {
	return &qrisService{
		merchants: merchants,
		charges:   charges,
		txns:      txns,
		account:   account,
		orch:      &orchestrator{repo: txns, account: account, publisher: publisher},
		cfg:       cfg,
	}
}

// ── Merchant registry ─────────────────────────────────────────────────────────

func (s *qrisService) RegisterMerchant(ctx context.Context, req *dto.MerchantRegisterRequest) (*dto.MerchantResponse, error) {
	// The linked credit account must exist in account-svc.
	if _, err := s.account.GetAccount(ctx, req.AccountID); err != nil {
		return nil, pkgerrors.Validation("account_id", "linked account not found or unreachable")
	}

	nmid := req.NMID
	if nmid == "" {
		nmid = generateNMID()
	}
	currency := req.Currency
	if currency == "" {
		currency = s.cfg.Currency
	}

	m := &dao.Merchant{
		NMID:      nmid,
		Name:      req.Name,
		City:      req.City,
		MCC:       req.MCC,
		Country:   qris.CountryID,
		AccountID: req.AccountID,
		Currency:  currency,
		Status:    dto.MerchantActive,
	}
	if req.PostalCode != "" {
		m.PostalCode = &req.PostalCode
	}
	if err := s.merchants.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("qris_service.RegisterMerchant: %w", err)
	}
	return merchantToResponse(m), nil
}

func (s *qrisService) GetMerchant(ctx context.Context, id string) (*dto.MerchantResponse, error) {
	m, err := s.merchants.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("qris_service.GetMerchant: %w", err)
	}
	return merchantToResponse(m), nil
}

// ── QR generation ─────────────────────────────────────────────────────────────

func (s *qrisService) GenerateCharge(ctx context.Context, req *dto.QRISGenerateRequest) (*dto.QRISChargeResponse, error) {
	m, err := s.merchants.GetByID(ctx, req.MerchantID)
	if err != nil {
		return nil, fmt.Errorf("qris_service.GenerateCharge: %w", err)
	}
	if m.Status != dto.MerchantActive {
		return nil, pkgerrors.Validation("merchant_id", "merchant is not active")
	}

	payload := qris.Payload{
		PointOfInitiation:    initiationFor(req.QRType),
		MerchantAccountGUID:  s.cfg.AcquirerGUID,
		MerchantNMID:         m.NMID,
		MerchantCategoryCode: m.MCC,
		TransactionCurrency:  qris.CurrencyIDR,
		CountryCode:          m.Country,
		MerchantName:         m.Name,
		MerchantCity:         m.City,
		ReferenceLabel:       req.ReferenceLabel,
		BillNumber:           req.BillNumber,
	}
	if m.PostalCode != nil {
		payload.PostalCode = *m.PostalCode
	}

	var amountPtr *int64
	if req.QRType == dto.QRTypeDynamic {
		payload.TransactionAmount = minorToMajor(req.Amount)
		amt := req.Amount
		amountPtr = &amt
	}

	qrString, err := qris.Encode(payload)
	if err != nil {
		return nil, fmt.Errorf("qris_service.GenerateCharge encode: %w", err)
	}

	charge := &dao.QRISCharge{
		MerchantID: m.ID,
		QRType:     req.QRType,
		QRString:   qrString,
		Amount:     amountPtr,
		Currency:   m.Currency,
		Status:     dto.QRISChargePending,
	}
	if req.ReferenceLabel != "" {
		charge.ReferenceLabel = &req.ReferenceLabel
	}
	if req.BillNumber != "" {
		charge.BillNumber = &req.BillNumber
	}
	if exp := s.chargeExpiry(req); exp != nil {
		charge.ExpiresAt = exp
	}

	if err := s.charges.Create(ctx, charge); err != nil {
		return nil, fmt.Errorf("qris_service.GenerateCharge persist: %w", err)
	}
	return chargeToResponse(charge), nil
}

// ── QR decoding ───────────────────────────────────────────────────────────────

func (s *qrisService) Decode(_ context.Context, qrString string) (*dto.QRISDecodedResponse, error) {
	p, err := qris.Decode(qrString)
	if err != nil {
		return nil, pkgerrors.Validation("qr_string", "malformed QRIS payload")
	}

	resp := &dto.QRISDecodedResponse{
		NMID:           p.MerchantNMID,
		MerchantName:   p.MerchantName,
		MerchantCity:   p.MerchantCity,
		MCC:            p.MerchantCategoryCode,
		Currency:       numericToCurrency(p.TransactionCurrency),
		ReferenceLabel: p.ReferenceLabel,
		BillNumber:     p.BillNumber,
		IsDynamic:      p.IsDynamic(),
		CRCValid:       p.CRCValid,
	}
	if p.TransactionAmount != "" {
		if minor, err := majorToMinor(p.TransactionAmount); err == nil {
			resp.Amount = &minor
		}
	}
	return resp, nil
}

// ── QR payment ────────────────────────────────────────────────────────────────

// paymentResolution is the outcome of resolving a QRIS Pay request into the
// merchant/amount/reference needed for the actual debit/credit, or an early
// response (a safe replay) that must be returned to the caller unchanged.
type paymentResolution struct {
	merchant      *dao.Merchant
	amount        int64
	extRef        string
	charge        *dao.QRISCharge          // non-nil only for a charge-based payment
	earlyResponse *dto.TransactionResponse // non-nil: return this directly, skip debit/credit
}

// resolveChargePayment resolves a payment made against a pre-generated QRIS charge.
func (s *qrisService) resolveChargePayment(ctx context.Context, req *dto.QRISPayRequest, idempotencyKey string) (paymentResolution, error) {
	charge, err := s.charges.GetByID(ctx, req.ChargeID)
	if err != nil {
		return paymentResolution{}, fmt.Errorf("qris_service.Pay charge: %w", err)
	}
	// Reject a second distinct settlement of the same charge, but allow the
	// original payer to replay their own request idempotently.
	if charge.Status != dto.QRISChargePending {
		if replay := s.replayForCharge(ctx, charge, idempotencyKey); replay != nil {
			return paymentResolution{earlyResponse: replay}, nil
		}
		return paymentResolution{}, pkgerrors.Conflict("qris_charge", "status", "already settled")
	}
	merchant, err := s.merchants.GetByID(ctx, charge.MerchantID)
	if err != nil {
		return paymentResolution{}, fmt.Errorf("qris_service.Pay merchant: %w", err)
	}
	amount, err := amountForCharge(charge, req.Amount)
	if err != nil {
		return paymentResolution{}, err
	}
	return paymentResolution{merchant: merchant, amount: amount, extRef: charge.ID, charge: charge}, nil
}

// resolveQRStringPayment resolves a payment made by scanning a raw QRIS string.
func (s *qrisService) resolveQRStringPayment(ctx context.Context, req *dto.QRISPayRequest) (paymentResolution, error) {
	p, derr := qris.Decode(req.QRString)
	if derr != nil {
		return paymentResolution{}, pkgerrors.Validation("qr_string", "malformed QRIS payload")
	}
	if !p.CRCValid {
		return paymentResolution{}, pkgerrors.Validation("qr_string", "invalid QRIS checksum")
	}
	merchant, err := s.merchants.GetByNMID(ctx, p.MerchantNMID)
	if err != nil {
		return paymentResolution{}, fmt.Errorf("qris_service.Pay merchant: %w", err)
	}
	amount, err := amountForDecoded(p, req.Amount)
	if err != nil {
		return paymentResolution{}, err
	}
	return paymentResolution{merchant: merchant, amount: amount, extRef: p.MerchantNMID}, nil
}

func (s *qrisService) Pay(ctx context.Context, idempotencyKey string, req *dto.QRISPayRequest, initiatedBy string) (*dto.TransactionResponse, error) {
	var resolution paymentResolution
	var err error
	if req.ChargeID != "" {
		resolution, err = s.resolveChargePayment(ctx, req, idempotencyKey)
	} else {
		resolution, err = s.resolveQRStringPayment(ctx, req)
	}
	if err != nil {
		return nil, err
	}
	if resolution.earlyResponse != nil {
		return resolution.earlyResponse, nil
	}

	txn, err := s.orch.executeDebitCredit(ctx, debitCreditInput{
		IdempotencyKey:    idempotencyKey,
		SourceAccountID:   req.SourceAccountID,
		DestAccountID:     resolution.merchant.AccountID,
		Amount:            resolution.amount,
		Currency:          resolution.merchant.Currency,
		PaymentType:       dto.TypeQRIS,
		Channel:           dto.ChannelQRIS,
		Description:       req.Description,
		ExternalReference: resolution.extRef,
		InitiatedBy:       initiatedBy,
	})
	if err != nil {
		return nil, err
	}

	// Settle the charge (PENDING → PAID). The repo guards on PENDING, so a
	// concurrent/replayed settlement is a harmless no-op we can ignore.
	if resolution.charge != nil && txn.Status == dto.StatusSuccess {
		if merr := s.charges.MarkPaid(ctx, resolution.charge.ID, txn.ID); merr != nil && !pkgerrors.IsConflict(merr) {
			return nil, fmt.Errorf("qris_service.Pay mark paid: %w", merr)
		}
	}
	return toResponse(txn), nil
}

// replayForCharge returns the settling transaction if it belongs to the same
// idempotency key, enabling a safe replay of an already-paid charge.
func (s *qrisService) replayForCharge(ctx context.Context, charge *dao.QRISCharge, idempotencyKey string) *dto.TransactionResponse {
	if charge.PaidTxnID == nil {
		return nil
	}
	paid, err := s.txns.GetByID(ctx, *charge.PaidTxnID)
	if err != nil || paid.IdempotencyKey != idempotencyKey {
		return nil
	}
	return toResponse(paid)
}

func (s *qrisService) chargeExpiry(req *dto.QRISGenerateRequest) *time.Time {
	var ttl time.Duration
	switch {
	case req.ExpirySeconds > 0:
		ttl = time.Duration(req.ExpirySeconds) * time.Second
	case req.QRType == dto.QRTypeDynamic:
		ttl = s.cfg.ChargeTTL
	default:
		return nil // static QR codes are reusable and do not expire
	}
	if ttl <= 0 {
		return nil
	}
	exp := time.Now().UTC().Add(ttl)
	return &exp
}

// ── free helpers ──────────────────────────────────────────────────────────────

func amountForCharge(charge *dao.QRISCharge, reqAmount int64) (int64, error) {
	if charge.QRType == dto.QRTypeDynamic {
		if charge.Amount == nil {
			return 0, pkgerrors.Validation("charge_id", "dynamic charge has no amount")
		}
		return *charge.Amount, nil
	}
	if reqAmount <= 0 {
		return 0, pkgerrors.Validation("amount", "amount is required for a static QR")
	}
	return reqAmount, nil
}

func amountForDecoded(p *qris.Payload, reqAmount int64) (int64, error) {
	if p.IsDynamic() {
		minor, err := majorToMinor(p.TransactionAmount)
		if err != nil {
			return 0, pkgerrors.Validation("qr_string", "invalid amount in QR")
		}
		return minor, nil
	}
	if reqAmount <= 0 {
		return 0, pkgerrors.Validation("amount", "amount is required for a static QR")
	}
	return reqAmount, nil
}

func initiationFor(qrType string) string {
	if qrType == dto.QRTypeStatic {
		return qris.InitiationStatic
	}
	return qris.InitiationDynamic
}

// minorToMajor renders int64 minor units as an EMVCo 2-decimal major string.
func minorToMajor(minor int64) string {
	neg := ""
	if minor < 0 {
		neg = "-"
		minor = -minor
	}
	return fmt.Sprintf("%s%d.%02d", neg, minor/100, minor%100)
}

// majorToMinor parses an EMVCo major-unit decimal string into int64 minor units.
func majorToMinor(s string) (int64, error) {
	parts := strings.SplitN(strings.TrimSpace(s), ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	var frac int64
	if len(parts) == 2 {
		f := parts[1]
		for len(f) < 2 {
			f += "0"
		}
		f = f[:2]
		if frac, err = strconv.ParseInt(f, 10, 64); err != nil {
			return 0, err
		}
	}
	return whole*100 + frac, nil
}

func numericToCurrency(numeric string) string {
	if numeric == qris.CurrencyIDR {
		return "IDR"
	}
	return numeric
}

// generateNMID produces a simulated 15-digit National Merchant ID.
// Real NMIDs are assigned by the acquirer; this is deterministic-length filler
// for a self-contained system.
func generateNMID() string {
	var sb strings.Builder
	sb.WriteString("9360") // simulated acquirer prefix
	for i := 0; i < 11; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(10)) //nolint:errcheck // crypto/rand.Reader failure is a fatal system-entropy condition, not something this simulated-merchant-ID generator can meaningfully recover from
		sb.WriteString(n.String())
	}
	return sb.String()
}

// ── mappers ───────────────────────────────────────────────────────────────────

func merchantToResponse(m *dao.Merchant) *dto.MerchantResponse {
	r := &dto.MerchantResponse{
		ID:        m.ID,
		NMID:      m.NMID,
		Name:      m.Name,
		City:      m.City,
		MCC:       m.MCC,
		Country:   m.Country,
		AccountID: m.AccountID,
		Currency:  m.Currency,
		Status:    m.Status,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
	if m.PostalCode != nil {
		r.PostalCode = *m.PostalCode
	}
	return r
}

func chargeToResponse(c *dao.QRISCharge) *dto.QRISChargeResponse {
	r := &dto.QRISChargeResponse{
		ID:         c.ID,
		MerchantID: c.MerchantID,
		QRType:     c.QRType,
		QRString:   c.QRString,
		Amount:     c.Amount,
		Currency:   c.Currency,
		Status:     c.Status,
		PaidTxnID:  c.PaidTxnID,
		ExpiresAt:  c.ExpiresAt,
		CreatedAt:  c.CreatedAt,
	}
	if c.ReferenceLabel != nil {
		r.ReferenceLabel = *c.ReferenceLabel
	}
	if c.BillNumber != nil {
		r.BillNumber = *c.BillNumber
	}
	return r
}
