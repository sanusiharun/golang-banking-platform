# goals.md — auth-svc

> **Source of truth.** Every item in context.md, architecture.md, progress-tracking.md, and review.md must be traceable back to a requirement defined here.

---

## 1. Service Identity

| Field | Value |
|---|---|
| **Service** | `auth-svc` |
| **Port** | `8082` |
| **Database** | `banking_auth` (PostgreSQL) |
| **Owner domain** | Identity & Access Management (IAM) |
| **Criticality** | Tier 1 — all other services depend on it |

---

## 2. Business Objectives

| ID | Objective |
|---|---|
| BO-01 | Enable human users to authenticate with username/password and receive short-lived access tokens. |
| BO-02 | Enable machine clients (services, scripts, CI) to authenticate with API keys tied to service accounts. |
| BO-03 | Prevent unauthorised access to banking resources by enforcing role-based access control on all protected endpoints. |
| BO-04 | Provide a full audit trail of every authentication and credential-management event for compliance and forensics. |
| BO-05 | Allow operations to pause login activity without a code deployment (maintenance window, incident response). |
| BO-06 | Protect the identity of users inside tokens so that token leakage does not expose internal user IDs. |

---

## 3. Functional Requirements

### 3.1 Human Authentication

| ID | Requirement |
|---|---|
| FR-01 | The service MUST accept `username` + `password` and, on success, return a signed access token (JWT RS256) and a refresh token. |
| FR-02 | The access token MUST expire within 15 minutes. |
| FR-03 | The refresh token MUST expire after 7 days and be single-use (revoked on first use). |
| FR-04 | The service MUST provide a token refresh endpoint that issues a new token pair and revokes the old refresh token. |
| FR-05 | The service MUST provide a logout endpoint that revokes the presented refresh token. |
| FR-06 | The service MUST block login when the `maintenance_mode` feature flag is active and return HTTP 503. |
| FR-07 | The service MUST never reveal whether a specific username exists (timing-safe dummy bcrypt for unknown users). |
| FR-08 | Inactive users MUST be rejected at login regardless of password correctness. |

### 3.2 Service Account & API Key Management

| ID | Requirement |
|---|---|
| FR-09 | Administrators MUST be able to create, read, update, and deactivate service accounts. |
| FR-10 | Administrators MUST be able to create and revoke API keys scoped to a service account. |
| FR-11 | An API key MUST be shown to the creator exactly once (at creation). The raw key MUST NOT be stored. |
| FR-12 | An API key prefix MUST be derivable without storing the raw key (`bp_live_` or `bp_test_` + 10-char prefix). |
| FR-13 | Machine clients MUST be able to authenticate using an API key. The resolved identity MUST carry tenant and roles. |
| FR-14 | API key lookup MUST be optimised for high frequency (cache-aside Redis layer). |
| FR-15 | Revoking an API key MUST immediately invalidate any cached entry. |
| FR-16 | API key expiry MUST be enforced at lookup time. |

### 3.3 Token Store

| ID | Requirement |
|---|---|
| FR-17 | The token store MUST support at minimum PostgreSQL and in-memory backends, selectable at runtime. |
| FR-18 | A Redis backend MUST be available as a high-performance token store option. |
| FR-19 | All refresh token stores MUST enforce expiry and revocation atomically. |

### 3.4 Observability & Operations

| ID | Requirement |
|---|---|
| FR-20 | All HTTP endpoints MUST expose Prometheus metrics (request latency histogram, request count by status). |
| FR-21 | All HTTP endpoints MUST produce OpenTelemetry spans. |
| FR-22 | All significant actions MUST publish a structured audit event to NATS (non-blocking). |
| FR-23 | The service MUST expose `/healthz/live` and `/healthz/ready` endpoints. |
| FR-24 | Idempotency MUST be supported for state-mutating endpoints via `Idempotency-Key` header. |

---

## 4. Non-Functional Requirements

| ID | Requirement | Target |
|---|---|---|
| NFR-01 | **Latency** — P99 login latency (including bcrypt) | ≤ 500 ms |
| NFR-02 | **Latency** — P99 token refresh latency | ≤ 100 ms |
| NFR-03 | **Latency** — P99 API key introspection latency (cache hit) | ≤ 10 ms |
| NFR-04 | **Latency** — P99 API key introspection latency (cache miss) | ≤ 50 ms |
| NFR-05 | **Availability** | ≥ 99.9 % |
| NFR-06 | **Security** — Tokens MUST be signed with RS256; private key never leaves auth-svc | — |
| NFR-07 | **Security** — Passwords MUST be hashed with bcrypt cost ≥ 12 | — |
| NFR-08 | **Security** — JWT Subject MUST be encrypted (AES-256-GCM) before embedding user ID | — |
| NFR-09 | **Security** — Refresh tokens and API key hashes MUST use SHA-256; raw values never persisted | — |
| NFR-10 | **Security** — API keys MUST carry environment prefix to prevent cross-environment misuse | — |
| NFR-11 | **Scalability** — The service MUST be stateless above the storage layer; horizontal scaling via replica | — |
| NFR-12 | **Operability** — Feature flags MUST be changeable without a deployment | — |
| NFR-13 | **Operability** — DB migrations run automatically at startup | — |
| NFR-14 | **Reliability** — Audit publisher failure MUST NOT affect user-facing endpoints | — |
| NFR-15 | **Reliability** — Redis unavailability MUST fall back to PostgreSQL, not cause 500s | — |
| NFR-16 | **Maintainability** — Business logic MUST reside in service layer, not handlers | — |
| NFR-17 | **Maintainability** — All shared helpers reside in `pkg/` and are import-only (no circular deps) | — |
| NFR-18 | **Compliance** — Full audit trail of authentication and credential events must be retained | — |

---

## 5. Constraints

| ID | Constraint |
|---|---|
| C-01 | Language: Go; router: chi + stdlib only (no Gin, Echo, Fiber). |
| C-02 | Each service owns exactly one database; cross-service DB access is forbidden. |
| C-03 | Services communicate via HTTP only; no shared memory or gRPC (for now). |
| C-04 | Docker Compose for local and CI; no Kubernetes at this stage. |
| C-05 | RSA key pair generated externally and injected via environment variable (base64). |
| C-06 | `.env` files and `CREDENTIALS.txt` MUST NOT be committed to git. |
| C-07 | Introspect endpoint (`/auth/apikey/introspect`) is reachable only within `banking-net` Docker network. |

---

## 6. Assumptions

| ID | Assumption |
|---|---|
| A-01 | A single PostgreSQL instance hosts all three service databases on separate schemas. Production may split this. |
| A-02 | Redis is available in production; the PostgreSQL fallback is for degraded mode only. |
| A-03 | NATS JetStream is available; audit events will eventually be delivered (no business impact if delayed). |
| A-04 | Flipt is available; unavailability defaults flags to `false` (maintenance mode off). |
| A-05 | `banking-net` Docker bridge network provides sufficient isolation for the internal introspect endpoint. |
| A-06 | User provisioning (creation) is out of scope for auth-svc; users are seeded or created via a future admin service. |

---

## 7. Acceptance Criteria

| ID | Criterion | Linked Requirement |
|---|---|---|
| AC-01 | Login with valid credentials returns HTTP 200, JWT, and refresh token. | FR-01 |
| AC-02 | Login with invalid password returns HTTP 401; timing is indistinguishable from non-existent user. | FR-07 |
| AC-03 | Login when `maintenance_mode` flag is active returns HTTP 503. | FR-06 |
| AC-04 | Refresh with valid token returns new JWT + refresh token; old refresh token is revoked. | FR-03, FR-04 |
| AC-05 | Refresh with already-revoked token returns HTTP 401. | FR-03 |
| AC-06 | Logout revokes refresh token; subsequent refresh with same token returns 401. | FR-05 |
| AC-07 | Create service account + API key; introspect with API key returns correct identity. | FR-09–FR-13 |
| AC-08 | Revoke API key; subsequent introspect returns 401 immediately (within Redis TTL margin). | FR-15 |
| AC-09 | Create API key with expiry in the past; introspect returns 401. | FR-16 |
| AC-10 | `GET /healthz/ready` returns 200 when Postgres is reachable. | FR-23 |
| AC-11 | Prometheus metrics endpoint returns metrics including `http_request_duration_seconds`. | FR-20 |
| AC-12 | Audit event published to NATS for every login, logout, refresh, API key create, and revoke. | FR-22, BO-04 |
| AC-13 | Redis unavailability does not cause 5xx on token operations. | NFR-15 |

---

## 8. Service Boundaries

auth-svc **owns**:
- User credential verification
- JWT issuance and signing (private key)
- Refresh token lifecycle
- Service account and API key management
- Authentication audit events (publish only)

auth-svc **does NOT own**:
- User creation / profile management
- Account or transaction logic
- Audit event storage (consumed by audit-svc)
- JWT verification in downstream services (they hold the public key)
- Business-level authorisation decisions (each service enforces its own rules after authentication)
