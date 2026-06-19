# review.md — notification-svc

> Post-implementation assessment against goals.md and architecture.md. Written by reverse-engineering the delivered code.

---

## Requirement Compliance

### Functional Requirements

| ID | Requirement | Status | Evidence |
|---|---|---|---|
| FR-01 | Accept via REST and NATS | ✅ Pass | `transport/router.go` + `worker/nats_consumer.go` |
| FR-02 | 5 channels: EMAIL, SMS, PUSH, WHATSAPP, WEBHOOK | ⚠️ Partial | WEBHOOK is production-ready; EMAIL/SMS/PUSH/WHATSAPP are stubs (TD-01–TD-04) |
| FR-03 | Persist every notification + status lifecycle | ✅ Pass | `notifications` table; status enum enforced in domain |
| FR-04 | Idempotency via `idempotency_key` | ✅ Pass | Partial unique index + service-level dedup before insert |
| FR-05 | Template-based notifications | ✅ Pass | `TemplateService.GetByCode` + `template.Renderer` called in dispatcher |
| FR-06 | Direct (raw payload) notifications | ✅ Pass | `payload` JSONB field; used when `template_code` is absent |
| FR-07 | Async dispatch via worker pool | ✅ Pass | `worker/dispatcher.go`; handlers return 201 before delivery |
| FR-08 | Configurable retry (0–10, default 3) | ✅ Pass | `max_retries` field; FAILED after exhaustion; RETRYING between attempts |
| FR-09 | Manual cancel (PENDING/RETRYING only) | ✅ Pass | `POST /v1/notifications/{id}/cancel`; 409 on invalid state |
| FR-10 | Manual retry (FAILED) | ✅ Pass | `POST /v1/notifications/{id}/retry`; resets retry_count=0 |
| FR-11 | Template CRUD + preview | ✅ Pass | 6-action API; soft-delete; version increment; preview renders without persist |
| FR-12 | Scheduling (one-time + recurring cron) | ✅ Pass | `worker/scheduler.go`; cron parsed with `robfig/cron`; next_run_at computed |
| FR-13 | Schedule CRUD + enable/disable | ✅ Pass | 7-action API; `SetEnabled` is atomic DB update |
| FR-14 | Paginated filterable notification list | ✅ Pass | Filters: status, channel, recipient, template_code, schedule_id, from/to; max 100/page |
| FR-15 | Liveness + readiness health checks | ✅ Pass | `/healthz/live` (always 200); `/healthz/ready` checks Postgres + NATS |
| FR-16 | RS256 JWT + API key auth | ✅ Pass | `AuthenticateAny` middleware; static RSA public key |
| FR-17 | RBAC: ADMIN + TELLER roles | ✅ Pass | Role-checked per route; TELLER blocked from schedule and admin-only endpoints |

### Non-Functional Requirements

| ID | Requirement | Status | Evidence / Note |
|---|---|---|---|
| NFR-01 | Dispatcher ≥ 500 notifications/min | ⬜ Unverified | Worker pool default 5 goroutines, batch 10; no load test recorded |
| NFR-02 | Write latency p99 ≤ 200 ms | ⬜ Unverified | Async design makes this likely achievable; no k6 test recorded |
| NFR-03 | Read latency p99 ≤ 300 ms | ⬜ Unverified | Indexes in place; no benchmark recorded |
| NFR-04 | ≥ 99.5% uptime | ⬜ Unverified | Graceful shutdown implemented; no production data |
| NFR-05 | Concurrent dispatcher safety | ✅ Pass | `SELECT … FOR UPDATE SKIP LOCKED` in `ClaimPending` and `ClaimDue` |
| NFR-06 | Observability | ✅ Pass | 7 Prometheus metrics + OTel traces + structured slog |
| NFR-07 | No secrets in logs / responses | ✅ Pass | slog fields do not include key material; no password in responses |
| NFR-08 | Graceful shutdown ≤ 30s | ✅ Pass | `SHUTDOWN_TIMEOUT=30s`; workers drained on OS signal |
| NFR-09 | Rate limiting | ✅ Pass | Token bucket, default 1000 RPS / burst 2000, on all protected routes |

---

## Architecture Compliance

| Decision | Compliant | Finding |
|---|---|---|
| chi + stdlib only | ✅ Compliant | No Gin/Echo/Fiber |
| Layered architecture (transport → service → repo → DAO) | ✅ Compliant | Clean separation across packages |
| `pkg/httpx` for all HTTP responses | ✅ Compliant | No local response.go; all handlers use `httpx.*` |
| `slog` for all logging | ✅ Compliant | No `fmt.Println` in source |
| Domain errors from `pkg/errors` | ✅ Compliant | `errors.NewNotFound`, `NewConflict` used correctly |
| No cross-service DB access | ✅ Compliant | Owns only `banking_notifications` |
| NATS for async inter-service | ✅ Compliant | Pull consumer on `NOTIFICATIONS` stream |
| `SELECT … FOR UPDATE SKIP LOCKED` for dispatcher | ✅ Compliant | Implemented in both `ClaimPending` and `ClaimDue` |
| Workers call service layer, not repo directly | ✅ Compliant | Dispatcher calls `NotificationService`, Scheduler calls `NotificationService` |
| No audit event publishing (out-of-scope currently) | ⚠️ Gap | NATS AUDIT stream ensured but never published to (TD-08) |
| Scheduler not distributed | ⚠️ Gap | Single goroutine per replica; safe at 1 replica (A-04, R-02) |

---

## Code Quality

### Strengths

| Dimension | Finding |
|---|---|
| Handler thinness | Handlers are thin and delegate to services correctly; no business logic in transport layer |
| Status machine | Notification status transitions are enforced at service layer, not handler layer |
| Idempotency | Implemented correctly at service layer with DB-level enforcement via partial unique index |
| Concurrency | `SKIP LOCKED` is the correct approach; no global mutexes; worker pool is configurable |
| Template rendering | Correctly distinguishes `text/template` vs `html/template` by format field |
| Config validation | Required env vars validated at startup with clear error messages |
| Graceful shutdown | OS signal handler drains workers before exit |

### Issues

| Severity | Location | Finding | Ref |
|---|---|---|---|
| **Critical** | `internal/channel/email/` | EMAIL stub silently succeeds — callers believe emails are delivered | TD-01 |
| **Critical** | `internal/channel/sms/`, `push/`, `whatsapp/` | SMS/PUSH/WHATSAPP stubs — no real delivery | TD-02–TD-04 |
| **Medium** | `internal/worker/dispatcher.go` | No exponential backoff between retries — flat poll interval can cause retry thundering herd | TD-05 |
| **Medium** | `internal/worker/scheduler.go` | Single goroutine per replica — duplicate fires if more than 1 replica deployed | TD-06 |
| **Medium** | `internal/template/renderer.go` | No timeout on template execution — malformed template can block dispatcher goroutine indefinitely | TD-07 |
| **Low** | `internal/repository/notification_repository.go` | `_ = json.Unmarshal(n.Payload, …)` silently ignores JSON errors in payload deserialization | — |
| **Low** | `internal/services/template_service.go` | `PUT /v1/templates/{id}` has no optimistic locking — concurrent updates last-write-wins | TD-09 |
| **Low** | `internal/worker/nats_consumer.go:87` | `c.nc.Drain()` errors suppressed on shutdown — intentional, but undocumented | — |
| **Low** | Missing | No audit events published to NATS `AUDIT` stream | TD-08 |

---

## Maintainability

| Dimension | Rating | Finding |
|---|---|---|
| Naming | ✅ Good | Consistent `Postgres{Noun}Repository`, `New{Type}` constructors, snake_case JSON |
| Test coverage | ⬜ Unknown | No test files found in service packages; integration test tag not observed |
| Error patterns | ✅ Good | Domain errors from `pkg/errors`; `fmt.Errorf("package.Function: %w", err)` wrapping |
| Comment density | ✅ Good | Minimal comments; code is self-documenting |
| Layer separation | ✅ Good | No business logic in handlers; no SQL in services |
| Interface usage | ✅ Good | All repositories and services are interfaces; providers implement `Channel` interface |

---

## Operational Readiness

| Item | Status | Finding |
|---|---|---|
| Health checks | ✅ Ready | Live + ready probes implemented |
| Prometheus metrics | ✅ Ready | 7 custom metrics + middleware HTTP metrics |
| Distributed traces | ✅ Ready | OTel SDK wired; opt-in via env |
| Structured logging | ✅ Ready | JSON slog with context propagation |
| Alert rules | ⬜ Not done | No Prometheus alert rules for notification_failed_total spike (TD-10) |
| Runbook | ⬜ Not done | No operational runbook documented |
| Graceful shutdown | ✅ Ready | 30s drain window |
| DB migrations | ✅ Ready | Auto-run on startup via golang-migrate |
| Docker image | ✅ Ready | Multi-stage Alpine build, CGO disabled |

---

## Security Posture

| Control | Status | Finding |
|---|---|---|
| JWT RS256 authentication | ✅ Pass | Public key injected at startup; no runtime auth-svc calls |
| API key alternative | ✅ Pass | Via `pkg/auth` |
| RBAC (ADMIN / TELLER) | ✅ Pass | Route-level enforcement |
| Secret masking in logs | ✅ Pass | No key material logged |
| Rate limiting | ✅ Pass | 1000 RPS default, configurable |
| SQL injection | ✅ Pass | GORM parameterised queries throughout |
| Template injection | ⚠️ Partial | `html/template` auto-escapes HTML; no execution timeout (TD-07) |
| HTTPS / TLS termination | ⬜ Platform | TLS terminated at Cloud API Gateway / reverse proxy; not within service |
| Credential in env vars | ✅ Pass | DB passwords and JWT keys via env; `.env` not committed |

---

## Reliability Assessment

| Failure Scenario | Behaviour | Acceptable? |
|---|---|---|
| PostgreSQL down at startup | Fatal start failure | ✅ Yes — readiness probe blocks traffic |
| PostgreSQL down at runtime | Dispatcher stalls; API returns 500 | ✅ Yes — auto-recovers when DB restored |
| NATS down at startup | Service starts; consumer not running; log warning | ⚠️ Partial — messages buffered in JetStream |
| NATS down at runtime | Consumer stops; messages redelivered when reconnected | ✅ Yes — durable consumer handles this |
| Channel provider timeout (WEBHOOK) | 10s timeout; error returned; retry logic triggered | ✅ Yes |
| All retries exhausted | Status=FAILED; metric incremented; no silent loss | ✅ Yes |
| Duplicate dispatcher replicas | `SKIP LOCKED` prevents double-processing | ✅ Yes |
| Duplicate scheduler replicas | Both will fire same schedules | ⚠️ No — see TD-06 |
| Template rendering panic | Not guarded — could kill dispatcher goroutine | ⚠️ No — see TD-07 |

---

## Technical Debt Summary

| ID | Description | Severity | Status |
|---|---|---|---|
| TD-01 | EMAIL stub — no real delivery | Critical | Open |
| TD-02 | SMS stub — no real delivery | Critical | Open |
| TD-03 | PUSH stub — no FCM/APNs | High | Open |
| TD-04 | WHATSAPP stub — no Business API | High | Open |
| TD-05 | Flat retry interval — no backoff | Medium | Open |
| TD-06 | Scheduler not distributed — safe at 1 replica only | Medium | Open |
| TD-07 | No template execution timeout | Medium | Open |
| TD-08 | No audit event publishing | Low | Open |
| TD-09 | Template updates last-write-wins | Low | Open |
| TD-10 | No Prometheus alert rules | Low | Open |

---

## Risks (Updated from context.md)

| ID | Risk | Mitigation Status |
|---|---|---|
| R-01 | Stubs ship to production | ⚠️ Open — must replace before production cut-over |
| R-02 | Duplicate scheduler firings at scale | ⚠️ Open — acceptable at 1 replica; needs lock before horizontal scale |
| R-03 | Template injection / hang | ⚠️ Partial — html/template auto-escapes; no execution timeout |
| R-04 | NATS disconnection / message loss | ✅ Mitigated — durable consumer + MaxDeliver=5 |
| R-05 | Thundering herd on provider failure | ⚠️ Open — no backoff (TD-05) |
| R-06 | Missing idempotency_key | ⚠️ Accepted — caller responsibility; documented in goals.md |

---

## Recommendations

### Immediate (before production)

1. **Replace EMAIL, SMS, PUSH, WHATSAPP stubs** (TD-01–TD-04) with real SDK integrations. WEBHOOK is the only production-safe channel today.
2. **Add template execution timeout** (`context.WithTimeout`, ~5s) inside the template renderer to prevent dispatcher goroutine hang (TD-07, R-03).
3. **Write integration tests** — no test files found; at minimum cover `NotificationService.Send`, idempotency path, retry exhaustion, and cancel/retry state guards.

### Short-term (next sprint)

4. **Exponential backoff with jitter** in dispatcher retry logic (TD-05) — prevents thundering herd under sustained provider failure.
5. **Prometheus alert rule** for `notification_failed_total` spike > threshold within 5-minute window (TD-10).
6. **Publish audit events** to NATS `AUDIT` stream on notification and template mutations (TD-08) — required for audit-svc integration.

### Medium-term

7. **Distributed scheduler lock** (Redis SETNX or Postgres advisory lock) before deploying more than 1 replica (TD-06, R-02).
8. **Optimistic locking on template update** — add `version` to `PUT` request and reject stale updates with 409 Conflict (TD-09).
9. **Operational runbook** — document how to: restart stuck dispatcher, force-retry a batch of FAILED notifications, roll back a bad template version.

---

## Refactoring Opportunities

| Area | Suggestion | Benefit |
|---|---|---|
| `dispatcher.go` retry path | Extract `processNotification()` into its own function with a `context.WithTimeout` wrapper | Testability + timeout safety |
| Channel stubs | Replace `uuid.New().String()` stub responses with a `NoOpChannel` that logs intent — makes stub status explicit | Clarity + avoids false-positive metrics |
| `nats_consumer.go` | Add a dead-letter queue or error counter when `MaxDeliver` is exceeded — currently silent | Operational visibility |
| `schedule_service.go` | Move `ComputeNextRun` to a separate `cron` package — it is tested independently and reused by the worker | Single responsibility |
