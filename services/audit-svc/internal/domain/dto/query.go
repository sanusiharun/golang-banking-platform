package dto

import "time"

// QueryParams holds the filters for GET /v1/audit/events.
// All fields are optional; omitted fields are not applied as filters.
type QueryParams struct {
	ActorID     string
	Action      string
	Status      string
	ServiceName string
	TraceID     string
	Resource    string
	ResourceID  string
	From        *time.Time
	To          *time.Time
	Limit       int    // default 50, max 200
	Cursor      string // opaque keyset cursor (base64 of "created_at|id")
}

// EventResponse is the JSON shape of a single audit event returned by the API.
type EventResponse struct {
	ID          string         `json:"id"`
	ActorType   string         `json:"actor_type"`
	ActorID     string         `json:"actor_id"`
	ActorEmail  string         `json:"actor_email,omitempty"`
	Action      string         `json:"action"`
	Status      string         `json:"status"`
	Resource    string         `json:"resource,omitempty"`
	ResourceID  string         `json:"resource_id,omitempty"`
	ServiceName string         `json:"service_name"`
	TraceID     string         `json:"trace_id,omitempty"`
	IPAddress   string         `json:"ip_address,omitempty"`
	UserAgent   string         `json:"user_agent,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// EventListResponse is the paginated list response data payload.
type EventListResponse struct {
	Events     []*EventResponse `json:"events"`
	NextCursor string           `json:"next_cursor,omitempty"`
	Total      int              `json:"total"`
}
