package dto

import "time"

// ── Service Account DTOs ──────────────────────────────────────────────────────

type CreateServiceAccountRequest struct {
	Name        string   `json:"name"        validate:"required,min=2,max=100"`
	Description string   `json:"description" validate:"max=500"`
	TenantID    string   `json:"tenant_id"   validate:"max=100"`
	Roles       []string `json:"roles"       validate:"required,min=1,dive,min=1"`
}

type ServiceAccountResponse struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	TenantID    string    `json:"tenant_id"`
	Roles       []string  `json:"roles"`
	IsActive    bool      `json:"is_active"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type UpdateServiceAccountRequest struct {
	Roles    []string `json:"roles"     validate:"omitempty,min=1,dive,min=1"`
	IsActive *bool    `json:"is_active"`
}

// ── API Key DTOs ──────────────────────────────────────────────────────────────

type CreateAPIKeyRequest struct {
	Name      string     `json:"name"       validate:"required,min=2,max=100"`
	ExpiresAt *time.Time `json:"expires_at"` // nil = non-expiring
}

// CreateAPIKeyResponse includes the raw key — returned ONCE, never again.
type CreateAPIKeyResponse struct {
	KeyID     string     `json:"key_id"`
	RawKey    string     `json:"key"`        // bp_live_<32chars> — store this securely
	KeyPrefix string     `json:"key_prefix"` // first 10 chars for identification
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type APIKeyResponse struct {
	ID               string     `json:"id"`
	ServiceAccountID string     `json:"service_account_id"`
	Name             string     `json:"name"`
	KeyPrefix        string     `json:"key_prefix"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}
