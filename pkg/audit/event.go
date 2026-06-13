// Package audit provides the canonical audit event types and publisher interface
// for the banking platform. Every service that needs to emit audit events imports
// this package instead of sending NATS messages directly.
//
// Design goals:
//   - Single source of truth for event schema and action constants
//   - Publisher interface keeps callers decoupled from transport (NATS / HTTP / noop)
//   - Audit failure must never block user-facing operations
package audit

import "time"

// ActorType identifies who triggered the action.
const (
	ActorTypeUser           = "user"
	ActorTypeServiceAccount = "service_account"
	ActorTypeSystem         = "system"
)

// Status of the audited action.
const (
	StatusSuccess = "success"
	StatusFailure = "failure"
	StatusDenied  = "denied"
)

// Action constants — always use these, never raw strings.
const (
	ActionAuthLogin          = "auth.login"
	ActionAuthLoginFailed    = "auth.login_failed"
	ActionAuthLogout         = "auth.logout"
	ActionAuthTokenRefresh   = "auth.token_refresh"
	ActionAPIKeyCreated      = "apikey.created"
	ActionAPIKeyRevoked      = "apikey.revoked"
	ActionAPIKeyUsed         = "apikey.used"
	ActionAccountCreated     = "account.created"
	ActionAccountCredit      = "account.credit"
	ActionAccountDebit       = "account.debit"
	ActionAccountBalanceRead = "account.balance_read"
	ActionAdminSvcAccCreated = "admin.service_account_created"
	ActionAdminSvcAccDeleted = "admin.service_account_deleted"
)

// AuditEvent is the canonical record for every significant platform action.
// Fields marked "filled by audit-svc" must not be set by the publisher — they
// are assigned during ingest so callers cannot forge IDs or timestamps.
type AuditEvent struct {
	// Who acted
	ActorType  string `json:"actor_type"`  // ActorType* constant
	ActorID    string `json:"actor_id"`    // user UUID or service account ID
	ActorEmail string `json:"actor_email"` // human-readable label; optional

	// What happened
	Action string `json:"action"` // Action* constant
	Status string `json:"status"` // Status* constant

	// What was affected
	Resource   string `json:"resource"`    // "account", "api_key", "user", etc.
	ResourceID string `json:"resource_id"` // specific record ID; empty if N/A

	// Request context
	ServiceName string `json:"service_name"` // "auth-svc", "account-svc"
	TraceID     string `json:"trace_id"`     // OTEL trace ID for cross-service correlation
	IPAddress   string `json:"ip_address"`
	UserAgent   string `json:"user_agent"`

	// Flexible payload — keep small; no PII, no secrets, no account numbers
	Metadata map[string]any `json:"metadata,omitempty"`

	// Filled by audit-svc on ingest — not set by the publishing service
	ID        string    `json:"id,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// NATSSubject returns the NATS topic for this event, e.g. "audit.events.auth.login".
func (e AuditEvent) NATSSubject() string {
	return "audit.events." + e.Action
}
