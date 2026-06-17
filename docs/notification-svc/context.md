# notification-svc — Context

## Domain Overview

`notification-svc` is the communications platform for the banking system. It accepts notification requests from any internal service, renders them through a template engine, and dispatches them via the appropriate channel provider. It persists a complete delivery history for auditing and supports scheduled and recurring notifications.

---

## Business Context

Business services (account-svc, auth-svc, future payment-svc) emit operational events that require customer communication — account created, transaction confirmed, OTP delivered, fraud alert. Embedding delivery logic in each service would scatter credential management, retry logic, and audit trails across the codebase. `notification-svc` consolidates this responsibility, satisfying **BO-01** and the audit requirement from **BO-02**.

---

## Service Responsibilities

| Capability | Description |
|---|---|
| Notification ingestion | Accept notification requests via HTTP POST or NATS message |
| Template rendering | Render HTML/text templates with variable substitution |
| Channel dispatch | Route rendered content to the correct channel provider |
| Retry management | Re-attempt failed deliveries with exponential back-off |
| Idempotency | Reject or return existing record for duplicate idempotency keys |
| Schedule management | Create one-time and recurring notification schedules |
| History query | Expose filtered, paginated notification history |
| Observability | Metrics, traces, structured logs |

---

## Bounded Context Diagram

```
┌────────────────────────────────────────────────────────────────┐
│                      notification-svc                          │
│                                                                │
│  ┌────────────┐   ┌────────────────┐   ┌──────────────────┐  │
│  │  HTTP API  │   │  NATS Consumer │   │ Scheduler Worker │  │
│  └─────┬──────┘   └───────┬────────┘   └────────┬─────────┘  │
│        │                  │                      │             │
│        └──────────────────▼──────────────────────▼            │
│                   ┌───────────────────┐                        │
│                   │ NotificationService│                       │
│                   └────────┬──────────┘                        │
│                            │                                   │
│                   ┌────────▼──────────┐                        │
│                   │  Dispatcher Worker│ (goroutine pool)       │
│                   └────────┬──────────┘                        │
│                            │                                   │
│              ┌─────────────▼──────────────────┐               │
│              │       Channel Registry          │               │
│              │  Email │ SMS │ Push │ WA │ Hook │               │
│              └────────────────────────────────┘               │
│                                                                │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │           Postgres (banking_notifications)                │ │
│  │  notifications │ templates │ schedules                    │ │
│  └──────────────────────────────────────────────────────────┘ │
└────────────────────────────────────────────────────────────────┘
         ▲                          ▲
         │ HTTP                     │ NATS
    ┌────┴─────┐              ┌─────┴──────┐
    │ Internal │              │  Internal  │
    │  callers │              │  services  │
    └──────────┘              └────────────┘
```

---

## Actors

| Actor | Type | Interaction |
|---|---|---|
| Internal services (account-svc, auth-svc, etc.) | Machine | HTTP POST or NATS publish to request notifications |
| Admin users | Human | HTTP API to manage templates, schedules, view history |
| Teller users | Human | HTTP API to send ad-hoc notifications |
| Dispatcher worker | Internal | Background goroutine pool; claims and delivers PENDING notifications |
| Scheduler worker | Internal | Background goroutine; creates notification records for due schedules |
| NATS consumer | Internal | Subscribes to NOTIFICATIONS stream; creates notification records |

---

## Business Workflows

### W-01: Immediate Notification via HTTP (FR-08, FR-09, FR-10)
1. Caller POSTs to `POST /v1/notifications` with channel, recipient, template_code (or body), vars, optional idempotency_key.
2. Handler validates request; checks idempotency key uniqueness.
3. Service creates a notification record with status=PENDING, scheduled_at=NULL.
4. Handler returns 201 with the new notification record.
5. Dispatcher worker polls Postgres for PENDING notifications, claims a batch, processes each:
   a. Fetch template by code, render body with vars.
   b. Call channel provider's Send().
   c. On success → status=SENT, sent_at=now.
   d. On transient failure → retry_count++; if < max_retries → status=RETRYING; else → status=FAILED.

### W-02: Asynchronous Notification via NATS (FR-08)
1. Internal service publishes a `SendNotificationRequest` JSON to `notifications.requests.{channel}`.
2. NATS consumer receives the message.
3. Consumer unmarshals the request and calls NotificationService.Send().
4. ACKs the message. Processing follows W-01 from step 5 onwards.

### W-03: Scheduled Notification (FR-09, FR-20)
1. Caller POSTs to `POST /v1/notifications` with scheduled_at in the future.
2. Service creates notification record with status=PENDING, scheduled_at set.
3. Dispatcher worker skips records where scheduled_at > NOW().
4. At scheduled_at, dispatcher picks up the record and delivers it.

### W-04: Recurring Schedule (FR-21, FR-22, FR-23, FR-24)
1. Caller POSTs to `POST /v1/schedules` with cron_expr, channel, template_code, recipient, vars.
2. Service creates a schedule record with recurring=true, next_run_at computed from cron_expr.
3. Scheduler worker ticks every minute; claims enabled schedules with next_run_at <= NOW().
4. For each due schedule: create a PENDING notification record, update last_run_at=NOW(), compute next_run_at from cron_expr.
5. Dispatcher delivers the notification as per W-01.

### W-05: Template Management (FR-03–FR-07)
1. Admin POSTs to `POST /v1/templates` with code, name, channel, format, body, variables.
2. Service creates template record with version=1.
3. On PUT /v1/templates/{id}, version is incremented.
4. Preview via POST /v1/templates/{id}/preview — renders the template with supplied vars, returns rendered string without sending.

### W-06: Retry Failed Notification (FR-15)
1. Admin POSTs to `POST /v1/notifications/{id}/retry`.
2. Service resets status=PENDING, retry_count=0 (or retains count).
3. Dispatcher picks it up and re-attempts delivery.

---

## Upstream Systems

| System | What We Read / Call | Coupling |
|---|---|---|
| auth-svc | RS256 public key (env var); API key validation via HTTP (optional) | Loose — public key cached at startup |
| NATS JetStream | Notification request messages on NOTIFICATIONS stream | Async — consumer, no tight coupling |
| Postgres | All persistent state | Tight — stateless service |

---

## Downstream Systems

| System | What We Deliver To | Coupling |
|---|---|---|
| Email provider (SMTP) | Rendered email notifications | Loose — channel provider stub |
| SMS gateway | SMS text messages | Loose — channel provider stub |
| Push provider (FCM/APNs) | Push notification payloads | Loose — channel provider stub |
| WhatsApp Business API | WhatsApp messages | Loose — channel provider stub |
| Webhook targets | HTTP POST to caller-supplied URL | Loose — webhook channel |

---

## Dependencies Map

| Package | Purpose |
|---|---|
| `github.com/sanusi/banking/pkg` | Shared: httpx, middleware, errors, observability, audit, database, messaging |
| `github.com/go-chi/chi/v5` | HTTP routing |
| `github.com/go-playground/validator/v10` | Request validation |
| `github.com/google/uuid` | ID generation |
| `github.com/nats-io/nats.go` | NATS JetStream consumer |
| `github.com/robfig/cron/v3` | Cron expression parsing for next_run_at computation |
| `gorm.io/gorm` + `gorm.io/driver/postgres` | ORM |
| `github.com/golang-migrate/migrate/v4` | SQL migrations |
| `go.opentelemetry.io/otel` | Distributed tracing + metrics |
| `github.com/prometheus/client_golang` | Custom metrics registration |

---

## Risks

| ID | Risk | Mitigation |
|---|---|---|
| R-01 | Worker crashes mid-delivery: notification stuck in PROCESSING | Worker acquires PROCESSING with a processing_deadline; scheduler resets stale PROCESSING rows (future: TD-01) |
| R-02 | Channel provider is unavailable | Retry with exponential back-off; move to FAILED after max_retries |
| R-03 | Duplicate notifications if idempotency key not supplied | Idempotency key is optional; callers are encouraged to supply it; without it, duplicates are possible |
| R-04 | Template injection / XSS via template body | `html/template` auto-escapes HTML; plain-text templates have no markup |
| R-05 | Cron expression misconfiguration causes schedule storm | Scheduler claims due schedules one at a time; max worker concurrency limits blast radius |
| R-06 | Sensitive recipient data in logs | Log recipient only at DEBUG level; production log level is INFO |

---

## Assumptions Revisited

| ID | Implication |
|---|---|
| A-01 | Channel providers are stubs in v1: they log the Send() call and return success. Real SDK integration is a future milestone. |
| A-02 | Container.build() calls `pkgmessaging` / NATS stream creation; idempotent. |
| A-03 | Recipient is a plain string — the caller is responsible for resolving user IDs to addresses. |
| A-04 | Template vars are `map[string]any`; serialised as JSONB in Postgres. |
| A-05 | robfig/cron v3 standard parser (5-field) is used. Seconds-precision cron is not supported in v1. |
