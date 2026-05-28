package transport

import (
	"crypto/rsa"
	"errors"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"

	pkgcrypto "github.com/sanusi/banking/pkg/crypto"
	pkgmiddleware "github.com/sanusi/banking/pkg/middleware"
)

// InspectHandler exposes POST /auth/inspect for local development only.
// It parses a JWT, decrypts the Subject claim, and returns all claims
// in human-readable form — useful for debugging tokens without needing
// an external JWT decoder.
//
// Never register this handler in staging or production.
type InspectHandler struct {
	publicKey  *rsa.PublicKey
	subjectKey []byte
	issuer     string
}

func NewInspectHandler(publicKey *rsa.PublicKey, subjectKey []byte, issuer string) *InspectHandler {
	return &InspectHandler{
		publicKey:  publicKey,
		subjectKey: subjectKey,
		issuer:     issuer,
	}
}

type inspectRequest struct {
	Token string `json:"token"`
}

type inspectResponse struct {
	UserID           string   `json:"user_id"`
	SubjectEncrypted string   `json:"subject_encrypted"`
	TenantID         string   `json:"tenant_id"`
	Roles            []string `json:"roles"`
	Issuer           string   `json:"issuer"`
	IssuedAt         string   `json:"issued_at"`
	ExpiresAt        string   `json:"expires_at"`
	Valid            bool     `json:"valid"`
	Expired          bool     `json:"expired"`
}

// Inspect handles POST /auth/inspect.
//
// Request:
//
//	{"token":"<rs256-jwt>"}
//
// Response 200:
//
//	{"success":true,"data":{"user_id":"usr_admin_001","subject_encrypted":"...","roles":[...],...}}
func (h *InspectHandler) Inspect(w http.ResponseWriter, r *http.Request) {
	var req inspectRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "MISSING_TOKEN", "token is required")
		return
	}

	claims := &pkgmiddleware.Claims{}

	// Parse without expiry enforcement so we can still inspect expired tokens.
	token, err := jwt.ParseWithClaims(req.Token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return h.publicKey, nil
	}, jwt.WithIssuer(h.issuer))

	valid := err == nil && token.Valid
	expired := false
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			// Still return claims for expired tokens — useful for debugging.
			expired = true
		} else {
			// Bad signature, wrong algorithm, malformed token, etc.
			writeError(w, http.StatusUnauthorized, "INVALID_TOKEN", err.Error())
			return
		}
	}

	// Decrypt subject → user ID.
	userID := claims.Subject
	if len(h.subjectKey) > 0 && claims.Subject != "" {
		if decrypted, err := pkgcrypto.DecryptSubject(h.subjectKey, claims.Subject); err == nil {
			userID = decrypted
		}
	}

	issuedAt := ""
	if claims.IssuedAt != nil {
		issuedAt = claims.IssuedAt.UTC().Format(time.RFC3339)
	}
	expiresAt := ""
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.UTC().Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, inspectResponse{
		UserID:           userID,
		SubjectEncrypted: claims.Subject,
		TenantID:         claims.TenantID,
		Roles:            claims.Roles,
		Issuer:           claims.Issuer,
		IssuedAt:         issuedAt,
		ExpiresAt:        expiresAt,
		Valid:            valid,
		Expired:          expired,
	})
}
