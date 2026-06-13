// Package dto contains request and response DTOs for audit-svc.
package dto

// IngestRequest is the body for POST /v1/audit/events (sync HTTP path).
// It mirrors pkg/audit.AuditEvent — callers must not set ID or CreatedAt.
type IngestRequest struct {
	ActorType   string         `json:"actor_type"   validate:"required,oneof=user service_account system"`
	ActorID     string         `json:"actor_id"     validate:"required"`
	ActorEmail  string         `json:"actor_email"`
	Action      string         `json:"action"       validate:"required"`
	Status      string         `json:"status"       validate:"required,oneof=success failure denied"`
	Resource    string         `json:"resource"`
	ResourceID  string         `json:"resource_id"`
	ServiceName string         `json:"service_name" validate:"required"`
	TraceID     string         `json:"trace_id"`
	IPAddress   string         `json:"ip_address"`
	UserAgent   string         `json:"user_agent"`
	Metadata    map[string]any `json:"metadata"`
}
