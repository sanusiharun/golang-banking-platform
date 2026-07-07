// Package qris implements the EMVCo Merchant-Presented Mode (MPM) QR code
// format used by QRIS (Quick Response Code Indonesian Standard).
//
// The format is a flat sequence of Tag-Length-Value (TLV) data objects, where
// Tag and Length are each two ASCII digits and Value is Length bytes long. Some
// tags (26–51 merchant account, 62 additional data) are themselves templates
// whose value is a nested TLV sequence. The final data object (tag 63) carries
// a CRC-16/CCITT-FALSE checksum over everything up to and including "6304".
//
// This package is pure and deterministic — no I/O, no external dependencies —
// which makes it straightforward to unit-test exhaustively.
package qris

// EMVCo root tag identifiers.
const (
	tagPayloadFormat  = "00"
	tagInitiation     = "01"
	tagMerchantAcct   = "26" // primary QRIS merchant account information template
	tagMCC            = "52"
	tagCurrency       = "53"
	tagAmount         = "54"
	tagCountry        = "58"
	tagMerchantName   = "59"
	tagMerchantCity   = "60"
	tagPostalCode     = "61"
	tagAdditionalData = "62"
	tagCRC            = "63"
)

// Sub-tags within the merchant account template (tag 26).
const (
	subGUID     = "00"
	subNMID     = "01"
	subCriteria = "03"
)

// Sub-tags within the additional data template (tag 62).
const (
	subBillNumber    = "01"
	subReferenceLbl  = "05"
	subTerminalLabel = "07"
)

// Point-of-initiation method values (tag 01).
const (
	InitiationStatic  = "11" // reusable QR; payer enters the amount
	InitiationDynamic = "12" // single-use QR; amount embedded in tag 54
)

// Well-known fixed values.
const (
	PayloadFormatV1 = "01"
	CurrencyIDR     = "360" // ISO 4217 numeric code for Indonesian Rupiah
	CountryID       = "ID"
)

// Payload is the decoded/decodable representation of a QRIS MPM QR string.
// Amounts here are the EMVCo major-unit decimal string (e.g. "15000.00"); the
// service layer is responsible for converting to/from int64 minor units.
type Payload struct {
	PayloadFormatIndicator string // tag 00 — always "01"
	PointOfInitiation      string // tag 01 — InitiationStatic | InitiationDynamic

	MerchantAccountGUID  string // tag 26/00 — acquirer reverse-domain GUID
	MerchantNMID         string // tag 26/01 — National Merchant ID
	MerchantCriteria     string // tag 26/03 — optional (e.g. "UMI")
	MerchantCategoryCode string // tag 52 — 4-digit MCC
	TransactionCurrency  string // tag 53 — "360" for IDR
	TransactionAmount    string // tag 54 — present only for dynamic QR
	CountryCode          string // tag 58 — "ID"
	MerchantName         string // tag 59
	MerchantCity         string // tag 60
	PostalCode           string // tag 61 — optional

	BillNumber     string // tag 62/01 — optional
	ReferenceLabel string // tag 62/05 — optional
	TerminalLabel  string // tag 62/07 — optional

	CRC string // tag 63 — checksum as parsed/encoded

	// CRCValid reports whether the parsed CRC matched a freshly computed one.
	// It is only meaningful for a Payload produced by Decode.
	CRCValid bool
}

// IsDynamic reports whether the payload is a single-use (amount-embedded) QR.
func (p *Payload) IsDynamic() bool {
	return p.PointOfInitiation == InitiationDynamic
}
