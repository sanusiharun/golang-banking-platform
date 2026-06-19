# Payment Service — Review

> Status: Pre-implementation. All criteria are ⬜ Unverified. Update this document at each major milestone and after production deployment.

---

## Requirement Compliance

### Functional Requirements

| ID | Requirement (short) | Status | Evidence |
|---|---|---|---|
| FR-01 | Internal account transfer | ⬜ Unverified | — |
| FR-02 | Merchant payment | ⬜ Unverified | — |
| FR-03 | Fee charging | ⬜ Unverified | — |
| FR-04 | Refund processing | ⬜ Unverified | — |
| FR-05 | Scheduled payment execution | ⬜ Unverified | — |
| FR-06 | Retrieve transaction by ID | ⬜ Unverified | — |
| FR-07 | Retrieve transaction status | ⬜ Unverified | — |
| FR-08 | Retrieve transaction history | ⬜ Unverified | — |
| FR-09 | Expose failure reason | ⬜ Unverified | — |
| FR-10 | Transaction reversal | ⬜ Unverified | — |
| FR-11 | Transaction cancellation | ⬜ Unverified | — |
| FR-12 | Transaction retry | ⬜ Unverified | — |
| FR-13 | Idempotency key validation | ⬜ Unverified | — |
| FR-14 | Account existence validation | ⬜ Unverified | — |
| FR-15 | Account status validation | ⬜ Unverified | — |
| FR-16 | Balance sufficiency validation | ⬜ Unverified | — |
| FR-17 | Transaction limit validation | ⬜ Unverified | — |
| FR-18 | Duplicate transaction detection | ⬜ Unverified | — |
| FR-19 | Event publishing on state transitions | ⬜ Unverified | — |
| FR-20 | Asynchronous message-driven processing | ⬜ Unverified | — |

### Non-Functional Requirements

| ID | Requirement (short) | Status | Evidence |
|---|---|---|---|
| NFR-01 | Idempotency key enforcement + cached response | ⬜ Unverified | — |
| NFR-02 | Retry with exponential backoff | ⬜ Unverified | — |
| NFR-03 | Circuit breaker on Account Service | ⬜ Unverified | — |
| NFR-04 | Dead-letter queue for unprocessable messages | ⬜ Unverified | — |
| NFR-05 | Graceful request timeout handling | ⬜ Unverified | — |
| NFR-06 | Business-level atomicity; partial failure compensation | ⬜ Unverified | — |
| NFR-07 | Transaction traceability (ID, reference, correlation) | ⬜ Unverified | — |
| NFR-08 | Structured logging with trace context | ⬜ Unverified | — |
| NFR-09 | Distributed tracing (OTEL) | ⬜ Unverified | — |
| NFR-10 | Prometheus metrics (latency, counts, error rate) | ⬜ Unverified | — |
| NFR-11 | Health check endpoint | ⬜ Unverified | — |
| NFR-12 | Full audit record on every transaction activity | ⬜ Unverified | — |
| NFR-13 | Stateless; horizontally scalable | ⬜ Unverified | — |
| NFR-14 | Independent deployability | ⬜ Unverified | — |

---

## Architecture Compliance

| Decision | Status | Finding |
|---|---|---|
| Layered architecture (transport → service → repository → DAO) | ⬜ Unverified | — |
| Handlers are thin; no business logic in transport layer | ⬜ Unverified | — |
| All HTTP responses via `pkg/httpx` | ⬜ Unverified | — |
| No cross-service direct DB access | ⬜ Unverified | — |
| Balance mutations exclusively via Account Service API | ⬜ Unverified | — |
| Redis `SET NX` used for first-writer-wins idempotency | ⬜ Unverified | — |
| NATS event publishing is non-blocking (goroutine) | ⬜ Unverified | — |
| NATS consumer uses queue group subscription | ⬜ Unverified | — |
| Circuit breaker implemented on Account Service client | ⬜ Unverified | — |
| Distributed Redis lock guards scheduled payment execution | ⬜ Unverified | — |

---

## Code Quality

| Severity | Location | Finding | Reference |
|---|---|---|---|
| — | — | To be populated after implementation | — |

### Dimensions to assess

- Handler thinness — business logic must not appear in transport layer
- Error wrapping — `fmt.Errorf("component.Method: %w", err)` pattern
- Domain error usage — `pkg/errors.NewNotFound`, `NewConflict`, etc.
- No `fmt.Println` — `slog` only
- No local `response.go` — `pkg/httpx` only

---

## Maintainability

| Dimension | Status | Notes |
|---|---|---|
| Naming conventions (interfaces, implementations, constructors) | ⬜ Unverified | — |
| Test coverage — service layer unit tests | ⬜ Unverified | — |
| Test coverage — integration tests with real DB/Redis | ⬜ Unverified | — |
| Error pattern consistency | ⬜ Unverified | — |
| Comment density — only non-obvious WHY comments | ⬜ Unverified | — |
| Import order — stdlib → external → pkg → service-local | ⬜ Unverified | — |

---

## Operational Readiness

| Check | Status | Notes |
|---|---|---|
| `/health` endpoint reports DB, Redis, Account Service liveness | ⬜ Unverified | — |
| Prometheus metrics endpoint `/metrics` active | ⬜ Unverified | — |
| OTEL traces emitted for all critical paths | ⬜ Unverified | — |
| Structured JSON logs with trace_id on all operations | ⬜ Unverified | — |
| DLQ topic defined and monitored | ⬜ Unverified | — |
| Circuit breaker state observable via metric | ⬜ Unverified | — |
| Alerting rules defined for error rate and DLQ depth | ⬜ Unverified | — |
| Runbook exists for partial failure (R-02) recovery | ⬜ Unverified | — |
| Runbook exists for stale Processing transaction recovery | ⬜ Unverified | — |

---

## Security Posture

| Control | Status | Finding |
|---|---|---|
| RS256 JWT validated on all protected endpoints | ⬜ Unverified | — |
| `initiated_by` extracted from JWT `sub` claim | ⬜ Unverified | — |
| Fee charging restricted to service-scoped JWT | ⬜ Unverified | — |
| Health and metrics endpoints not exposed through API gateway | ⬜ Unverified | — |
| Request body size limit enforced | ⬜ Unverified | — |
| `metadata` field stored as JSONB (never interpolated into SQL) | ⬜ Unverified | — |
| Transaction IDs are UUIDs (no sequential enumeration) | ⬜ Unverified | — |
| Amount field never logged in plain text in production | ⬜ Unverified | — |

---

## Reliability Assessment

| Failure scenario | Expected behaviour | Status |
|---|---|---|
| Account Service returns 5xx | Retry × N → circuit breaker open → 503 to caller | ⬜ Unverified |
| Account Service circuit breaker open | Return 503 with `Retry-After`; no Account Service call | ⬜ Unverified |
| Debit succeeds, credit fails | Compensating debit reversal; transaction marked Failed | ⬜ Unverified |
| Redis unavailable | Reject inbound requests (fail-safe) | ⬜ Unverified |
| NATS unavailable | Transaction completes; event enqueued to DLQ after local retry | ⬜ Unverified |
| Duplicate idempotency key (concurrent) | Exactly one transaction record created | ⬜ Unverified |
| Scheduled payment fires on two instances simultaneously | Distributed lock ensures exactly-once execution | ⬜ Unverified |
| Reversal attempted twice | Second reversal rejected (unique constraint + state guard) | ⬜ Unverified |
| Transaction stuck in Processing | Background reconciliation detects and triggers recovery | ⬜ Unverified |

---

## Technical Debt Summary

| ID | Description | Severity | Status |
|---|---|---|---|
| TD-01 | DLQ consumer is a stub; no reprocessing interface | Medium | Open |
| TD-02 | Stale Processing recovery strategy undefined (auto vs. manual) | High | Open |
| TD-03 | Scheduler Redis lock does not handle holder-death edge case | Medium | Open |

---

## Risks (Updated from context.md)

| ID | Threat | Mitigation status |
|---|---|---|
| R-01 | Account Service unavailability | ⬜ Unverified — circuit breaker + retry not yet implemented |
| R-02 | Partial failure (debit success / credit fail) | ⬜ Unverified — compensation flow not yet implemented |
| R-03 | Concurrent duplicate idempotency key | ⬜ Unverified — Redis SET NX + DB constraint not yet in place |
| R-04 | Redis unavailability | ⬜ Unverified — fail-safe strategy not yet implemented |
| R-05 | NATS unavailability | ⬜ Unverified — DLQ not yet implemented |
| R-06 | Retry re-executes partial transaction | ⬜ Unverified — state machine guard not yet implemented |
| R-07 | Scheduled payment fires multiple times | ⬜ Unverified — distributed lock not yet implemented |
| R-08 | Double reversal | ⬜ Unverified — unique constraint not yet in place |

---

## Recommendations

### Immediate (before first deployment)

| Priority | Action | File / component |
|---|---|---|
| P0 | Implement compensation flow (BW-02) before enabling any debit/credit operations | `internal/service/transfer.go` |
| P0 | Implement Redis `SET NX` idempotency before opening any payment endpoints | `internal/infra/redis/idempotency_store.go` |
| P0 | Add DB unique constraint on `transactions.idempotency_key` as backstop | `migrations/001_create_payments_tables.sql` |
| P0 | Add DB unique constraint on `reversals.original_txn_id` | `migrations/001_create_payments_tables.sql` |

### Short-term (first sprint after initial delivery)

| Priority | Action |
|---|---|
| P1 | Define and document the stale Processing recovery runbook (TD-02) |
| P1 | Add integration tests for the circuit breaker behaviour (E9-T05) |
| P1 | Operationalise DLQ: monitoring alert + reprocessing procedure (TD-01) |

### Medium-term

| Priority | Action |
|---|---|
| P2 | Resolve TD-03: define behaviour when distributed lock holder dies mid-execution (heartbeat or TTL extension) |
| P2 | Add load/performance tests to verify latency targets under concurrent transaction volume |
| P2 | Define and implement transaction limit configuration per product type (FR-17) |

---

## Refactoring Opportunities

To be populated after implementation is complete and code review findings are available.
