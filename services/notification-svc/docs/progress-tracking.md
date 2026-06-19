# progress-tracking.md — notification-svc

> Tracks implementation status. Updated after every work session. Stale tracking is worse than no tracking.

---

## Legend

| Symbol | Meaning |
|---|---|
| ✅ | Complete |
| 🔄 | In progress |
| ⬜ | Not started |
| 🚫 | Blocked |
| 💸 | Technical debt |

---

## Epics

### E1 — Core HTTP API & Transport

| Task | ID | Status | Notes | Location |
|---|---|---|---|---|
| chi router setup with global middleware | E1-T01 | ✅ | RealIP, RequestID, Logger, Tracing, Metrics, Recovery | `internal/transport/router.go` |
| JWT + API key auth middleware wired | E1-T02 | ✅ | `AuthenticateAny`, role extraction | `internal/transport/router.go` |
| Rate limiting middleware | E1-T03 | ✅ | Token bucket, configurable RPS/burst | `internal/transport/router.go` |
| Per-handler timeout middleware | E1-T04 | ✅ | Default 25s via `HANDLER_TIMEOUT_SECS` | `internal/transport/router.go` |
| NotificationHandler (Send, List, GetByID, Retry, Cancel) | E1-T05 | ✅ | All five actions implemented | `internal/transport/notification_handler.go` |
| TemplateHandler (Create, List, GetByID, Update, Delete, Preview) | E1-T06 | ✅ | Soft delete via active=false | `internal/transport/template_handler.go` |
| ScheduleHandler (Create, List, GetByID, Update, Delete, Enable, Disable) | E1-T07 | ✅ | 7 actions, ADMIN-only | `internal/transport/schedule_handler.go` |
| Health handler (live + ready) | E1-T08 | ✅ | Checks Postgres + NATS | `internal/transport/health_handler.go` |
| Prometheus `/metrics` endpoint | E1-T09 | ✅ | Standard Prometheus handler | `internal/transport/router.go` |

**Satisfies:** FR-01, FR-09, FR-10, FR-11, FR-12, FR-13, FR-14, FR-15, FR-16, FR-17

---

### E2 — Domain & Repository Layer

| Task | ID | Status | Notes | Location |
|---|---|---|---|---|
| Notification DAO struct | E2-T01 | ✅ | All columns, JSONB fields | `internal/domain/notification.go` |
| Template DAO struct | E2-T02 | ✅ | Version, soft-delete active flag | `internal/domain/template.go` |
| Schedule DAO struct | E2-T03 | ✅ | Cron / one-time constraint | `internal/domain/schedule.go` |
| NotificationRepository (Create, GetByID, GetByIdempotencyKey, Update, UpdateStatus, List, ClaimPending) | E2-T04 | ✅ | SKIP LOCKED for concurrent safety | `internal/repository/notification_repository.go` |
| TemplateRepository (Create, GetByID, GetByCode, Update, SoftDelete, List) | E2-T05 | ✅ | active-only for GetByCode | `internal/repository/template_repository.go` |
| ScheduleRepository (Create, GetByID, Update, Delete, SetEnabled, List, ClaimDue, UpdateAfterRun) | E2-T06 | ✅ | SKIP LOCKED for ClaimDue | `internal/repository/schedule_repository.go` |
| DB migration files (3 tables) | E2-T07 | ✅ | Embedded via `migrations/` | `migrations/` |
| Auto-migration on startup | E2-T08 | ✅ | golang-migrate with embedded FS | `internal/container.go` |

**Satisfies:** FR-03, FR-04, FR-05, FR-11, FR-12, NFR-05

---

### E3 — Service Layer (Business Logic)

| Task | ID | Status | Notes | Location |
|---|---|---|---|---|
| NotificationService: Send with idempotency | E3-T01 | ✅ | Dedup by key before insert | `internal/services/notification_service.go` |
| NotificationService: Retry (reset status) | E3-T02 | ✅ | Rejects CANCELLED; resets retry_count | `internal/services/notification_service.go` |
| NotificationService: Cancel (PENDING/RETRYING only) | E3-T03 | ✅ | 409 on invalid state | `internal/services/notification_service.go` |
| NotificationService: GetByID + List with filters | E3-T04 | ✅ | Paginated, ordered by created_at DESC | `internal/services/notification_service.go` |
| TemplateService: CRUD + SoftDelete | E3-T05 | ✅ | Version auto-incremented on update | `internal/services/template_service.go` |
| TemplateService: Preview (render without persist) | E3-T06 | ✅ | Uses template renderer | `internal/services/template_service.go` |
| ScheduleService: Create with cron parse + next_run_at | E3-T07 | ✅ | 422 on invalid cron | `internal/services/schedule_service.go` |
| ScheduleService: Enable/Disable | E3-T08 | ✅ | Atomic DB flag | `internal/services/schedule_service.go` |
| ComputeNextRun helper | E3-T09 | ✅ | Exported, used by scheduler worker | `internal/services/schedule_service.go` |

**Satisfies:** FR-03, FR-04, FR-05, FR-06, FR-08, FR-09, FR-10, FR-11, FR-12, FR-13

---

### E4 — Channel Providers & Template Engine

| Task | ID | Status | Notes | Location |
|---|---|---|---|---|
| Channel interface + Registry | E4-T01 | ✅ | `Channel.Send()` contract | `internal/channel/channel.go` |
| WEBHOOK provider (production-ready) | E4-T02 | ✅ | HTTP POST, 10s timeout, error on 4xx/5xx | `internal/channel/webhook/` |
| EMAIL provider stub | E4-T03 | 💸 TD-01 | Returns UUID as provider_ref | `internal/channel/email/` |
| SMS provider stub | E4-T04 | 💸 TD-02 | Returns UUID as provider_ref | `internal/channel/sms/` |
| PUSH provider stub | E4-T05 | 💸 TD-03 | FCM/APNs TODO | `internal/channel/push/` |
| WHATSAPP provider stub | E4-T06 | 💸 TD-04 | WhatsApp Business API TODO | `internal/channel/whatsapp/` |
| Template renderer (text + html modes) | E4-T07 | ✅ | `text/template` and `html/template` | `internal/template/` |

**Satisfies:** FR-02, FR-05, FR-06, C-09

---

### E5 — Background Workers

| Task | ID | Status | Notes | Location |
|---|---|---|---|---|
| Dispatcher worker pool (configurable goroutines) | E5-T01 | ✅ | WORKER_COUNT goroutines, poll every WORKER_POLL_SECS | `internal/worker/dispatcher.go` |
| Dispatcher: ClaimPending + SKIP LOCKED | E5-T02 | ✅ | Batch claim with FOR UPDATE SKIP LOCKED | `internal/worker/dispatcher.go` |
| Dispatcher: template render → channel.Send → status update | E5-T03 | ✅ | Retry increment or FAILED on exhaustion | `internal/worker/dispatcher.go` |
| Dispatcher: metrics (sent, failed, retried, duration) | E5-T04 | ✅ | Prometheus counters/histograms | `internal/worker/dispatcher.go` |
| Scheduler worker (60s tick) | E5-T05 | ✅ | Single goroutine per replica | `internal/worker/scheduler.go` |
| Scheduler: ClaimDue + fire + UpdateAfterRun | E5-T06 | ✅ | One-time auto-disables; recurring recomputes | `internal/worker/scheduler.go` |
| Scheduler: idempotency key {schedule_id}:{minute} | E5-T07 | ✅ | Prevents duplicate firing within same minute | `internal/worker/scheduler.go` |
| NATS JetStream pull consumer | E5-T08 | ✅ | Durable, batch 20, MaxDeliver 5 | `internal/worker/nats_consumer.go` |
| Exponential backoff for dispatcher retries | E5-T09 | ⬜ TD-05 | Currently flat poll interval; thundering herd risk | `internal/worker/dispatcher.go` |
| Distributed scheduler lock | E5-T10 | ⬜ TD-06 | Required before horizontal scaling | `internal/worker/scheduler.go` |

**Satisfies:** FR-07, FR-08, NFR-01, NFR-05, C-04

---

### E6 — Observability

| Task | ID | Status | Notes | Location |
|---|---|---|---|---|
| Prometheus metrics registered | E6-T01 | ✅ | 7 custom metrics + middleware metrics | `internal/worker/dispatcher.go`, `scheduler.go` |
| OTel tracing bootstrapped (optional) | E6-T02 | ✅ | Disabled by default, opt-in via OTEL_ENABLED | `internal/container.go` |
| OTel log bridge (optional) | E6-T03 | ✅ | OTEL_LOGS_ENABLED flag | `internal/container.go` |
| ServiceTracer on all service operations | E6-T04 | ✅ | Span per service method | `internal/services/` |
| Request span via chi middleware | E6-T05 | ✅ | Via `pkg/middleware` | `internal/transport/router.go` |
| Queue depth gauge (approx PENDING count) | E6-T06 | ✅ | Polled in dispatcher | `internal/worker/dispatcher.go` |
| Alert rules for notification_failed_total | E6-T07 | ⬜ | Prometheus alert rule not yet written | `monitoring/` |

**Satisfies:** NFR-06

---

### E7 — Configuration & Startup

| Task | ID | Status | Notes | Location |
|---|---|---|---|---|
| Config struct with all env vars + defaults | E7-T01 | ✅ | Required: DB_HOST, DB_NAME, DB_USER, DB_PASSWORD, JWT_PUBLIC_KEY_B64 | `config/config.go` |
| .env file loading (optional) | E7-T02 | ✅ | Not required in production | `config/config.go` |
| DI container wiring all components | E7-T03 | ✅ | `container.go` constructs all deps | `internal/container.go` |
| Graceful shutdown (30s drain) | E7-T04 | ✅ | OS signal handler; drains workers | `cmd/server/main.go` |
| Dockerfile (Alpine, CGO disabled) | E7-T05 | ✅ | Multi-stage build | `Dockerfile` |
| docker-compose integration | E7-T06 | ✅ | Part of platform docker-compose.yml | `docker-compose.yml` |
| NATS stream ensure on startup | E7-T07 | ✅ | Creates stream if not exists | `internal/container.go` |

**Satisfies:** C-01, C-02, C-03, C-04, NFR-08

---

## Dependency Graph

```
E1 (HTTP API)
  └─ depends on E3 (Services)
       └─ depends on E2 (Repository)
            └─ depends on E7 (Config / Startup)

E5 (Workers)
  └─ depends on E3 (Services)
  └─ depends on E4 (Channels + Template Engine)
  └─ depends on E2 (Repository)

E6 (Observability)
  └─ depends on E7 (Startup — OTel bootstrap)
  └─ depends on E5 (worker metrics)
```

---

## Current Blockers

| What | Affects | Owner | Resolution |
|---|---|---|---|
| No active blockers | — | — | — |

---

## Technical Debt Register

| ID | Description | Severity | Linked Task |
|---|---|---|---|
| `TD-01` | EMAIL channel is a stub — returns mock provider_ref, no real delivery | **Critical** | E4-T03 |
| `TD-02` | SMS channel is a stub — no real delivery | **Critical** | E4-T04 |
| `TD-03` | PUSH channel is a stub — no FCM/APNs integration | **High** | E4-T05 |
| `TD-04` | WHATSAPP channel is a stub — no WhatsApp Business API integration | **High** | E4-T06 |
| `TD-05` | Dispatcher retry is flat-interval — no exponential backoff or jitter; thundering herd risk under provider failure | **Medium** | E5-T09 |
| `TD-06` | Scheduler is not distributed — multiple replicas will fire duplicate schedules | **Medium** | E5-T10 |
| `TD-07` | Template rendering has no execution timeout or CPU sandbox — malformed templates can hang dispatcher goroutine | **Medium** | — |
| `TD-08` | No audit event published to NATS `AUDIT` stream on notification CRUD — audit-svc receives no events from this service | **Low** | — |
| `TD-09` | Template version has no conflict detection (last-write-wins) — concurrent updates may silently overwrite each other | **Low** | — |
| `TD-10` | Alert rules for `notification_failed_total` spike not yet written in Prometheus config | **Low** | E6-T07 |
