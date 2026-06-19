# context.md — notification-svc

> Translates requirements into domain understanding. Explains _why_ the service exists and _how_ it fits the ecosystem.

---

## Domain Overview

`notification-svc` is the centralised communication gateway for the banking platform. It owns the full lifecycle of outbound notifications — from acceptance of a send request, through template rendering and delivery, to status tracking and retry management. No other service delivers notifications directly to users or external systems; all communication is funnelled through this service.

---

## Business Context

Transactional notifications are a compliance and operational requirement for any banking platform: OTP delivery, transaction confirmations, account alerts, statement reminders, and fraud signals all depend on timely and reliable delivery. Centralising this concern into a dedicated microservice achieves three things:

1. **Decoupling** — upstream services (auth-svc, account-svc) fire-and-forget a notification event; they are not blocked by delivery latency.
2. **Standardisation** — templating ensures consistent wording and formatting across all channels and calling services.
3. **Observability** — a single service provides a single place to monitor delivery rates, failures, and retry queues.

---

## Service Responsibilities

| Capability | Description |
|---|---|
| Notification ingestion | Accept notification requests via REST API and NATS JetStream |
| Idempotency | Deduplicate requests using caller-supplied keys |
| Template management | CRUD for notification templates; variable rendering; preview |
| Schedule management | One-time and cron-based recurring schedules |
| Async dispatch | Worker pool polls DB, renders templates, calls channel providers |
| Retry management | Configurable max-retries per notification; exponential-style re-queuing |
| Status lifecycle | Track each notification through PENDING → PROCESSING → SENT / DELIVERED / FAILED / CANCELLED |
| Observability | Prometheus metrics, OTel traces, structured slog logging |
| Auth enforcement | JWT + API key authentication; ADMIN / TELLER RBAC |

---

## Bounded Context Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                         notification-svc                            │
│                                                                     │
│  ┌──────────────┐   ┌──────────────┐   ┌──────────────────────┐    │
│  │  REST API    │   │NATS Consumer │   │  Scheduler Worker    │    │
│  │ /v1/notify   │   │NOTIFICATIONS │   │  (60s tick)          │    │
│  │ /v1/template │   │  stream      │   │                      │    │
│  │ /v1/schedule │   └──────┬───────┘   └──────────┬───────────┘    │
│  └──────┬───────┘          │                      │                │
│         │                  ▼                      ▼                │
│         │         ┌─────────────────────────────────────────────┐  │
│         └────────▶│           PostgreSQL DB                     │  │
│                   │   notifications / templates / schedules     │  │
│                   └─────────────────────────────────────────────┘  │
│                                      ▲                              │
│                   ┌──────────────────┴──────────────────────────┐  │
│                   │         Dispatcher Worker Pool               │  │
│                   │  (5 goroutines, poll every 5s)               │  │
│                   └──────────────────┬──────────────────────────┘  │
│                                      │                              │
└──────────────────────────────────────┼──────────────────────────────┘
                                       │ channel.Send()
                          ┌────────────┼────────────────┐
                          ▼            ▼                 ▼
                      [EMAIL]        [SMS]         [WEBHOOK]
                      (stub)        (stub)     (production-ready)
                                           [PUSH] [WHATSAPP] (stubs)
```

---

## Actors

| Actor | Type | Role |
|---|---|---|
| ADMIN user | Human | Full read/write access to notifications, templates, and schedules |
| TELLER user | Human | Read access to notifications and templates; limited cancel action |
| auth-svc | Upstream service | Issues RS256 JWTs consumed by this service's auth middleware |
| account-svc | Upstream service | Sends notification events via NATS after account operations |
| Platform NATS | Infrastructure | JetStream stream `NOTIFICATIONS` transports async notification requests |
| Email provider | External system | Receives rendered email payloads (stub → Sendgrid / AWS SES) |
| SMS provider | External system | Receives rendered SMS payloads (stub → Twilio / Vonage) |
| Webhook target | External system | Receives HTTP POST payloads for WEBHOOK channel |
| Push provider | External system | FCM / APNs (stub) |
| WhatsApp provider | External system | WhatsApp Business API (stub) |
| Prometheus | Infrastructure | Scrapes `/metrics` for delivery counters, histograms, and gauges |
| Jaeger / OTEL Collector | Infrastructure | Receives traces via OTLP gRPC on port 4317 |

---

## Business Workflows

### BW-01 — Send a notification via REST (FR-01, FR-03, FR-07)

1. Caller (ADMIN or TELLER) sends `POST /v1/notifications` with JWT.
2. Auth middleware validates JWT, extracts role.
3. Handler validates request body (channel, recipient are required).
4. Service checks idempotency key — if key exists in DB, returns existing record.
5. Service creates a new Notification DAO with `status=PENDING`.
6. Repository inserts record to `notifications` table.
7. Handler returns 201 Created with `NotificationResponse`.
8. (Async) Dispatcher worker polls `notifications` table every 5 s, claims PENDING records with `SELECT … FOR UPDATE SKIP LOCKED`, sets status=PROCESSING.
9. Dispatcher resolves template (if `template_code` provided), renders body and subject.
10. Dispatcher calls `channel.Send()` for the appropriate provider.
11. On success: repository updates record to `status=SENT`, sets `sent_at`, records `provider_ref`.
12. On failure: increments `retry_count`. If `retry_count < max_retries`, sets `status=RETRYING`. If exhausted, sets `status=FAILED`.

### BW-02 — Send a notification via NATS (FR-01)

1. Upstream service publishes a message to `notifications.requests.*` on NATS JetStream.
2. NATS consumer goroutine pulls up to 20 messages per batch.
3. Consumer calls `NotificationService.Send()` — same flow as BW-01 from step 4.
4. On success: NATS message is Ack'd.
5. On error: NATS message is Nak'd (JetStream redelivers, up to `MaxDeliver=5`).

### BW-03 — Template rendering (FR-05)

1. Dispatcher fetches template by `template_code` via `TemplateRepository.GetByCode()` (active templates only).
2. Renders `body` using `text/template` or `html/template` (based on `format` field).
3. Renders `subject` if channel is EMAIL.
4. Resulting content is delivered to the channel provider.

### BW-04 — Scheduled notification firing (FR-12, FR-13)

1. Scheduler worker ticks every 60 s.
2. Calls `ScheduleRepository.ClaimDue()` — selects enabled schedules where `next_run_at <= now`.
3. For each due schedule: calls `NotificationService.Send()` with an idempotency key of `{schedule_id}:{minute}`.
4. After firing: calls `ScheduleRepository.UpdateAfterRun()`:
   - Recurring: computes and stores next `next_run_at` from cron expression.
   - One-time: sets `enabled=false`.

### BW-05 — Manual retry (FR-10)

1. ADMIN calls `POST /v1/notifications/{id}/retry`.
2. Service fetches notification — 404 if not found.
3. Rejects if `status=CANCELLED` (409 Conflict).
4. Resets `status=PENDING`, `retry_count=0`, clears `error_message`.
5. Dispatcher picks up the record on next poll.

### BW-06 — Template preview (FR-11)

1. ADMIN or TELLER calls `POST /v1/templates/{id}/preview` with a `variables` map.
2. Service fetches template, renders body and subject with supplied variables.
3. Returns `PreviewTemplateResponse` with rendered content — nothing is persisted.

---

## Upstream Systems

| System | Coupling | Purpose |
|---|---|---|
| `auth-svc` (indirect) | Loose — public key only | RS256 JWT public key used at startup to configure auth middleware |
| NATS JetStream | Tight | Async ingestion of notification requests from other services |
| PostgreSQL | Tight | Primary store for all notifications, templates, and schedules |

---

## Downstream Systems

| System | Coupling | Purpose |
|---|---|---|
| Email provider (stub → SES/Sendgrid) | Loose (provider abstraction) | Delivers EMAIL channel notifications |
| SMS provider (stub → Twilio) | Loose | Delivers SMS channel notifications |
| WEBHOOK target URLs | Dynamic | HTTP POST for WEBHOOK channel |
| FCM / APNs (stub) | Loose | Delivers PUSH notifications |
| WhatsApp Business API (stub) | Loose | Delivers WHATSAPP notifications |
| Prometheus | Pull | Scrapes `/metrics` |
| OTel Collector | Push | Receives traces over OTLP gRPC |

---

## Dependencies Map

| Dependency | Type | Purpose |
|---|---|---|
| `github.com/go-chi/chi/v5` | HTTP router | REST API routing |
| `gorm.io/gorm` + `gorm.io/driver/postgres` | ORM | Database access |
| `github.com/golang-migrate/migrate/v4` | Migration runner | Auto-migration on startup |
| `github.com/nats-io/nats.go` | NATS client | JetStream consumer |
| `github.com/robfig/cron/v3` | Cron parser | Parsing 5-field cron expressions |
| `github.com/google/uuid` | UUID generation | ID generation for all entities |
| `go.opentelemetry.io/*` | OTel SDK | Traces and log bridge |
| `github.com/prometheus/client_golang` | Prometheus client | Metrics exposition |
| `golang.org/x/time/rate` | Rate limiter | Token bucket for API rate limiting |
| `github.com/sanusi/banking/pkg` | Shared module | `httpx`, `errors`, `middleware`, `auth`, `audit` |

---

## Risks

| ID | Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| `R-01` | Channel provider stubs ship to production | Medium | High — notifications silently succeed without real delivery | Replace stubs before production cut-over (see TD-01–TD-04 in progress-tracking.md) |
| `R-02` | Scheduler is not distributed — multiple replicas cause duplicate schedule firings | Low (single instance per cluster assumed) | Medium — duplicate notifications sent | Add distributed lock (Redis SETNX or Postgres advisory lock) before horizontal scaling |
| `R-03` | Template injection — malicious Go template body executes arbitrary logic | Low | High | Validate templates at create time; sandbox template execution with a call timeout |
| `R-04` | NATS disconnection causes notification loss | Low | High | JetStream provides persistence; durable consumer with `MaxDeliver=5`; monitor NATS reconnect events |
| `R-05` | Unbound retry loop increases DB size and dispatcher load under sustained provider failure | Medium | Medium | Max retries capped at 10; monitor `notification_failed_total` and alert on spike |
| `R-06` | Idempotency key not supplied by caller → duplicate notifications | Medium | Medium | Document the contract; consider enforcing idempotency_key as required for certain event types |

---

## Assumptions Revisited

| ID | Assumption | Implementation Impact |
|---|---|---|
| `A-01` | Callers include valid RS256 JWTs | Auth middleware uses static RSA public key from env; no discovery or JWKS endpoint needed |
| `A-02` | NATS stream pre-exists | Container startup creates stream if absent using `AddStream` with no-error-if-exists semantics |
| `A-03` | Channel delivery latency is out-of-SLA | Dispatcher is async; API only measures acceptance time |
| `A-04` | Single scheduler instance per replica | `worker.Scheduler` runs as a single goroutine; safe at current scale |
| `A-05` | Go standard template engine is sufficient | `text/template` and `html/template` are used; no sandboxing currently implemented (see R-03) |
| `A-06` | Idempotency is caller-enforced | Service deduplicates by DB partial unique index on `idempotency_key` |
