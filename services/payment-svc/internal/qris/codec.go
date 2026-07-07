package qris

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Codec errors. Callers should translate these to domain errors at the service
// boundary.
var (
	ErrMalformed    = errors.New("qris: malformed TLV payload")
	ErrValueTooLong = errors.New("qris: field value exceeds 99 bytes")
	ErrMissingField = errors.New("qris: required field missing")
)

// tlv formats a single EMVCo data object: two-digit tag, two-digit length,
// then the value. Values longer than 99 bytes cannot be represented.
func tlv(tag, value string) (string, error) {
	if len(value) > 99 {
		return "", fmt.Errorf("%w: tag %s (%d bytes)", ErrValueTooLong, tag, len(value))
	}
	return fmt.Sprintf("%s%02d%s", tag, len(value), value), nil
}

// Encode renders a Payload as a QRIS MPM QR string, appending the tag-63 CRC.
// Required fields: PointOfInitiation, MerchantNMID, MerchantCategoryCode,
// MerchantName, MerchantCity. Missing optional templates are omitted.
func Encode(p Payload) (string, error) {
	if p.PointOfInitiation == "" || p.MerchantNMID == "" ||
		p.MerchantCategoryCode == "" || p.MerchantName == "" || p.MerchantCity == "" {
		return "", ErrMissingField
	}

	// Apply defaults for the fixed-value fields.
	format := firstNonEmpty(p.PayloadFormatIndicator, PayloadFormatV1)
	guid := firstNonEmpty(p.MerchantAccountGUID, "ID.CO.QRIS.WWW")
	currency := firstNonEmpty(p.TransactionCurrency, CurrencyIDR)
	country := firstNonEmpty(p.CountryCode, CountryID)

	// Merchant account template (tag 26): GUID + NMID (+ optional criteria).
	merchantAcct, err := buildTemplate([]tagValue{
		{subGUID, guid},
		{subNMID, p.MerchantNMID},
		{subCriteria, p.MerchantCriteria},
	})
	if err != nil {
		return "", err
	}

	// Additional data template (tag 62): only if any sub-field is present.
	additional, err := buildTemplate([]tagValue{
		{subBillNumber, p.BillNumber},
		{subReferenceLbl, p.ReferenceLabel},
		{subTerminalLabel, p.TerminalLabel},
	})
	if err != nil {
		return "", err
	}

	// Assemble root objects in canonical EMVCo order.
	objects := []tagValue{
		{tagPayloadFormat, format},
		{tagInitiation, p.PointOfInitiation},
		{tagMerchantAcct, merchantAcct},
		{tagMCC, p.MerchantCategoryCode},
		{tagCurrency, currency},
	}
	if p.TransactionAmount != "" {
		objects = append(objects, tagValue{tagAmount, p.TransactionAmount})
	}
	objects = append(objects,
		tagValue{tagCountry, country},
		tagValue{tagMerchantName, p.MerchantName},
		tagValue{tagMerchantCity, p.MerchantCity},
	)
	if p.PostalCode != "" {
		objects = append(objects, tagValue{tagPostalCode, p.PostalCode})
	}
	if additional != "" {
		objects = append(objects, tagValue{tagAdditionalData, additional})
	}

	var sb strings.Builder
	for _, o := range objects {
		s, err := tlv(o.tag, o.value)
		if err != nil {
			return "", err
		}
		sb.WriteString(s)
	}

	// The CRC is computed over the payload including the "6304" tag+length
	// prefix, then appended as the value.
	sb.WriteString(tagCRC)
	sb.WriteString("04")
	crc := CRC16(sb.String())
	sb.WriteString(crc)

	return sb.String(), nil
}

// Decode parses a QRIS MPM QR string into a Payload and validates its CRC.
// Structurally invalid input (bad lengths, truncation) returns an error; a
// well-formed string with a mismatched CRC returns a Payload with
// CRCValid=false rather than an error, so callers can surface the distinction.
func Decode(s string) (*Payload, error) {
	root, err := parseTLV(s)
	if err != nil {
		return nil, err
	}

	p := &Payload{
		PayloadFormatIndicator: root[tagPayloadFormat],
		PointOfInitiation:      root[tagInitiation],
		MerchantCategoryCode:   root[tagMCC],
		TransactionCurrency:    root[tagCurrency],
		TransactionAmount:      root[tagAmount],
		CountryCode:            root[tagCountry],
		MerchantName:           root[tagMerchantName],
		MerchantCity:           root[tagMerchantCity],
		PostalCode:             root[tagPostalCode],
		CRC:                    root[tagCRC],
	}

	if acct, ok := root[tagMerchantAcct]; ok {
		sub, err := parseTLV(acct)
		if err != nil {
			return nil, err
		}
		p.MerchantAccountGUID = sub[subGUID]
		p.MerchantNMID = sub[subNMID]
		p.MerchantCriteria = sub[subCriteria]
	}

	if add, ok := root[tagAdditionalData]; ok {
		sub, err := parseTLV(add)
		if err != nil {
			return nil, err
		}
		p.BillNumber = sub[subBillNumber]
		p.ReferenceLabel = sub[subReferenceLbl]
		p.TerminalLabel = sub[subTerminalLabel]
	}

	// The CRC covers everything except the 4-char checksum value itself.
	if len(s) >= 4 {
		p.CRCValid = strings.EqualFold(CRC16(s[:len(s)-4]), s[len(s)-4:])
	}

	return p, nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

type tagValue struct {
	tag   string
	value string
}

// buildTemplate concatenates the non-empty sub-objects into a nested TLV value.
// Returns "" if every sub-value is empty.
func buildTemplate(items []tagValue) (string, error) {
	var sb strings.Builder
	for _, it := range items {
		if it.value == "" {
			continue
		}
		s, err := tlv(it.tag, it.value)
		if err != nil {
			return "", err
		}
		sb.WriteString(s)
	}
	return sb.String(), nil
}

// parseTLV splits a flat TLV string into a tag→value map. It fails if a length
// header is non-numeric or points past the end of the string, or if trailing
// bytes remain that do not form a complete object.
func parseTLV(s string) (map[string]string, error) {
	m := make(map[string]string)
	for i := 0; i < len(s); {
		if i+4 > len(s) {
			return nil, ErrMalformed
		}
		tag := s[i : i+2]
		length, err := strconv.Atoi(s[i+2 : i+4])
		if err != nil {
			return nil, fmt.Errorf("%w: bad length at %q", ErrMalformed, tag)
		}
		start := i + 4
		end := start + length
		if end > len(s) {
			return nil, fmt.Errorf("%w: tag %s overruns payload", ErrMalformed, tag)
		}
		m[tag] = s[start:end]
		i = end
	}
	return m, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
