# Payment Service — Context

## Domain Overview

The Payment Service is the transaction orchestration layer of the banking platform. It owns the complete lifecycle of every financial transaction — from initial validation through debit, credit, state management, and event publishing — but it never owns account state or balances. Its single external runtime dependency for monetary operations is the Account Service, which it calls synchronously to execute debits and credits. All other consumers (Notification, Audit, Reporting, Reconciliation) are downstream and receive events asynchronously.

---

## Business Context

Without a dedicated orchestration layer, each payment product (transfers, merchant payments, refunds, fee postings) would need to independently implement idempotency enforcement, balance validation, debit/credit coordination, partial failure recovery, and event publishing. That duplication would introduce inconsistency across products and pull business-product logic into the Account Service.

The Payment Service exists to prevent that. It is the single place where the platform's business rules for monetary movement are applied. New payment products are introduced by adding a product-specific handler inside this service, not by modifying the Account Service or other services.

From a compliance perspective, the service centralises the audit trail for all financial transactions. Every state transition, failure reason, and retry attempt is recorded in one place, making regulatory reporting and incident investigation tractable.

---

## Service Responsibilities

| Capability | Description |
|---|---|
| Transaction initiation | Accept and validate inbound payment requests for all supported product types |
| Idempotency enforcement | Detect duplicate requests and return cached responses without reprocessing |
| Business rule validation | Enforce account existence, status, balance sufficiency, and transaction limits |
| Account orchestration | Coordinate debit and credit calls to the Account Service in the correct sequence |
| Transaction state management | Maintain and transition transaction states through the full lifecycle |
| Partial failure compensation | Detect and recover from debit-success/credit-failure scenarios |
| Reversal processing | Execute compensating operations for previously successful transactions |
| Cancellation processing | Cancel transactions that have not yet reached a final state |
| Retry management | Re-execute recoverable failed transactions without creating duplicate records |
| Scheduled payment execution | Execute time-triggered payments via asynchronous message consumption |
| Event publishing | Publish transaction lifecycle events to all downstream consumers via NATS |
| Audit record generation | Ensure every operation produces a complete, queryable audit trail |

---

## Bounded Context Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          payment-svc boundary                           │
│                                                                         │
│   ┌─────────┐    ┌────────────┐    ┌─────────────────────────────────┐ │
│   │  HTTP   │───▶│  Request   │───▶│       Transaction               │ │
│   │   API   │    │ Validator  │    │       Orchestrator              │ │
│   └─────────┘    └────────────┘    └──────────────┬──────────────────┘ │
│                                                   │                     │
│              ┌──────────────────────┬─────────────┼──────────────────┐ │
│              ▼                      ▼             ▼                  ▼ │
│   ┌──────────────────┐  ┌─────────────────┐  ┌──────────┐  ┌──────────┐│
│   │  Idempotency     │  │  Transaction    │  │ Account  │  │  Event   ││
│   │  Store (Redis)   │  │  Repository    │  │  Client  │  │Publisher ││
│   └──────────────────┘  └───────┬─────────┘  └────┬─────┘  └────┬─────┘│
│                                 │                  │             │      │
│                        ┌────────▼──────┐           │             │      │
│                        │  banking_     │           │             │      │
│                        │  payments DB  │           │             │      │
│                        └───────────────┘           │             │      │
└────────────────────────────────────────────────────┼─────────────┼──────┘
                                                     │             │
                                              ┌──────▼──────┐  ┌───▼────┐
                                              │ account-svc │  │  NATS  │
                                              └─────────────┘  └───┬────┘
                                                               ┌────▼──────────────────┐
                                                               │ notification-svc       │
                                                               │ audit-svc             │
                                                               │ reporting / recon     │
                                                               └───────────────────────┘
```

---

## Actors

| Actor | Type | Interaction |
|---|---|---|
| End user | Human | Initiates payments via client application; receives payment results |
| Client application | Machine | Calls Payment Service HTTP API; responsible for generating idempotency keys |
| Scheduler / job runner | Machine | Publishes scheduled payment messages to NATS for async execution |
| Account Service | Machine | Called synchronously for account validation, debit, and credit operations |
| Notification Service | Machine | Consumes transaction events from NATS to send user notifications |
| Audit Service | Machine | Consumes transaction events from NATS to persist audit records |
| Reporting Service | Machine | Consumes transaction events from NATS for analytics and reporting |
| Reconciliation process | Machine | Consumes transaction events from NATS for settlement and reconciliation |
| Prometheus / monitoring | Machine | Scrapes metrics endpoint for observability |

---

## Business Workflows

### BW-01 — Account Transfer (FR-01, FR-13 – FR-18, NFR-01)

1. Client sends `POST /v1/payments/transfer` with idempotency key in header
2. Validate request structure (required fields, amount > 0, valid currency)
3. Look up idempotency key in Redis — if found and not expired, return cached response immediately
4. Validate source and destination accounts exist via Account Service
5. Validate both accounts are in an active/eligible status
6. Validate source account balance is sufficient for the requested amount
7. Validate amount does not exceed configured transaction limits
8. Create transaction record in `banking_payments` with state `Pending`
9. Store idempotency key → transaction ID mapping in Redis with TTL
10. Transition state to `Processing`
11. Call Account Service: debit source account
12. Call Account Service: credit destination account
13. Transition state to `Success`; record completion timestamp
14. Publish `transaction.completed` event to NATS
15. Return success response with transaction details
16. Cache final response against idempotency key

### BW-02 — Partial Failure Recovery (NFR-06, AC-07)

Enters after step 11 of BW-01 when step 12 (credit) fails:

1. Detect credit failure response from Account Service
2. Call Account Service: compensating debit reversal on source account (restore balance)
3. If compensating debit also fails — mark transaction `Failed` with reason `COMPENSATION_REQUIRED` and enqueue to DLQ for manual recovery
4. If compensating debit succeeds — transition original transaction to `Failed` with reason capturing the credit failure
5. Publish `transaction.failed` event to NATS
6. Return error response to caller

### BW-03 — Transaction Reversal (FR-10, AC-03, AC-11)

1. Client sends `POST /v1/payments/{id}/reverse`
2. Load original transaction; validate it exists
3. Validate transaction state is `Success` — reject if already `Reversed`, `Failed`, or `Cancelled`
4. Validate no reversal record already exists for this transaction (prevent duplicates)
5. Create reversal transaction record with state `Pending`
6. Call Account Service: credit source account (restore original debited amount)
7. Call Account Service: debit destination account (reclaim original credited amount)
8. Transition reversal record to `Success`; transition original transaction to `Reversed`
9. Publish `transaction.reversed` event to NATS
10. Return reversal confirmation

### BW-04 — Transaction Cancellation (FR-11, AC-04)

1. Client sends `POST /v1/payments/{id}/cancel`
2. Load transaction; validate it exists
3. Validate state is `Pending` — reject with 422 if state is `Processing`, `Success`, `Reversed`, or `Failed`
4. Transition state to `Cancelled`
5. Publish `transaction.cancelled` event to NATS
6. Return cancellation confirmation

### BW-05 — Transaction Retry (FR-12, AC-05)

1. Triggered by client request or internal retry scheduler
2. Load original transaction; validate it exists and state is `Failed`
3. Validate retry count has not exceeded the configured maximum
4. Validate the failure reason is a recoverable error type
5. Increment retry count; record retry timestamp
6. Re-enter BW-01 from step 10 (Processing), reusing the original transaction record
7. On success — transition to `Success`; publish `transaction.completed`
8. On failure — apply exponential backoff delay; re-enqueue if max retries not reached, else transition to terminal `Failed`

### BW-06 — Idempotency Enforcement (FR-13, NFR-01, AC-02, AC-12)

1. Extract idempotency key from `Idempotency-Key` request header
2. Validate key is present; reject with 400 if missing
3. Attempt atomic `SET NX` in Redis: key → request fingerprint, TTL = configured expiry
4. If key already exists — load cached response and return it immediately (no processing)
5. If key is new — allow processing to proceed
6. After processing completes — store final HTTP response (status + body) in Redis keyed by idempotency key

### BW-07 — Asynchronous Message Processing (FR-20, FR-05)

1. NATS consumer receives message on payment processing queue
2. Deserialise and validate message envelope
3. Route to appropriate handler based on message type (scheduled payment, retry, external trigger)
4. Execute the relevant business workflow (BW-01 through BW-05 as applicable)
5. Publish result event on completion
6. Acknowledge message on success; negatively acknowledge (NACK) on failure to trigger redelivery
7. On repeated failure — route to dead-letter queue (NFR-04)

### BW-08 — Transaction Inquiry (FR-06 – FR-09)

1. Client sends `GET /v1/payments/{id}` or `GET /v1/payments?account_id=…`
2. Validate path/query parameters
3. Query `banking_payments` for matching records
4. Return transaction details including current state, failure reason if applicable, and all lifecycle timestamps

---

## Upstream Systems

| System | Coupling | Call type | Purpose |
|---|---|---|---|
| Account Service | Strong — in the critical payment path | Synchronous HTTP | Account existence check, status check, balance inquiry, debit, credit |

The Account Service is the only upstream system with strong coupling. The Payment Service cannot complete a transaction without it. This coupling is intentional — balance mutations must be authoritative and consistent through a single owner.

---

## Downstream Systems

| System | Coupling | Call type | What it consumes |
|---|---|---|---|
| Notification Service | Weak — async | NATS subscription | `transaction.completed`, `transaction.failed`, `transaction.reversed`, `transaction.cancelled` |
| Audit Service | Weak — async | NATS subscription | All transaction lifecycle events |
| Reporting Service | Weak — async | NATS subscription | All transaction lifecycle events |
| Reconciliation process | Weak — async | NATS subscription | `transaction.completed`, `transaction.reversed` |
| Prometheus / monitoring | None — pull | HTTP metrics scrape | Latency histograms, counters, error rates |

Downstream coupling is intentionally weak. The Payment Service publishes events and does not wait for downstream acknowledgement. Transaction success is independent of event delivery.

---

## Dependencies Map

| Dependency | Type | Purpose |
|---|---|---|
| `chi` | External package | HTTP routing |
| `pkg/httpx` | Internal shared | HTTP response helpers, request decoding |
| `pkg/errors` | Internal shared | Domain error types |
| `pkg/audit` | Internal shared | NATS event publisher client |
| `database/sql` + `pgx` | External package | Postgres driver |
| `banking_payments` | Postgres database | Transaction records, idempotency fallback store |
| Redis | External store | Primary idempotency key store; distributed scheduler lock |
| NATS | Message broker | Async event publishing; inbound async payment messages |
| Account Service HTTP API | Internal service | Account validation, debit, credit |

---

## Risks

| ID | Threat | Mitigation | References |
|---|---|---|---|
| R-01 | Account Service unavailable during debit/credit — payment processing halts entirely | Circuit breaker (NFR-03) + exponential backoff retry (NFR-02) + DLQ (NFR-04); design Account Service timeout budget explicitly | NFR-02, NFR-03, NFR-04, A-01 |
| R-02 | Partial failure: debit succeeds but credit fails — funds leave source account and do not arrive at destination | Immediate compensating debit reversal on credit failure (BW-02); if compensation also fails, flag `COMPENSATION_REQUIRED` and enqueue to DLQ for manual recovery | NFR-06, AC-07 |
| R-03 | Concurrent duplicate requests with the same idempotency key — two transaction records created | Atomic Redis `SET NX` for first-writer-wins; database unique constraint on `(idempotency_key)` as backstop | NFR-01, AC-12 |
| R-04 | Redis (idempotency store) unavailable — cannot enforce idempotency | Fail-safe: reject inbound requests when Redis is unreachable rather than risk duplicates; fallback to DB-backed idempotency check as degraded mode | NFR-01, A-02 |
| R-05 | NATS unavailable — transaction events not published | Event publishing is non-blocking; local retry with bounded attempts; undelivered events route to DLQ; audit trail in `banking_payments` DB is the source of truth regardless of event delivery | NFR-04, FR-19, A-02 |
| R-06 | Retry re-executes a partially completed transaction — double debit | State machine guard: only `Failed` transactions with recoverable failure codes are eligible for retry; idempotency key check at retry entry re-uses original key | FR-12, NFR-01, AC-05 |
| R-07 | Scheduled payments fire multiple times on instance restart or race between scaled instances | Distributed lock (Redis) acquired before scheduler execution; lock TTL matches minimum execution interval; last-execution timestamp recorded in DB | FR-05, NFR-13 |
| R-08 | Reversal executed on an already-reversed transaction — funds moved twice | Unique constraint on reversal records per source transaction ID; state guard in BW-03 step 3 | FR-10, AC-11 |

---

## Assumptions Revisited

| ID | Assumption | How it shapes the implementation |
|---|---|---|
| A-01 | Account Service is available and responsive for synchronous calls | Circuit breaker and retry are mandatory, not optional. The timeout budget for a payment request must account for two Account Service calls (debit + credit) plus network overhead. If Account Service p99 latency is high, payment p99 will be worse. |
| A-02 | NATS is available for async event publishing and message consumption | Event publishing is fire-and-forget — payment success must never block on NATS delivery. The DLQ is the backstop. If NATS is down for an extended period, DLQ capacity and reprocessing procedures must be defined operationally. |
| A-03 | Clients generate idempotency keys | The API contract must document the `Idempotency-Key` header as required, the recommended format (UUID v4), and the expiry window. The service validates presence and length but does not verify uniqueness of the key format itself. |
| A-04 | Port 8085 is reserved | Docker Compose, API gateway routing, and service discovery configurations must reference `8085` for this service. No other service in the `808x` range should be assigned this port. |
| A-05 | `banking_payments` database is pre-provisioned | The service health check must verify DB connectivity at startup and return unhealthy if the DB is unreachable. Migration scripts must be applied before the service container starts. |
| A-06 | Account Service debit/credit APIs are atomic within their own store | The Payment Service does not implement sub-ledger consistency. It treats each Account Service response as authoritative for that individual operation. This means the risk of inter-service inconsistency (R-02) is fully owned by the Payment Service's compensation logic. |
