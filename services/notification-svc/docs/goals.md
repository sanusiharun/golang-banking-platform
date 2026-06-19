# goals.md — notification-svc

> Source of truth for all requirements. Every downstream document traces back here.

---

## Service Identity

| Property | Value |
|---|---|
| Service name | `notification-svc` |
| Port | `8084` |
| Database | `banking_notifications` (PostgreSQL) |
| Owner domain | Notification / Communication |
| Criticality tier | **High** — required for transactional alerts, OTP, account events |
| Language | Go 1.26 |
| Framework | `chi` v5 + stdlib |

---

## Business Objectives

| ID | Statement |
|---|---|
| `BO-01` | Provide a single centralised service through which all banking platform microservices send notifications to end-users and external systems. |
| `BO-02` | Decouple notification delivery from business logic so that upstream services are not affected by channel provider latency or failure. |
| `BO-03` | Support reusable notification templates to standardise messaging across channels and reduce duplication in calling services. |
| `BO-04` | Enable scheduled and recurring notifications to support time-based communication workflows (e.g., statement reminders, expiry alerts). |
| `BO-05` | Provide operational visibility into notification delivery status so that support staff can diagnose and retry failures without engineering intervention. |

---

## Functional Requirements

| ID | Requirement |
|---|---|
| `FR-01` | The service MUST accept notification requests via a REST API and via NATS JetStream. |
| `FR-02` | The service MUST support five delivery channels: EMAIL, SMS, PUSH, WHATSAPP, WEBHOOK. |
| `FR-03` | The service MUST persist every notification request and maintain its lifecycle status (PENDING → PROCESSING → SENT / FAILED / RETRYING / CANCELLED / DELIVERED). |
| `FR-04` | The service MUST implement idempotency: if a request with a duplicate `idempotency_key` arrives, the original record MUST be returned without creating a duplicate. |
| `FR-05` | The service MUST support template-based notifications where the caller supplies a `template_code` and a map of variables; the service resolves and renders the template. |
| `FR-06` | The service MUST support direct (raw) notifications where the caller supplies a pre-rendered payload without referencing a template. |
| `FR-07` | Notification delivery MUST be asynchronous: a notification request is accepted (201), persisted as PENDING, and handed to a background dispatcher worker pool. |
| `FR-08` | The service MUST support configurable retry behaviour per notification (max retries 0–10, default 3). When `max_retries` is exhausted the notification MUST be marked FAILED. |
| `FR-09` | The service MUST allow authorised users to manually cancel PENDING or RETRYING notifications. |
| `FR-10` | The service MUST allow authorised users to manually retry a FAILED notification. |
| `FR-11` | The service MUST provide a template management API: create, read, update, soft-delete, preview, and list templates. |
| `FR-12` | The service MUST support notification scheduling: one-time (fire at a specific UTC timestamp) and recurring (cron expression, 5-field standard format). |
| `FR-13` | The service MUST provide CRUD and enable/disable control for schedules. |
| `FR-14` | The service MUST expose a paginated, filterable list of notifications for administrative review. |
| `FR-15` | The service MUST expose liveness and readiness health checks. |
| `FR-16` | The service MUST authenticate all non-health API requests using RS256 JWT. API key authentication MUST be supported as an alternative. |
| `FR-17` | The service MUST implement role-based authorisation: ADMIN has full access; TELLER has read-only and limited action access. |

---

## Non-Functional Requirements

| ID | Requirement | Target |
|---|---|---|
| `NFR-01` | **Throughput** | Dispatcher worker pool MUST process ≥ 500 notifications per minute under steady-state load. |
| `NFR-02` | **API latency (write)** | `POST /v1/notifications` p99 ≤ 200 ms (excludes channel delivery, which is async). |
| `NFR-03` | **API latency (read)** | `GET /v1/notifications` p99 ≤ 300 ms for result sets ≤ 1000 rows. |
| `NFR-04` | **Availability** | Service MUST achieve ≥ 99.5% uptime in production. |
| `NFR-05` | **Concurrency safety** | Dispatcher MUST be safe to run in multiple replicas simultaneously (no duplicate delivery). |
| `NFR-06` | **Observability** | Service MUST expose Prometheus metrics and OpenTelemetry traces for all notification lifecycle events. |
| `NFR-07` | **Security** | No secrets (keys, credentials) MUST be logged or returned in API responses. |
| `NFR-08` | **Graceful shutdown** | Service MUST drain in-flight work within 30 seconds before exiting. |
| `NFR-09` | **Rate limiting** | Public-facing routes MUST be rate-limited (default 1000 RPS, burst 2000). |

---

## Constraints

| ID | Constraint |
|---|---|
| `C-01` | Language: Go only. No CGO. |
| `C-02` | HTTP framework: `chi` v5. No Gin, Echo, or Fiber. |
| `C-03` | Storage: PostgreSQL (`banking_notifications` database). No shared database with other services. |
| `C-04` | Async transport: NATS JetStream stream `NOTIFICATIONS`. |
| `C-05` | Authentication: RS256 JWT signed by `auth-svc`. Public key is injected as an env var; this service MUST NOT contact `auth-svc` at runtime. |
| `C-06` | Network: all inter-service communication uses the `banking-net` Docker bridge network. |
| `C-07` | Logging: `slog` only. No `fmt.Println`. |
| `C-08` | HTTP helpers: `pkg/httpx` only. No service-local response.go. |
| `C-09` | Channel providers for EMAIL, SMS, PUSH, WHATSAPP are stubs in the current version. Only WEBHOOK is production-ready. |
| `C-10` | Observability: Prometheus on `/metrics`; OTel tracing on OTLP exporter. Both are opt-in via environment flags. |

---

## Assumptions

| ID | Assumption |
|---|---|
| `A-01` | Calling services (auth-svc, account-svc, etc.) will include valid RS256 JWTs issued by the platform's auth-svc. |
| `A-02` | NATS JetStream is available and the `NOTIFICATIONS` stream is pre-created before this service starts. |
| `A-03` | Channel delivery latency (SMTP, Twilio, etc.) is outside the SLA of this service; only acceptance latency is measured. |
| `A-04` | The scheduler worker runs as a single goroutine per replica. Distributed locking is not required at the current scale. |
| `A-05` | Template variable rendering uses Go's standard `text/template` and `html/template`; no sandboxed scripting engine is required. |
| `A-06` | `idempotency_key` uniqueness is caller-enforced. The service only deduplicates within its own DB. |

---

## Acceptance Criteria

| ID | Criterion | Verifies |
|---|---|---|
| `AC-01` | `POST /v1/notifications` with a valid JWT, valid channel and recipient returns 201 and a record with `status=PENDING`. | FR-01, FR-03, FR-07 |
| `AC-02` | Two requests with the same `idempotency_key` return the same notification ID and only one DB record exists. | FR-04 |
| `AC-03` | `POST /v1/notifications` with `template_code` and `template_vars` returns 201 and the dispatcher renders the template before delivery. | FR-05 |
| `AC-04` | After `max_retries` failures, notification status is `FAILED`. | FR-08 |
| `AC-05` | `POST /v1/notifications/{id}/cancel` on a PENDING notification returns 200 with `status=CANCELLED`. | FR-09 |
| `AC-06` | `POST /v1/notifications/{id}/cancel` on a CANCELLED notification returns 409 Conflict. | FR-09 |
| `AC-07` | `POST /v1/notifications/{id}/retry` on a FAILED notification returns 200 with `status=PENDING` and `retry_count=0`. | FR-10 |
| `AC-08` | `POST /v1/templates` creates a template with `version=1` and `active=true`. | FR-11 |
| `AC-09` | `DELETE /v1/templates/{id}` sets `active=false` and the template no longer appears in `GetByCode` queries. | FR-11 |
| `AC-10` | `POST /v1/templates/{id}/preview` with valid variables returns rendered subject and body. | FR-11 |
| `AC-11` | `POST /v1/schedules` with a valid `cron_expr` returns 201 and `next_run_at` is populated. | FR-12 |
| `AC-12` | `POST /v1/schedules` with a one-time `scheduled_at` returns 201 and the schedule auto-disables after the first run. | FR-12 |
| `AC-13` | `POST /v1/schedules/{id}/disable` sets `enabled=false` and the scheduler does not fire that schedule. | FR-13 |
| `AC-14` | `GET /v1/notifications` without a JWT returns 401. | FR-16 |
| `AC-15` | A TELLER-role JWT cannot call `POST /v1/schedules` (returns 403). | FR-17 |
| `AC-16` | `/healthz/ready` returns 503 when Postgres or NATS is unavailable. | FR-15 |
| `AC-17` | Two concurrent dispatcher replicas claiming PENDING notifications never deliver the same notification twice. | NFR-05 |

---

## Service Boundaries

**In scope:**
- Accepting notification requests via REST and NATS
- Persisting notification records and their status transitions
- Managing notification templates (CRUD, preview, render)
- Managing notification schedules (one-time and recurring)
- Dispatching notifications to channel providers (worker pool)
- Exposing delivery metrics and traces

**Out of scope:**
- Real-time delivery receipts / push-back from external providers (webhooks from Twilio, etc.)
- User preference management (opt-in / opt-out) — caller is responsible for gate-keeping
- Email bounce handling
- Provider SDK management (responsibility of the infrastructure team when stubs are replaced)
- End-user notification history UI
- Audit log publishing (currently ensured in NATS but not yet published by this service)
