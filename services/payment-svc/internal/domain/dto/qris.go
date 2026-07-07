package dto

import "time"

// ── QRIS constants ────────────────────────────────────────────────────────────

const (
	QRTypeStatic  = "STATIC"
	QRTypeDynamic = "DYNAMIC"

	QRISChargePending   = "PENDING"
	QRISChargePaid      = "PAID"
	QRISChargeExpired   = "EXPIRED"
	QRISChargeCancelled = "CANCELLED"

	MerchantActive   = "ACTIVE"
	MerchantInactive = "INACTIVE"
)

// ── Merchant registry ─────────────────────────────────────────────────────────

// MerchantRegisterRequest is the payload for POST /v1/merchants.
// NMID is optional; when omitted the service generates a simulated one.
type MerchantRegisterRequest struct {
	Name       string `json:"name"        validate:"required,max=25"`
	City       string `json:"city"        validate:"required,max=15"`
	PostalCode string `json:"postal_code" validate:"omitempty,max=10"`
	MCC        string `json:"mcc"         validate:"required,len=4,numeric"`
	NMID       string `json:"nmid"        validate:"omitempty,max=32"`
	AccountID  string `json:"account_id"  validate:"required,uuid"`
	Currency   string `json:"currency"    validate:"omitempty,len=3,uppercase"`
}

// MerchantResponse is returned for merchant endpoints.
type MerchantResponse struct {
	ID         string    `json:"id"`
	NMID       string    `json:"nmid"`
	Name       string    `json:"name"`
	City       string    `json:"city"`
	PostalCode string    `json:"postal_code,omitempty"`
	MCC        string    `json:"mcc"`
	Country    string    `json:"country"`
	AccountID  string    `json:"account_id"`
	Currency   string    `json:"currency"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ── QR generation ─────────────────────────────────────────────────────────────

// QRISGenerateRequest is the payload for POST /v1/payments/qris/generate.
// Amount is in minor units and required only for dynamic QR codes.
type QRISGenerateRequest struct {
	MerchantID     string `json:"merchant_id"     validate:"required,uuid"`
	QRType         string `json:"qr_type"         validate:"required,oneof=STATIC DYNAMIC"`
	Amount         int64  `json:"amount"          validate:"required_if=QRType DYNAMIC,omitempty,gt=0"`
	ReferenceLabel string `json:"reference_label" validate:"omitempty,max=25"`
	BillNumber     string `json:"bill_number"     validate:"omitempty,max=25"`
	ExpirySeconds  int    `json:"expiry_seconds"  validate:"omitempty,gt=0"`
}

// QRISChargeResponse describes a generated QR charge.
type QRISChargeResponse struct {
	ID             string     `json:"id"`
	MerchantID     string     `json:"merchant_id"`
	QRType         string     `json:"qr_type"`
	QRString       string     `json:"qr_string"`
	Amount         *int64     `json:"amount,omitempty"`
	Currency       string     `json:"currency"`
	ReferenceLabel string     `json:"reference_label,omitempty"`
	BillNumber     string     `json:"bill_number,omitempty"`
	Status         string     `json:"status"`
	PaidTxnID      *string    `json:"paid_txn_id,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// ── QR decoding ───────────────────────────────────────────────────────────────

// QRISDecodeRequest is the payload for POST /v1/payments/qris/decode.
type QRISDecodeRequest struct {
	QRString string `json:"qr_string" validate:"required"`
}

// QRISDecodedResponse is the structured view of a scanned QR string.
// Amount is converted to minor units; nil for static QR codes.
type QRISDecodedResponse struct {
	NMID           string `json:"nmid"`
	MerchantName   string `json:"merchant_name"`
	MerchantCity   string `json:"merchant_city"`
	MCC            string `json:"mcc"`
	Amount         *int64 `json:"amount,omitempty"`
	Currency       string `json:"currency"`
	ReferenceLabel string `json:"reference_label,omitempty"`
	BillNumber     string `json:"bill_number,omitempty"`
	IsDynamic      bool   `json:"is_dynamic"`
	CRCValid       bool   `json:"crc_valid"`
}

// ── QR payment ────────────────────────────────────────────────────────────────

// QRISPayRequest is the payload for POST /v1/payments/qris/pay.
// Exactly one of ChargeID or QRString identifies the target. Amount (minor
// units) is required only when paying a static QR that carries no amount.
type QRISPayRequest struct {
	SourceAccountID string `json:"source_account_id" validate:"required,uuid"`
	ChargeID        string `json:"charge_id"         validate:"omitempty,uuid,required_without=QRString"`
	QRString        string `json:"qr_string"         validate:"omitempty,required_without=ChargeID"`
	Amount          int64  `json:"amount"            validate:"omitempty,gt=0"`
	Description     string `json:"description"       validate:"omitempty,max=500"`
}
