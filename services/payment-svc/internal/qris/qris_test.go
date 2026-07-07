package qris

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCRC16_CanonicalCheckValue(t *testing.T) {
	// CRC-16/CCITT-FALSE of "123456789" is the well-known vector 0x29B1.
	assert.Equal(t, "29B1", CRC16("123456789"))
}

func TestCRC16_KnownEMVCoPrefix(t *testing.T) {
	// A minimal payload's CRC is stable; recompute must match itself.
	data := "00020101021263" + "04"
	first := CRC16(data)
	assert.Len(t, first, 4)
	assert.Equal(t, first, CRC16(data), "CRC must be deterministic")
}

func dynamicPayload() Payload {
	return Payload{
		PointOfInitiation:    InitiationDynamic,
		MerchantAccountGUID:  "ID.CO.QRIS.WWW",
		MerchantNMID:         "ID1234567890123",
		MerchantCategoryCode: "5411",
		TransactionAmount:    "15000.00",
		MerchantName:         "KOPI SANUSI",
		MerchantCity:         "JAKARTA",
		PostalCode:           "12190",
		ReferenceLabel:       "INV-001",
	}
}

func TestEncode_RoundTrip_Dynamic(t *testing.T) {
	p := dynamicPayload()

	s, err := Encode(p)
	require.NoError(t, err)

	// Structural sanity: fixed-value tags present, dynamic amount embedded.
	assert.True(t, strings.HasPrefix(s, "000201"), "payload format tag first")
	assert.Contains(t, s, "0102"+InitiationDynamic, "dynamic point-of-initiation")
	assert.Contains(t, s, "5303"+CurrencyIDR, "IDR currency tag")
	assert.Contains(t, s, "5802"+CountryID, "country tag")
	assert.Contains(t, s, "540815000.00", "amount tag 54 length+value")

	decoded, err := Decode(s)
	require.NoError(t, err)
	assert.True(t, decoded.CRCValid, "freshly encoded payload must have a valid CRC")
	assert.True(t, decoded.IsDynamic())
	assert.Equal(t, p.MerchantNMID, decoded.MerchantNMID)
	assert.Equal(t, p.MerchantAccountGUID, decoded.MerchantAccountGUID)
	assert.Equal(t, p.MerchantCategoryCode, decoded.MerchantCategoryCode)
	assert.Equal(t, "15000.00", decoded.TransactionAmount)
	assert.Equal(t, p.MerchantName, decoded.MerchantName)
	assert.Equal(t, p.MerchantCity, decoded.MerchantCity)
	assert.Equal(t, p.PostalCode, decoded.PostalCode)
	assert.Equal(t, p.ReferenceLabel, decoded.ReferenceLabel)
}

func TestEncode_Static_OmitsAmount(t *testing.T) {
	p := dynamicPayload()
	p.PointOfInitiation = InitiationStatic
	p.TransactionAmount = "" // static QR carries no amount

	s, err := Encode(p)
	require.NoError(t, err)
	assert.Contains(t, s, "0102"+InitiationStatic)
	assert.NotContains(t, s, "5408", "static QR must not embed a tag-54 amount")

	decoded, err := Decode(s)
	require.NoError(t, err)
	assert.False(t, decoded.IsDynamic())
	assert.Empty(t, decoded.TransactionAmount)
	assert.True(t, decoded.CRCValid)
}

func TestEncode_MissingRequiredField(t *testing.T) {
	p := dynamicPayload()
	p.MerchantNMID = ""
	_, err := Encode(p)
	assert.ErrorIs(t, err, ErrMissingField)
}

func TestDecode_TamperedCRC(t *testing.T) {
	p := dynamicPayload()
	s, err := Encode(p)
	require.NoError(t, err)

	// Flip a character in the merchant name region; CRC must no longer match.
	tampered := strings.Replace(s, "KOPI SANUSI", "KOP1 SANUSI", 1)
	require.NotEqual(t, s, tampered)

	decoded, err := Decode(tampered)
	require.NoError(t, err, "structure is still valid, only the CRC is wrong")
	assert.False(t, decoded.CRCValid, "tampered payload must report CRCValid=false")
}

func TestDecode_MalformedLength(t *testing.T) {
	// Tag length 99 but no value bytes follow → overrun.
	_, err := Decode("0099")
	assert.ErrorIs(t, err, ErrMalformed)
}

func TestDecode_NonNumericLength(t *testing.T) {
	_, err := Decode("00XX01")
	assert.ErrorIs(t, err, ErrMalformed)
}
