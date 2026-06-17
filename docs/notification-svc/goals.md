# notification-svc — Goals

## Service Identity

| Field | Value |
|---|---|
| Service name | `notification-svc` |
| Port | `8084` |
| Database | `banking_notifications` (Postgres) |
| Owner domain | Platform / Communications |
| Criticality tier | High |

---

## Business Objectives

| ID | Objective |
|---|---|
| BO-01 | Provide a centralised, channel-agnostic notification platform that decouples notification delivery from business services |
| BO-02 | Ensure all customer-facing communications are auditable, traceable, and reliably delivered |
| BO-03 | Enable the business to add new communication channels without modifying consuming services |
| BO-04 | Support time-sensitive and scheduled communications at scale |

---

## Functional Requirements

### Channel Support
| ID | Requirement |
|---|---|
| FR-01 | Send notifications via Email, SMS, Push Notification, WhatsApp, and Webhook channels |
| FR-02 | The channel abstraction must allow new channels to be registered without changing core routing logic |

### Template Management
| ID | Requirement |
|---|---|
| FR-03 | Manage reusable notification templates identified by a unique code |
| FR-04 | Support HTML format for email templates and plain-text format for other channels |
| FR-05 | Render templates with dynamic variable substitution |
| FR-06 | Support template versioning; the version is incremented on each update |
| FR-07 | Provide a template preview endpoint that renders a template with supplied variables without sending |

### Notification Processing
| ID | Requirement |
|---|---|
| FR-08 | Accept notification requests via HTTP (synchronous) and NATS JetStream (asynchronous) |
| FR-09 | Support immediate delivery (no scheduled_at) and scheduled delivery (scheduled_at in future) |
| FR-10 | Process notifications via a background worker pool |
| FR-11 | Retry failed notifications up to a configurable max_retries with exponential back-off |
| FR-12 | Enforce idempotent processing via an idempotency_key unique constraint |
| FR-13 | Move permanently failed notifications to a dead-letter state (status=FAILED, retry_count=max_retries) |
| FR-14 | Support cancellation of pending/scheduled notifications |
| FR-15 | Support on-demand retry of failed notifications |

### Notification History
| ID | Requirement |
|---|---|
| FR-16 | Persist every notification with: id, channel, recipient, template_id, template_code, template_vars, payload, status, provider_ref, provider_resp, error_message, retry_count, scheduled_at, sent_at, delivered_at, created_at, updated_at |
| FR-17 | Support statuses: PENDING, PROCESSING, SENT, DELIVERED, FAILED, RETRYING, CANCELLED |
| FR-18 | List notification history with filtering by status, channel, recipient, template_code, date range, schedule_id |
| FR-19 | Support pagination on history queries |

### Scheduler
| ID | Requirement |
|---|---|
| FR-20 | Create one-time scheduled notifications (scheduled_at = specific timestamp) |
| FR-21 | Create recurring notifications using a cron expression |
| FR-22 | Enable and disable individual schedules |
| FR-23 | Track last_run_at and next_run_at on every schedule |
| FR-24 | On each scheduler tick, claim due schedules and create notification records |

### APIs
| ID | Requirement |
|---|---|
| FR-25 | Notification API: send, retry, cancel, get detail, list history |
| FR-26 | Template API: create, update, delete, get, list, preview |
| FR-27 | Schedule API: create, update, delete, enable, disable, get, list |

### Observability
| ID | Requirement |
|---|---|
| FR-28 | Expose `/metrics` (Prometheus), `/healthz/live`, `/healthz/ready` |
| FR-29 | Emit structured slog logs with request_id, user_id context |
| FR-30 | Instrument all service and repository operations with OTEL traces |
| FR-31 | Expose custom metrics: notifications_sent_total, notifications_failed_total, notifications_retried_total, notification_processing_duration_seconds, notification_queue_depth, scheduled_jobs_executed_total, scheduled_jobs_failed_total |

---

## Non-Functional Requirements

| ID | Requirement |
|---|---|
| NFR-01 | At-least-once delivery guarantee for all accepted notifications |
| NFR-02 | Worker pool must support concurrent processing (configurable worker count) |
| NFR-03 | Idempotent notification processing — duplicate requests with the same idempotency_key must not result in duplicate deliveries |
| NFR-04 | Horizontal scaling — stateless HTTP tier; workers compete on DB row locking |
| NFR-05 | Graceful shutdown — drain in-flight notifications before exit |
| NFR-06 | API authentication via RS256 JWT and API key (same as existing services) |
| NFR-07 | Sensitive provider credentials must not be logged or exposed via API |
| NFR-08 | All database operations must be traced via OTEL |
| NFR-09 | Service must start within 15 seconds in a healthy environment |

---

## Constraints

| ID | Constraint |
|---|---|
| C-01 | Language: Go 1.26; router: chi; no Gin/Echo/Fiber |
| C-02 | Database: Postgres via GORM; migrations via golang-migrate |
| C-03 | Messaging: NATS JetStream for async input |
| C-04 | Observability: OTEL SDK (traces → Jaeger, metrics → Prometheus) |
| C-05 | Auth: RS256 JWT (public key only — no JWT issuance) |
| C-06 | Port 8084 (per platform port scheme) |
| C-07 | Must not import other services directly — communication via HTTP only |
| C-08 | pkg/ is the only shared module |

---

## Assumptions

| ID | Assumption |
|---|---|
| A-01 | Channel providers (email SMTP, SMS gateway, push FCM/APNs) are configured via environment variables; v1 implementations are stubs that log the send |
| A-02 | NATS JetStream is available and the NOTIFICATIONS stream will be created by this service |
| A-03 | Notification recipients are plain strings (email address, phone number, device token, URL) — no user preference lookup in v1 |
| A-04 | Template variables are JSON-serialisable key/value pairs |
| A-05 | cron expressions use standard 5-field format (min hour dom mon dow) |

---

## Acceptance Criteria

| ID | Criterion | Verifies |
|---|---|---|
| AC-01 | POST /v1/notifications returns 201 and a notification record with status=PENDING | FR-08, FR-16 |
| AC-02 | Worker transitions notification from PENDING → SENT within 30s under normal conditions | FR-10, NFR-01 |
| AC-03 | Duplicate request with same idempotency_key returns 200 with the existing record, no second delivery | FR-12, NFR-03 |
| AC-04 | Failed notification with retry_count < max_retries transitions to RETRYING | FR-11 |
| AC-05 | Failed notification with retry_count = max_retries transitions to FAILED (dead letter) | FR-13 |
| AC-06 | POST /v1/templates with HTML body can be rendered via POST /v1/templates/{id}/preview | FR-03, FR-05, FR-07 |
| AC-07 | GET /v1/notifications?status=SENT&channel=EMAIL returns paginated results | FR-18, FR-19 |
| AC-08 | Recurring schedule with cron_expr fires within 90s of next_run_at | FR-21 |
| AC-09 | DELETE /v1/notifications/{id}/cancel transitions PENDING → CANCELLED | FR-14 |
| AC-10 | /healthz/ready returns 503 if Postgres is unreachable | NFR-09 |
| AC-11 | /metrics exposes notifications_sent_total counter | FR-31 |
| AC-12 | NATS consumer receives a send request and creates a PENDING notification | FR-08 |

---

## Service Boundaries

### In scope
- Notification record lifecycle (PENDING → terminal state)
- Template management and rendering
- Schedule management and execution
- Channel dispatch (stub implementations in v1)
- Notification history query API
- Worker pool and scheduler loop
- NATS consumer for async notification requests

### Out of scope
- User notification preference management (future)
- Provider failover / multi-provider per channel (future)
- Rate limiting per recipient (future)
- Notification batching (future)
- Analytics dashboards (future)
- Multi-tenant configuration (future)
- Localisation / multi-language templates (future)
- Actual provider SDK integration (v1 stubs only — A-01)
