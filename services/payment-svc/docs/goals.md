# Payment Service — Goals

## Service Identity

| Property | Value |
|---|---|
| Service name | `payment-svc` |
| Port | `8085` |
| Database | `banking_payments` |
| Owner domain | Transaction processing / Payment orchestration |
| Criticality tier | Critical — handles all monetary movements |

---

## Business Objectives

| ID | Objective |
|---|---|
| BO-01 | Centralise all monetary transaction processing under a single service so that payment logic is never duplicated across consumers |
| BO-02 | Decouple transaction workflows from account management so that the Account Service remains focused on account state and balance custody |
| BO-03 | Provide a single, consistent transaction processing model that supports multiple payment products without requiring changes to the Account Service |
| BO-04 | Ensure every financial transaction is traceable, consistent, and fully auditable end-to-end |
| BO-05 | Provide resilient and idempotent transaction processing so that network retries and client failures cannot produce duplicate monetary movements |

---

## Functional Requirements

### Payment Initiation

| ID | Requirement |
|---|---|
| FR-01 | The service shall accept and process internal account transfer requests |
| FR-02 | The service shall accept and process merchant payment requests |
| FR-03 | The service shall accept and process fee-charging requests |
| FR-04 | The service shall accept and process refund requests |
| FR-05 | The service shall execute scheduled payment requests at their configured time |

### Payment Inquiry

| ID | Requirement |
|---|---|
| FR-06 | The service shall provide an API to retrieve full transaction details by transaction ID |
| FR-07 | The service shall provide an API to retrieve the current status of a transaction |
| FR-08 | The service shall provide an API to retrieve paginated transaction history for an account |
| FR-09 | The service shall expose the failure reason for any transaction in a Failed state |

### Payment Lifecycle Operations

| ID | Requirement |
|---|---|
| FR-10 | The service shall reverse a previously successful transaction by executing compensating debit and credit operations and updating the transaction state to Reversed |
| FR-11 | The service shall cancel a transaction that has not yet reached a final state (Success, Failed, Reversed) |
| FR-12 | The service shall retry failed transactions caused by recoverable errors, preserving the original transaction reference and preventing duplicate records |

### Validation

| ID | Requirement |
|---|---|
| FR-13 | The service shall validate the idempotency key on every inbound transaction request |
| FR-14 | The service shall validate that the source and destination accounts exist before processing |
| FR-15 | The service shall validate that both accounts are in an active/eligible status before processing |
| FR-16 | The service shall validate that the source account has sufficient balance before executing a debit |
| FR-17 | The service shall validate that the requested amount does not exceed configured transaction limits |
| FR-18 | The service shall detect and reject duplicate transactions independent of idempotency key checks |

### Event Publishing

| ID | Requirement |
|---|---|
| FR-19 | The service shall publish a transaction event after every state transition to downstream consumers: Notification Service, Audit Service, Reporting Service, Monitoring, and Reconciliation |

### Asynchronous Processing

| ID | Requirement |
|---|---|
| FR-20 | The service shall support message-driven transaction processing to handle high volume, delayed processing, scheduled transactions, external integrations, and retry flows |

---

## Non-Functional Requirements

### Idempotency

| ID | Requirement |
|---|---|
| NFR-01 | Every transaction request must carry an idempotency key; the service must store the processing result and return the original response for any duplicate request received within a configurable expiration window |

### Reliability

| ID | Requirement |
|---|---|
| NFR-02 | The service shall implement retry with exponential backoff for recoverable failures when calling the Account Service |
| NFR-03 | The service shall implement a circuit breaker on all Account Service calls to prevent cascade failures |
| NFR-04 | The service shall route unprocessable messages to a dead-letter queue for inspection and reprocessing |
| NFR-05 | The service shall handle request timeouts gracefully and return a retriable error to the caller |

### Consistency

| ID | Requirement |
|---|---|
| NFR-06 | Debit and credit operations must be treated as a single business transaction; a partial failure (debit succeeds, credit fails) must trigger compensation or be flagged for recovery |
| NFR-07 | Every transaction must be fully traceable from initiation to final state through its transaction ID, reference number, and correlation ID |

### Observability

| ID | Requirement |
|---|---|
| NFR-08 | The service shall emit structured logs with trace context on every significant operation |
| NFR-09 | The service shall support distributed tracing across all Account Service calls and message broker interactions |
| NFR-10 | The service shall expose Prometheus metrics including: processing latency (histogram), success count, failure count, transaction throughput, and error rate |
| NFR-11 | The service shall expose a health check endpoint reporting its own and its dependency liveness |

### Auditability

| ID | Requirement |
|---|---|
| NFR-12 | All transaction activities must produce an auditable record including: user identity, transaction references, processing timestamps, failure reasons, system-generated actions, reversal information, and retry counts |

### Scalability

| ID | Requirement |
|---|---|
| NFR-13 | The service shall be stateless and support horizontal scaling without coordination between instances |
| NFR-14 | The service shall be independently deployable without requiring co-deployment of other services |

---

## Constraints

| ID | Constraint |
|---|---|
| C-01 | Implementation language: Go; router: chi; no heavy frameworks (Gin, Echo, Fiber) |
| C-02 | The Payment Service must never write directly to account balance columns; all balance mutations must go through Account Service APIs |
| C-03 | The Payment Service owns only the `banking_payments` database; cross-service direct DB access is forbidden |
| C-04 | Inter-service communication is HTTP only; no shared in-process imports between services |
| C-05 | All services run on Docker using the `banking-net` bridge network |
| C-06 | All HTTP responses must use `pkg/httpx` helpers; no local response utilities |

---

## Assumptions

| ID | Assumption |
|---|---|
| A-01 | The Account Service is available and responsive for synchronous debit/credit calls during transaction processing; its SLA is acceptable for inline orchestration |
| A-02 | A NATS message broker is available on `banking-net` for asynchronous event publishing |
| A-03 | Clients are responsible for generating idempotency keys; the Payment Service validates uniqueness but does not generate them |
| A-04 | Port `8085` is available and reserved for the Payment Service following the `808x` scheme |
| A-05 | A dedicated Postgres database `banking_payments` will be provisioned before the service is deployed |
| A-06 | The Account Service debit and credit APIs are atomic with respect to their own store; the Payment Service does not need to manage sub-account consistency |

---

## Acceptance Criteria

| ID | Links | Criterion |
|---|---|---|
| AC-01 | FR-01 | A transfer request with valid accounts and sufficient balance completes successfully; both accounts reflect updated balances when queried through the Account Service |
| AC-02 | FR-13, NFR-01 | A second request with the same idempotency key returns the original response body and HTTP status; no second transaction record is created |
| AC-03 | FR-10 | Reversing a successful transaction creates a compensating operation, sets the original transaction to `Reversed`, and publishes a reversal event |
| AC-04 | FR-11 | Cancelling a `Pending` transaction sets its state to `Cancelled`; cancelling a `Success` or `Reversed` transaction is rejected with a 422 |
| AC-05 | FR-12 | Retrying a recoverable failed transaction reuses the original transaction ID and reference; no duplicate monetary movement occurs |
| AC-06 | FR-16 | A transfer where the source account balance is insufficient is rejected before any debit is executed; the transaction is recorded as `Failed` with the reason `INSUFFICIENT_BALANCE` |
| AC-07 | NFR-06 | If the credit operation fails after a successful debit, the service records the partial failure, initiates a compensating reversal of the debit, and marks the transaction `Failed` |
| AC-08 | NFR-03 | When the Account Service returns errors beyond the circuit breaker threshold, the Payment Service returns a retriable error to the caller without invoking the Account Service further |
| AC-09 | NFR-10 | Processing latency, success count, failure count, and error rate are all observable at the Prometheus metrics endpoint |
| AC-10 | FR-19 | After every terminal state transition, a corresponding event is consumable by the Notification Service and Audit Service |
| AC-11 | FR-10 | A reversal request on a transaction that is already `Reversed` is rejected; duplicate reversals are prevented |
| AC-12 | NFR-13 | Two concurrent instances of the service processing the same idempotency key produce exactly one transaction record |

---

## Service Boundaries

### In scope

- Transaction record creation and full lifecycle management
- Business rule validation for all payment operations
- Orchestration of debit and credit via Account Service APIs
- Idempotency enforcement and duplicate detection
- Reversal, cancellation, and retry workflows
- Scheduled and asynchronous payment processing
- Event publishing for all transaction outcomes

### Out of scope

| Concern | Owner |
|---|---|
| Account creation and account master data | Account Service |
| Balance storage and direct balance mutations | Account Service |
| User authentication and JWT issuance | Auth Service |
| Audit log storage and querying | Audit Service |
| Notification delivery | Notification Service |
| Reporting, analytics, and reconciliation storage | Downstream consumers |
| QR payments, Virtual Accounts, external integrations | Future payment products (extensible via FR-20) |
