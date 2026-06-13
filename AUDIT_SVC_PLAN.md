# Audit Trail Service — Architecture Plan

> Created: 2026-06-12
> Status: Planning

---

## Overview

`audit-svc` is an append-only event store that records every significant action
across the banking platform. It is **write-once, read-many** — no UPDATE or DELETE
is ever allowed against audit rows.

### Goals
- Full actor accountability: who did what, when, from where
- Correlatable with traces (OTEL TraceID linkage)
- Non-blocking to the calling service (async NATS primary path)
- Queryable for compliance, debugging, and support

---

## Port Assignment

| Service | Port |
|---|---|
| account-svc | 8081 (existing) |
| auth-svc | 8082 (existing) |
| **audit-svc** | **8083** ← new |
| payment-svc | 8084 (future) |
| notification-svc | 8085 (future) |

---

## Transport Strategy — Dual Mode

```
auth-svc / account-svc
        │
        ├─── NATS JetStream ──► audit.events.* ──► audit-svc consumer ──► PostgreSQL
        │        (async, primary path)
        │
        └─── HTTP POST /v1/audit/events   (sync fallback, critical ops only)
```

**Primary path — NATS JetStream (async)**
- Services publish an `AuditEvent` message to `audit.events.<action>` and return immediately.
- JetStream provides at-least-once delivery with acknowledgement.
- audit-svc runs a durable consumer and persists events to PostgreSQL.
- No latency added to the calling request path.

**Fallback path — HTTP (sync)**
- Used when a caller needs confirmation that the audit record was written
  (e.g., a compliance-sensitive admin action).
- `POST /v1/audit/events` — returns the created event ID.
- Services should prefer NATS; use HTTP only when durability confirmation is required.

---

## NATS JetStream Configuration

```
Stream name:   AUDIT
Subjects:      audit.events.>       (wildcard — catches all sub-topics)
Retention:     limits (max age: 7 days for hot stream)
Storage:       file
Consumer:      durable, name=audit-svc-consumer
Ack policy:    explicit
Max deliver:   5 (retry up to 5x before dead-lettering)
```

Topic convention:

| Topic | Events |
|---|---|
| `audit.events.auth.login` | Successful login |
| `audit.events.auth.login_failed` | Failed login attempt |
| `audit.events.auth.logout` | Logout |
| `audit.events.auth.token_refresh` | Token refresh |
| `audit.events.apikey.created` | API key created |
| `audit.events.apikey.revoked` | API key revoked |
| `audit.events.apikey.used` | API key authenticated a request |
| `audit.events.account.created` | New account opened |
| `audit.events.account.credit` | Credit operation |
| `audit.events.account.debit` | Debit operation |
| `audit.events.account.balance_read` | Balance queried |
| `audit.events.admin.service_account_created` | Service account created |
| `audit.events.admin.service_account_deleted` | Service account deleted |

---

## Shared Client — `pkg/audit`

Every service imports this package instead of NATS directly.
Keeps the event schema canonical and makes testing easy.

### File layout

```
pkg/audit/
├── event.go        ← AuditEvent struct + action constants
├── publisher.go    ← Publisher interface
├── nats.go         ← NATSPublisher (primary)
├── http.go         ← HTTPPublisher (sync fallback)
└── noop.go         ← NoopPublisher (tests / local dev with NATS off)
```

### `event.go`

```go
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

// Action constants — use these, never raw strings.
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
type AuditEvent struct {
    // Identity
    ActorType  string `json:"actor_type"`  // "user" | "service_account" | "system"
    ActorID    string `json:"actor_id"`    // user UUID or service account ID
    ActorEmail string `json:"actor_email"` // human-readable label; optional

    // What happened
    Action     string `json:"action"`      // use Action* constants
    Status     string `json:"status"`      // "success" | "failure" | "denied"

    // What it affected
    Resource   string `json:"resource"`    // "account", "api_key", "user", etc.
    ResourceID string `json:"resource_id"` // specific record ID; empty if N/A

    // Context
    ServiceName string `json:"service_name"` // "auth-svc", "account-svc"
    TraceID     string `json:"trace_id"`      // OTEL trace ID for cross-service correlation
    IPAddress   string `json:"ip_address"`
    UserAgent   string `json:"user_agent"`

    // Flexible payload — keep small; no PII, no secret values
    Metadata map[string]any `json:"metadata,omitempty"`

    // Filled by audit-svc on ingest (not by caller)
    ID        string    `json:"id,omitempty"`
    CreatedAt time.Time `json:"created_at,omitempty"`
}

// NATSSubject returns the NATS topic for this event.
func (e AuditEvent) NATSSubject() string {
    return "audit.events." + e.Action
}
```

### `publisher.go`

```go
package audit

import "context"

// Publisher sends an audit event to the audit pipeline.
// Implementations: NATSPublisher, HTTPPublisher, NoopPublisher.
type Publisher interface {
    Publish(ctx context.Context, event AuditEvent) error
}
```

### `nats.go` (primary)

```go
// NATSPublisher publishes AuditEvents to NATS JetStream.
// It marshals to JSON and publishes to audit.events.<action>.
// Errors are logged but never propagated to the caller — audit failure
// must never block a user-facing operation.
type NATSPublisher struct {
    js nats.JetStreamContext
}

func NewNATSPublisher(nc *nats.Conn) (*NATSPublisher, error) {
    js, err := nc.JetStream()
    // ... create/assert AUDIT stream
    return &NATSPublisher{js: js}, err
}

func (p *NATSPublisher) Publish(ctx context.Context, event AuditEvent) error {
    b, _ := json.Marshal(event)
    _, err := p.js.PublishAsync(event.NATSSubject(), b)
    return err
}
```

### `noop.go` (tests)

```go
type NoopPublisher struct{}
func (NoopPublisher) Publish(_ context.Context, _ AuditEvent) error { return nil }
```

---

## PostgreSQL Schema

```sql
-- migrations/audit-svc/001_create_audit_events.up.sql

CREATE TABLE audit_events (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_type   TEXT        NOT NULL,
    actor_id     TEXT        NOT NULL,
    actor_email  TEXT,
    action       TEXT        NOT NULL,
    status       TEXT        NOT NULL DEFAULT 'success',
    resource     TEXT,
    resource_id  TEXT,
    service_name TEXT        NOT NULL,
    trace_id     TEXT,
    ip_address   TEXT,
    user_agent   TEXT,
    metadata     JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- No UPDATE or DELETE — enforced below
REVOKE UPDATE, DELETE ON audit_events FROM PUBLIC;

-- Query patterns
CREATE INDEX idx_audit_actor        ON audit_events (actor_id, created_at DESC);
CREATE INDEX idx_audit_action       ON audit_events (action, created_at DESC);
CREATE INDEX idx_audit_resource     ON audit_events (resource, resource_id, created_at DESC);
CREATE INDEX idx_audit_trace        ON audit_events (trace_id) WHERE trace_id IS NOT NULL;
CREATE INDEX idx_audit_service_time ON audit_events (service_name, created_at DESC);
```

**Partitioning note**: not needed at launch. Add `PARTITION BY RANGE (created_at)` monthly
once volume requires it (typically >10M rows / month).

---

## Service Structure

Follow the exact same layout as `auth-svc` and `account-svc`:

```
services/audit-svc/
├── main.go
├── Dockerfile
├── .env
├── go.mod
├── internal/
│   ├── config/
│   │   └── config.go          ← env-based config
│   ├── domain/
│   │   ├── audit_event.go     ← domain model (mirrors pkg/audit.AuditEvent)
│   │   └── dto/
│   │       ├── ingest.go      ← IngestRequest (HTTP path)
│   │       └── query.go       ← QueryParams, EventResponse
│   ├── repository/
│   │   ├── audit_repo.go      ← interface
│   │   └── postgres/
│   │       └── audit_repo.go  ← SQL implementation
│   ├── services/
│   │   └── audit_service.go   ← business logic (validate, persist, enrich)
│   └── transport/
│       ├── handler.go         ← HTTP handlers
│       ├── consumer.go        ← NATS JetStream consumer loop
│       ├── response.go        ← stub (uses pkg/httpx)
│       └── routes.go
└── migrations/
    └── 001_create_audit_events.up.sql
```

---

## HTTP API Endpoints

All endpoints require a valid Bearer JWT or API key (same `AuthenticateAny` middleware).

| Method | Path | Description |
|---|---|---|
| `POST` | `/v1/audit/events` | Sync ingest (HTTP fallback path) |
| `GET` | `/v1/audit/events` | Query events with filters |
| `GET` | `/v1/audit/events/:id` | Get a single event |
| `GET` | `/v1/audit/actors/:actor_id/events` | All events for one actor |
| `GET` | `/v1/audit/resources/:resource/:resource_id/events` | All events on one resource |

### Query filters (`GET /v1/audit/events`)

```
?actor_id=<uuid>
?action=auth.login
?status=failure
?service=auth-svc
?trace_id=<trace>
?from=2026-06-01T00:00:00Z
?to=2026-06-12T23:59:59Z
?limit=50&cursor=<opaque_cursor>     ← keyset pagination on (created_at DESC, id)
```

### Response envelope

Uses `pkg/httpx.WriteSuccess` — same shape as all other services:

```json
{
  "success": true,
  "request_id": "...",
  "timestamp": "...",
  "data": {
    "events": [...],
    "next_cursor": "..."
  }
}
```

---

## Service Config (`.env`)

```env
# HTTP
HTTP_PORT=8083

# PostgreSQL (same ds_postgres)
DB_HOST=localhost
DB_PORT=5432
DB_NAME=auditdb
DB_USER=audit_user
DB_PASSWORD=audit_pass
DB_SSLMODE=disable

# NATS
NATS_URL=nats://localhost:9053
NATS_STREAM=AUDIT
NATS_CONSUMER=audit-svc-consumer

# JWT (public key only — same as account-svc)
JWT_PUBLIC_KEY_PATH=./keys/public.pem

# OTEL
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
OTEL_SERVICE_NAME=audit-svc
```

Docker overrides in root `docker-compose.yml`:
```yaml
DB_HOST: ds_postgres
NATS_URL: nats://platform-nats:4222
OTEL_EXPORTER_OTLP_ENDPOINT: banking-jaeger:4317
REDIS_ADDR: ""   # audit-svc has no Redis dependency
```

---

## Integration Points — Existing Services

### auth-svc changes

1. Wire `pkg/audit.Publisher` into `AuthHandler` and `APIKeyHandler` constructors.
2. After `Login` succeeds → `publisher.Publish(ctx, AuditEvent{Action: ActionAuthLogin, ...})`
3. After `Login` fails with `ErrInvalidCredentials` → publish `ActionAuthLoginFailed` with `Status: StatusFailure`
4. After `Logout` → publish `ActionAuthLogout`
5. After API key created/revoked → publish `ActionAPIKeyCreated` / `ActionAPIKeyRevoked`

The publisher call goes **after** the response is written (fire-and-forget pattern):

```go
// In Login handler, after httpx.WriteSuccess(w, r, resp):
go func() {
    _ = h.audit.Publish(r.Context(), audit.AuditEvent{
        ActorType:   audit.ActorTypeUser,
        ActorID:     resp.UserID,
        ActorEmail:  req.Email,
        Action:      audit.ActionAuthLogin,
        Status:      audit.StatusSuccess,
        ServiceName: "auth-svc",
        TraceID:     span.SpanContext().TraceID().String(),
        IPAddress:   r.RemoteAddr,
        UserAgent:   r.UserAgent(),
    })
}()
```

### account-svc changes

1. Wire `pkg/audit.Publisher` into `AccountHandler`.
2. Publish on: Credit, Debit, CreateAccount, GetBalance.
3. For Credit/Debit include `Metadata: map[string]any{"amount": req.Amount, "currency": "IDR"}` — **never include account numbers or balances in metadata**.

---

## NATS Stream Provisioning

audit-svc creates the JetStream stream on startup if it doesn't exist:

```go
func ensureStream(js nats.JetStreamContext) error {
    _, err := js.StreamInfo("AUDIT")
    if errors.Is(err, nats.ErrStreamNotFound) {
        _, err = js.AddStream(&nats.StreamConfig{
            Name:       "AUDIT",
            Subjects:   []string{"audit.events.>"},
            Retention:  nats.LimitsPolicy,
            MaxAge:     7 * 24 * time.Hour,
            Storage:    nats.FileStorage,
            Replicas:   1,
        })
    }
    return err
}
```

---

## docker-compose.yml Additions

Add to the root `docker-compose.yml` following the account-svc block:

```yaml
audit-svc:
  build:
    context: .
    dockerfile: services/audit-svc/Dockerfile
    args:
      SERVICE: audit-svc
  ports:
    - "8083:8083"
  env_file:
    - services/audit-svc/.env
  environment:
    HTTP_PORT: "8083"
    DB_HOST: ds_postgres
    NATS_URL: nats://platform-nats:4222
    OTEL_EXPORTER_OTLP_ENDPOINT: banking-jaeger:4317
  depends_on:
    platform-nats:
      condition: service_started
    ds_postgres:
      condition: service_started
  networks:
    - banking-net
```

---

## Prometheus Scrape Target

Add to `prometheus.yml`:

```yaml
- job_name: 'audit-svc'
  static_configs:
    - targets: ['host.docker.internal:8083']
```

---

## Makefile Additions

```makefile
audit-migrate:
	$(GO) run ./services/audit-svc/cmd/migrate/... up

audit-build:
	$(GO) build -o bin/audit-svc ./services/audit-svc/...
```

---

## Implementation Order

1. **`pkg/audit`** — define `AuditEvent`, `Publisher` interface, `NoopPublisher`, `NATSPublisher`
2. **PostgreSQL migration** — `001_create_audit_events.up.sql` + create `auditdb` and user
3. **audit-svc scaffold** — `config`, `domain`, `repository/postgres`, `services/audit_service`
4. **NATS consumer** — `transport/consumer.go` — subscribe to `audit.events.>`, persist via service
5. **HTTP transport** — `POST /v1/audit/events` (sync ingest) + query endpoints
6. **auth-svc integration** — wire publisher, emit events from Login/Logout/APIKey handlers
7. **account-svc integration** — wire publisher, emit events from Credit/Debit/CreateAccount
8. **docker-compose + Prometheus** — add service block and scrape target
9. **End-to-end test** — login → check NATS → check PostgreSQL row

---

## Traps to Avoid

| ❌ Don't | ✓ Do instead |
|---|---|
| Block request path waiting for audit confirmation | `go func()` for NATS publish, or accept eventual consistency |
| Store sensitive values in `Metadata` | Store only non-sensitive labels (amount, currency, resource ID) |
| Allow DELETE/UPDATE on `audit_events` | REVOKE at DB level; enforce via repo interface having no Delete method |
| Hardcode NATS subject strings in services | Use `audit.ActionAuthLogin` constants from `pkg/audit` |
| Create a new `nats.Conn` per service | Pass shared NATS connection via DI; one connection per process |
| Use `localhost:4222` inside Docker | Override with `platform-nats:4222` in docker-compose environment block |
| Miss NATS container name | Container is `platform-nats` (confirm from `platform/docker-compose.yml`) |

---

## Open Questions (decide before implementation)

1. **Database**: create a dedicated `auditdb` database, or a separate schema inside the existing `bankingdb`?
   — Recommendation: separate schema `audit` in the same PostgreSQL instance keeps it simple.

2. **Authentication on query endpoints**: only internal/admin access, or also accessible to end users (e.g., "my recent activity")?
   — Suggestion: internal-only for now; add user-scoped endpoint later.

3. **Retention policy**: how long should audit rows be kept?
   — Suggestion: 90 days hot (PostgreSQL), archive to MongoDB after that (future).

4. **NATS container name**: verify it's `platform-nats` in `platform/docker-compose.yml` before wiring.
