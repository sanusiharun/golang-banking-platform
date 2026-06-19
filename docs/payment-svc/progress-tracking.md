# Payment Service — Progress Tracking

## Legend

| Symbol | Meaning |
|---|---|
| ✅ | Complete |
| 🔄 | In progress |
| ⬜ | Not started |
| 🚫 | Blocked |
| 💳 | Technical debt |

---

## Epics

### E1 — Project Scaffold & Infrastructure

| ID | Status | Task | Notes | Satisfies |
|---|---|---|---|---|
| E1-T01 | ✅ | Create `payment-svc` module with `go.mod` | Follow existing service module pattern | C-01 |
| E1-T02 | ✅ | Set up directory structure per `architecture.md` package layout | `cmd/`, `internal/`, `migrations/` | C-01 |
| E1-T03 | ✅ | Wire `cmd/payment-svc/main.go` entry point | Config load, dependency injection, server start | C-01 |
| E1-T04 | ✅ | Add `config/config.go` | DB DSN, Redis addr, Account Service URL, NATS URL, ports, idempotency TTL | C-01 |
| E1-T05 | ✅ | Write `Dockerfile` for `payment-svc` | All service go.mod COPYs included; port 8085 | C-05 |
| E1-T06 | ✅ | Add `payment-svc` to `docker-compose.yml` | Port `8085`; `banking-net`; depends on account-svc + auth-svc | C-05, A-04 |
| E1-T07 | ⬜ | Provision `banking_payments` Postgres database | Run `datasource/postgres/06_setup_banking_payments.sql` as superuser | A-05 |
| E1-T08 | ✅ | Write `migrations/001_create_payments_tables.sql` | `transactions`, `reversals`, `idempotency_requests` DDL | A-05 |

---

### E2 — Domain Layer

| ID | Status | Task | Notes | Satisfies |
|---|---|---|---|---|
| E2-T01 | ✅ | Write `internal/domain/dao/transaction.go` | GORM model; BIGINT amount (minor units) | FR-01 – FR-05 |
| E2-T02 | ✅ | Write `internal/domain/dao/reversal.go` | GORM model for reversals | FR-10 |
| E2-T03 | ✅ | Write `internal/domain/repository/transaction.go` | `TransactionRepository` interface: Create, UpdateStatus, GetByID, ListByAccount, GetByIdempotencyKey, reversal methods | FR-06 – FR-09 |
| E2-T04 | ✅ | Write `internal/domain/dto/payment.go` | All request/response DTOs + constants | FR-01 – FR-09 |

---

### E3 — Infrastructure Layer

| ID | Status | Task | Notes | Satisfies |
|---|---|---|---|---|
| E3-T01 | ✅ | Write `internal/infra/postgres/transaction_repo.go` | `PostgresTransactionRepository` implementing `TransactionRepository`; full CRUD | FR-06 – FR-09, C-03 |
| E3-T02 | ✅ | Idempotency — Postgres fallback | Uses `pkg/idempotency.PostgresStore` (DualStore); no custom code needed | NFR-01, R-04 |
| E3-T03 | ✅ | Idempotency — Redis primary | Uses `pkg/idempotency.RedisStore` (Lua atomic SET NX); no custom code needed | NFR-01, R-03, AC-12 |
| E3-T04 | ✅ | Write `internal/infra/accountclient/client.go` | HTTP client using `pkg/httpclient`; GetAccount, GetBalance, Debit, Credit | FR-14 – FR-17, C-02, C-04 |
| E3-T05 | ✅ | Retry + exponential backoff on account client | `pkg/httpclient.DefaultConfig()` — retry × 3, backoff, jitter built in | NFR-02, R-01 |
| E3-T06 | ⬜ | Circuit breaker on account client | `pkg/httpclient` has retry but no circuit breaker; add separate CB wrapper | NFR-03, AC-08, R-01 |
| E3-T07 | ✅ | Write `internal/infra/eventpublisher/payment_publisher.go` | Typed event publisher wrapping `pkg/messaging.NATSPublisher` | FR-19, NFR-04, R-05 |

---

### E4 — Service Layer

| ID | Status | Task | Notes | Satisfies |
|---|---|---|---|---|
| E4-T01 | ✅ | Define `PaymentService` interface in `internal/service/payment_service.go` | Interface + stub implementation; GetByID and List work; write methods return errNotImplemented | FR-01 – FR-12 |
| E4-T02 | ⬜ | Write `internal/service/validation.go` | Shared validators: account existence, status, balance, limits, duplicate detection | FR-14 – FR-18 |
| E4-T03 | ⬜ | Write `internal/service/transfer.go` — BW-01 | Full transfer orchestration including idempotency check, state transitions, debit/credit | FR-01, AC-01, AC-10 |
| E4-T04 | ⬜ | Write partial failure compensation in `transfer.go` — BW-02 | Debit-success / credit-fail → compensating debit reversal | NFR-06, AC-07, R-02 |
| E4-T05 | ⬜ | Write `internal/service/reversal.go` — BW-03 | Validate eligibility, compensating credit/debit, state updates, event | FR-10, AC-03, AC-11 |
| E4-T06 | ⬜ | Write cancellation logic — BW-04 | State guard: only Pending; event on success | FR-11, AC-04 |
| E4-T07 | ⬜ | Write `internal/service/retry.go` — BW-05 | Recoverable failure check, retry count guard, re-enter from Processing | FR-12, AC-05, R-06 |
| E4-T08 | ⬜ | Implement merchant payment handler (FR-02) | Reuse transfer orchestration; product_type = MERCHANT_PAYMENT | FR-02 |
| E4-T09 | ⬜ | Implement fee charging handler (FR-03) | Service-scoped JWT required; product_type = FEE | FR-03 |
| E4-T10 | ⬜ | Implement refund handler (FR-04) | product_type = REFUND; validate original transaction reference | FR-04 |
| E4-T11 | ⬜ | Implement idempotency enforcement — BW-06 | Redis SET NX + DB fallback; cache final response after completion | NFR-01, AC-02, AC-12, R-03 |

---

### E5 — Transport Layer (HTTP)

| ID | Status | Task | Notes | Satisfies |
|---|---|---|---|---|
| E5-T01 | ✅ | Write `internal/transport/http/router.go` | All routes registered; pkg middleware stack applied | C-01, C-04 |
| E5-T02 | ✅ | Auth middleware | Uses `pkg/middleware.AuthenticateAny` (JWT + API key) | FR-01 – FR-12 |
| E5-T03 | ✅ | Tracing middleware | Uses `pkg/middleware.Tracing("payment-svc")` | NFR-09 |
| E5-T04 | ✅ | Metrics middleware | Uses `pkg/middleware.NewMetrics("payment_svc")` | NFR-10 |
| E5-T05 | ✅ | Write `internal/transport/http/payment_handler.go` | Transfer, merchant, fee, refund, reverse, cancel, retry; delegates to service | FR-01 – FR-04, FR-10 – FR-12, C-06 |
| E5-T06 | ✅ | Write `internal/transport/http/inquiry_handler.go` | GetByID, List; delegates to service | FR-06 – FR-09 |
| E5-T07 | ✅ | Lifecycle handlers (reverse, cancel, retry) | Already in payment_handler.go | FR-10 – FR-12 |
| E5-T08 | ✅ | Health check handler | DB + Redis + NATS liveness in container; `pkg/observability.HealthHandler` | NFR-11 |
| E5-T09 | ✅ | Prometheus `/metrics` endpoint | `pkg/middleware.PrometheusHandler()` mounted | NFR-10 |

---

### E6 — Async Transport (NATS Consumer)

| ID | Status | Task | Notes | Satisfies |
|---|---|---|---|---|
| E6-T01 | ✅ | Write `internal/transport/nats/consumer.go` | Queue group scaffold; logs + acks; message routing stubbed | FR-20, NFR-04 |
| E6-T02 | ⬜ | Implement scheduled payment message handler | Route to transfer orchestration with SCHEDULED type | FR-05 |
| E6-T03 | ⬜ | Implement DLQ consumer / inspector (stub) | Subscribe to DLQ topic; log and alert | NFR-04, R-05 |

---

### E7 — Scheduler (Distributed Lock)

| ID | Status | Task | Notes | Satisfies |
|---|---|---|---|---|
| E7-T01 | ⬜ | Implement Redis distributed lock for scheduled execution | `SET NX` with TTL per job type; release on completion | FR-05, NFR-13, R-07 |
| E7-T02 | ⬜ | Background reconciliation job — stale Processing detection | Detect transactions stuck in Processing beyond timeout; trigger recovery | R-02 |

---

### E8 — Observability

| ID | Status | Task | Notes | Satisfies |
|---|---|---|---|---|
| E8-T01 | ⬜ | Register all Prometheus metrics from `architecture.md` | All counters, histograms, gauges | NFR-10 |
| E8-T02 | ⬜ | Instrument Account Service client with metrics and spans | Duration histogram + call counter; OTEL span per call | NFR-09, NFR-10 |
| E8-T03 | ⬜ | Instrument NATS publisher with metrics | `payment_dlq_enqueued_total`, publish failure counter | NFR-10 |
| E8-T04 | ⬜ | Add `slog` structured logging to service layer | transaction_id, payment_type, status, trace_id, duration_ms on all significant operations | NFR-08 |

---

### E9 — Tests

| ID | Status | Task | Notes | Satisfies |
|---|---|---|---|---|
| E9-T01 | ⬜ | Unit tests for service layer (transfer, reversal, cancellation, retry) | Mock repository and account client | FR-01, FR-10 – FR-12 |
| E9-T02 | ⬜ | Unit tests for idempotency enforcement | Duplicate key → cached response; concurrent race test | NFR-01, AC-02, AC-12 |
| E9-T03 | ⬜ | Unit tests for partial failure compensation (BW-02) | Debit succeeds, credit fails → compensating debit | NFR-06, AC-07 |
| E9-T04 | ⬜ | Integration tests (`//go:build integration`) against real Postgres + Redis | Happy path transfer; duplicate idempotency key; reversal | AC-01 – AC-12 |
| E9-T05 | ⬜ | Integration test: Account Service circuit breaker | Simulate Account Service 5xx beyond threshold → 503 returned | AC-08 |

---

## Dependency Graph

```
E1 (Scaffold)
    └── E2 (Domain)
            └── E3 (Infrastructure)
                    └── E4 (Service)
                            ├── E5 (HTTP Transport)
                            ├── E6 (NATS Consumer)
                            └── E7 (Scheduler)
                                    └── E8 (Observability) — can layer on E4-E7
                                            └── E9 (Tests) — runs after all layers complete
```

E8 (observability instrumentation) can be layered in alongside E4–E7 rather than waiting for all of them.

---

## Current Blockers

| Blocker | Affects | Owner | Resolution |
|---|---|---|---|
| No blockers at this stage | — | — | — |

---

## Technical Debt Register

| ID | Description | Severity | Linked task |
|---|---|---|---|
| TD-01 | DLQ consumer (E6-T03) is a stub — undeliverable events are logged but not actionable without an ops interface or reprocessing tool | Medium | E6-T03 |
| TD-02 | Reconciliation job (E7-T02) detects stale Processing records but recovery strategy (auto-retry vs. manual review) is not yet defined | High | E7-T02 |
| TD-03 | Scheduled payment execution (E7-T01) uses Redis distributed lock but does not yet handle lock expiry edge case where the holder dies mid-execution | Medium | E7-T01 |
| TD-04 | Circuit breaker on Account Service client (E3-T06) not yet implemented; `pkg/httpclient` handles retry but not open/half-open CB state machine | High | E3-T06 |
| TD-05 | `go.sum` not generated — run `cd services/payment-svc && go mod tidy` before first build | Low | E1-T01 |
