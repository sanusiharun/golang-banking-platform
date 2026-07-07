package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgerrors "github.com/sanusi/banking/pkg/errors"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dao"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/dto"
	"github.com/sanusi/banking/services/payment-svc/internal/domain/repository"
	"github.com/sanusi/banking/services/payment-svc/internal/infra/accountclient"
	"github.com/sanusi/banking/services/payment-svc/internal/infra/eventpublisher"
	"github.com/sanusi/banking/services/payment-svc/internal/service"
)

// ── Mocks ─────────────────────────────────────────────────────────────────────

type acctCall struct {
	account string
	amount  int64
}

type mockAccount struct {
	balance       int64
	status        string
	debitErr      error
	creditFailFor string // account whose Credit call fails
	debits        []acctCall
	credits       []acctCall
}

func (m *mockAccount) GetAccount(_ context.Context, id string) (*accountclient.AccountInfo, error) {
	return &accountclient.AccountInfo{ID: id, Status: "ACTIVE", Currency: "IDR"}, nil
}

func (m *mockAccount) GetBalance(_ context.Context, id string) (*accountclient.BalanceInfo, error) {
	return &accountclient.BalanceInfo{AccountID: id, Balance: m.balance, Status: m.status, Currency: "IDR"}, nil
}

func (m *mockAccount) Debit(_ context.Context, id string, amount int64, _, _ string) error {
	m.debits = append(m.debits, acctCall{id, amount})
	return m.debitErr
}

func (m *mockAccount) Credit(_ context.Context, id string, amount int64, _, _ string) error {
	m.credits = append(m.credits, acctCall{id, amount})
	if id == m.creditFailFor {
		return errors.New("credit rejected")
	}
	return nil
}

var _ accountclient.AccountClient = (*mockAccount)(nil)

type mockTxnRepo struct {
	byID    map[string]*dao.Transaction
	byKey   map[string]*dao.Transaction
	counter int
}

func newMockTxnRepo() *mockTxnRepo {
	return &mockTxnRepo{byID: map[string]*dao.Transaction{}, byKey: map[string]*dao.Transaction{}}
}

func (m *mockTxnRepo) Create(_ context.Context, txn *dao.Transaction) error {
	if _, ok := m.byKey[txn.IdempotencyKey]; ok {
		return errors.New("duplicate idempotency_key")
	}
	m.counter++
	txn.ID = fmt.Sprintf("txn-%d", m.counter)
	m.byID[txn.ID] = txn
	m.byKey[txn.IdempotencyKey] = txn
	return nil
}

func (m *mockTxnRepo) UpdateStatus(_ context.Context, id, status string, reason *string) error {
	t, ok := m.byID[id]
	if !ok {
		return pkgerrors.NotFound("transaction", id)
	}
	t.Status = status
	t.FailureReason = reason
	return nil
}

func (m *mockTxnRepo) GetByID(_ context.Context, id string) (*dao.Transaction, error) {
	if t, ok := m.byID[id]; ok {
		return t, nil
	}
	return nil, pkgerrors.NotFound("transaction", id)
}

func (m *mockTxnRepo) GetByIdempotencyKey(_ context.Context, key string) (*dao.Transaction, error) {
	if t, ok := m.byKey[key]; ok {
		return t, nil
	}
	return nil, pkgerrors.NotFound("transaction", key)
}

func (m *mockTxnRepo) ListByAccount(_ context.Context, _ dto.ListFilter) ([]*dao.Transaction, int64, error) {
	return nil, 0, nil
}
func (m *mockTxnRepo) IncrementRetryCount(_ context.Context, _ string) error { return nil }
func (m *mockTxnRepo) GetReversal(_ context.Context, _ string) (*dao.Reversal, error) {
	return nil, pkgerrors.NotFound("reversal", "")
}
func (m *mockTxnRepo) CreateReversal(_ context.Context, _ *dao.Reversal) error { return nil }
func (m *mockTxnRepo) UpdateReversalStatus(_ context.Context, _, _ string, _ *string) error {
	return nil
}

var _ repository.TransactionRepository = (*mockTxnRepo)(nil)

type mockMerchantRepo struct {
	byID   map[string]*dao.Merchant
	byNMID map[string]*dao.Merchant
}

func newMockMerchantRepo() *mockMerchantRepo {
	return &mockMerchantRepo{byID: map[string]*dao.Merchant{}, byNMID: map[string]*dao.Merchant{}}
}

func (m *mockMerchantRepo) Create(_ context.Context, mm *dao.Merchant) error {
	if mm.ID == "" {
		mm.ID = "m-" + mm.NMID
	}
	m.byID[mm.ID] = mm
	m.byNMID[mm.NMID] = mm
	return nil
}
func (m *mockMerchantRepo) GetByID(_ context.Context, id string) (*dao.Merchant, error) {
	if v, ok := m.byID[id]; ok {
		return v, nil
	}
	return nil, pkgerrors.NotFound("merchant", id)
}
func (m *mockMerchantRepo) GetByNMID(_ context.Context, nmid string) (*dao.Merchant, error) {
	if v, ok := m.byNMID[nmid]; ok {
		return v, nil
	}
	return nil, pkgerrors.NotFound("merchant", nmid)
}

var _ repository.MerchantRepository = (*mockMerchantRepo)(nil)

type mockChargeRepo struct {
	byID map[string]*dao.QRISCharge
}

func newMockChargeRepo() *mockChargeRepo {
	return &mockChargeRepo{byID: map[string]*dao.QRISCharge{}}
}

func (m *mockChargeRepo) Create(_ context.Context, c *dao.QRISCharge) error {
	if c.ID == "" {
		c.ID = fmt.Sprintf("c-%d", len(m.byID)+1)
	}
	m.byID[c.ID] = c
	return nil
}
func (m *mockChargeRepo) GetByID(_ context.Context, id string) (*dao.QRISCharge, error) {
	if v, ok := m.byID[id]; ok {
		return v, nil
	}
	return nil, pkgerrors.NotFound("qris_charge", id)
}
func (m *mockChargeRepo) MarkPaid(_ context.Context, id, txnID string) error {
	c, ok := m.byID[id]
	if !ok {
		return pkgerrors.NotFound("qris_charge", id)
	}
	if c.Status != dto.QRISChargePending {
		return pkgerrors.Conflict("qris_charge", "status", "not pending")
	}
	c.Status = dto.QRISChargePaid
	c.PaidTxnID = &txnID
	return nil
}
func (m *mockChargeRepo) UpdateStatus(_ context.Context, id, status string) error {
	c, ok := m.byID[id]
	if !ok {
		return pkgerrors.NotFound("qris_charge", id)
	}
	c.Status = status
	return nil
}

var _ repository.QRISChargeRepository = (*mockChargeRepo)(nil)

type mockPublisher struct{ completed int }

func (m *mockPublisher) PublishCompleted(_ context.Context, _ eventpublisher.PaymentEvent) error {
	m.completed++
	return nil
}
func (m *mockPublisher) PublishFailed(_ context.Context, _ eventpublisher.PaymentEvent) error {
	return nil
}
func (m *mockPublisher) PublishReversed(_ context.Context, _ eventpublisher.PaymentEvent) error {
	return nil
}
func (m *mockPublisher) PublishCancelled(_ context.Context, _ eventpublisher.PaymentEvent) error {
	return nil
}

var _ eventpublisher.PaymentEventPublisher = (*mockPublisher)(nil)

// ── Fixtures ──────────────────────────────────────────────────────────────────

type harness struct {
	svc       service.QRISService
	account   *mockAccount
	txns      *mockTxnRepo
	merchants *mockMerchantRepo
	charges   *mockChargeRepo
	publisher *mockPublisher
}

const (
	merchantAcct = "acc-merchant"
	payerAcct    = "acc-payer"
)

func newHarness(balance int64) *harness {
	acct := &mockAccount{balance: balance, status: "ACTIVE"}
	txns := newMockTxnRepo()
	merchants := newMockMerchantRepo()
	charges := newMockChargeRepo()
	pub := &mockPublisher{}
	cfg := service.QRISConfig{
		AcquirerGUID: "ID.CO.QRIS.WWW",
		DefaultMCC:   "5411",
		Currency:     "IDR",
		ChargeTTL:    time.Hour,
	}
	return &harness{
		svc:       service.NewQRISService(merchants, charges, txns, acct, pub, cfg),
		account:   acct,
		txns:      txns,
		merchants: merchants,
		charges:   charges,
		publisher: pub,
	}
}

func (h *harness) seedMerchant() *dao.Merchant {
	m := &dao.Merchant{
		ID:        "m1",
		NMID:      "9360123456789012",
		Name:      "KOPI SANUSI",
		City:      "JAKARTA",
		MCC:       "5411",
		Country:   "ID",
		AccountID: merchantAcct,
		Currency:  "IDR",
		Status:    dto.MerchantActive,
	}
	_ = h.merchants.Create(context.Background(), m)
	return m
}

func (h *harness) seedCharge(qrType string, amount *int64) *dao.QRISCharge {
	c := &dao.QRISCharge{
		ID:         "c1",
		MerchantID: "m1",
		QRType:     qrType,
		QRString:   "PAYLOAD",
		Amount:     amount,
		Currency:   "IDR",
		Status:     dto.QRISChargePending,
	}
	_ = h.charges.Create(context.Background(), c)
	return c
}

func ptrInt64(v int64) *int64 { return &v }

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestPay_DynamicCharge_HappyPath(t *testing.T) {
	h := newHarness(5_000_000)
	h.seedMerchant()
	h.seedCharge(dto.QRTypeDynamic, ptrInt64(1_500_000))

	resp, err := h.svc.Pay(context.Background(), "key-1",
		&dto.QRISPayRequest{SourceAccountID: payerAcct, ChargeID: "c1"}, "user-1")
	require.NoError(t, err)

	assert.Equal(t, dto.StatusSuccess, resp.Status)
	assert.Equal(t, dto.TypeQRIS, resp.PaymentType)
	assert.Equal(t, int64(1_500_000), resp.Amount)
	assert.Equal(t, payerAcct, resp.SourceAccountID)
	assert.Equal(t, merchantAcct, resp.DestinationAccountID)

	require.Len(t, h.account.debits, 1)
	assert.Equal(t, acctCall{payerAcct, 1_500_000}, h.account.debits[0])
	require.Len(t, h.account.credits, 1)
	assert.Equal(t, acctCall{merchantAcct, 1_500_000}, h.account.credits[0])

	assert.Equal(t, dto.QRISChargePaid, h.charges.byID["c1"].Status)
	assert.Equal(t, 1, h.publisher.completed)
}

func TestPay_StaticCharge_UsesRequestAmount(t *testing.T) {
	h := newHarness(5_000_000)
	h.seedMerchant()
	h.seedCharge(dto.QRTypeStatic, nil)

	resp, err := h.svc.Pay(context.Background(), "key-2",
		&dto.QRISPayRequest{SourceAccountID: payerAcct, ChargeID: "c1", Amount: 200_000}, "user-1")
	require.NoError(t, err)

	assert.Equal(t, dto.StatusSuccess, resp.Status)
	assert.Equal(t, int64(200_000), resp.Amount)
	require.Len(t, h.account.debits, 1)
	assert.Equal(t, int64(200_000), h.account.debits[0].amount)
}

func TestPay_StaticCharge_MissingAmount(t *testing.T) {
	h := newHarness(5_000_000)
	h.seedMerchant()
	h.seedCharge(dto.QRTypeStatic, nil)

	_, err := h.svc.Pay(context.Background(), "key-3",
		&dto.QRISPayRequest{SourceAccountID: payerAcct, ChargeID: "c1"}, "user-1")
	require.Error(t, err)
	assert.True(t, pkgerrors.IsValidation(err))
	assert.Empty(t, h.account.debits, "no debit should occur without an amount")
}

func TestPay_InsufficientBalance(t *testing.T) {
	h := newHarness(1_000) // far below charge amount
	h.seedMerchant()
	h.seedCharge(dto.QRTypeDynamic, ptrInt64(1_500_000))

	_, err := h.svc.Pay(context.Background(), "key-4",
		&dto.QRISPayRequest{SourceAccountID: payerAcct, ChargeID: "c1"}, "user-1")
	require.Error(t, err)
	assert.True(t, pkgerrors.IsValidation(err))
	assert.Empty(t, h.account.debits)
	assert.Equal(t, dto.QRISChargePending, h.charges.byID["c1"].Status, "charge stays PENDING")
}

func TestPay_CreditFails_CompensatesAndFails(t *testing.T) {
	h := newHarness(5_000_000)
	h.seedMerchant()
	h.seedCharge(dto.QRTypeDynamic, ptrInt64(1_500_000))
	h.account.creditFailFor = merchantAcct // destination credit fails

	_, err := h.svc.Pay(context.Background(), "key-5",
		&dto.QRISPayRequest{SourceAccountID: payerAcct, ChargeID: "c1"}, "user-1")
	require.Error(t, err)

	// Debit happened once on the payer.
	require.Len(t, h.account.debits, 1)
	// Two credit attempts: the failed merchant credit, then the compensating
	// refund back to the payer.
	require.Len(t, h.account.credits, 2)
	assert.Equal(t, merchantAcct, h.account.credits[0].account)
	assert.Equal(t, payerAcct, h.account.credits[1].account, "compensation refunds the payer")

	assert.Equal(t, dto.StatusFailed, h.txns.byKey["key-5"].Status)
	assert.Equal(t, dto.QRISChargePending, h.charges.byID["c1"].Status, "unpaid charge stays PENDING")
}

func TestPay_IdempotentReplay(t *testing.T) {
	h := newHarness(5_000_000)
	h.seedMerchant()
	h.seedCharge(dto.QRTypeDynamic, ptrInt64(1_500_000))

	req := &dto.QRISPayRequest{SourceAccountID: payerAcct, ChargeID: "c1"}
	first, err := h.svc.Pay(context.Background(), "key-6", req, "user-1")
	require.NoError(t, err)

	second, err := h.svc.Pay(context.Background(), "key-6", req, "user-1")
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID, "replay returns the same transaction")
	assert.Len(t, h.account.debits, 1, "no second debit on replay")
	assert.Equal(t, 1, h.publisher.completed, "no duplicate event on replay")
}

func TestPay_SecondDistinctPayment_Conflict(t *testing.T) {
	h := newHarness(5_000_000)
	h.seedMerchant()
	h.seedCharge(dto.QRTypeDynamic, ptrInt64(1_500_000))

	req := &dto.QRISPayRequest{SourceAccountID: payerAcct, ChargeID: "c1"}
	_, err := h.svc.Pay(context.Background(), "key-7a", req, "user-1")
	require.NoError(t, err)

	// A different idempotency key trying to settle the already-paid charge.
	_, err = h.svc.Pay(context.Background(), "key-7b", req, "user-2")
	require.Error(t, err)
	assert.True(t, pkgerrors.IsConflict(err))
	assert.Len(t, h.account.debits, 1, "the second payer is never debited")
}

func TestGenerateCharge_Dynamic_ProducesDecodablePayload(t *testing.T) {
	h := newHarness(0)
	h.seedMerchant()

	charge, err := h.svc.GenerateCharge(context.Background(), &dto.QRISGenerateRequest{
		MerchantID: "m1", QRType: dto.QRTypeDynamic, Amount: 1_500_000, ReferenceLabel: "INV-9",
	})
	require.NoError(t, err)
	require.NotEmpty(t, charge.QRString)
	require.NotNil(t, charge.Amount)
	assert.Equal(t, int64(1_500_000), *charge.Amount)

	// The generated payload must decode cleanly with a valid CRC.
	decoded, err := h.svc.Decode(context.Background(), charge.QRString)
	require.NoError(t, err)
	assert.True(t, decoded.CRCValid)
	assert.True(t, decoded.IsDynamic)
	assert.Equal(t, "9360123456789012", decoded.NMID)
	require.NotNil(t, decoded.Amount)
	assert.Equal(t, int64(1_500_000), *decoded.Amount)
}
