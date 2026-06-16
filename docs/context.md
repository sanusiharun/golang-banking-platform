# context.md — auth-svc

> **Purpose:** Translate requirements from [goals.md](goals.md) into domain understanding. Explains *why* auth-svc exists and *how* it fits into the banking platform ecosystem.

---

## 1. Domain Overview

auth-svc is the **Identity & Access Management** boundary of the banking platform. It answers one question on behalf of every other service: *"Who is this caller, and are they allowed to be here?"*

It does this through two mechanisms:

1. **JWT issuance** — Human users exchange credentials for short-lived, cryptographically signed tokens. Downstream services validate these tokens locally (public key only) without calling auth-svc on every request.
2. **API key introspection** — Machine clients present an opaque API key. auth-svc resolves it to a `ServiceAccountIdentity` (tenant, roles, key ID) so downstream services can apply the same RBAC logic.

The asymmetry is deliberate: JWTs are verified without network calls (low latency, high throughput); API keys require a lookup (but are cached in Redis for sub-10 ms hot-path performance).

---

## 2. Business Context

### Why this service exists (→ BO-01 to BO-06)

A multi-service banking platform has two categories of caller:
- **Human operators and customers** who authenticate interactively with a username and password.
- **Machine clients** (background jobs, partner integrations, internal automation) that need a non-interactive, long-lived credential.

Both need authenticated access to `account-svc` and future services. Without a central IAM service:
- Every service would implement its own password hashing and token issuance — a security anti-pattern.
- There would be no single place to revoke access (logout, key rotation, incident response).
- Audit evidence of who accessed what would be scattered across services.

auth-svc centralises these concerns so all other services can be **authentication-agnostic** (they only validate an existing token/key, never issue one).

### Compliance driver (→ BO-04, NFR-18)

Banking platforms are subject to regulatory audit requirements. Every authentication event, credential creation, and revocation must be traceable to a specific actor, timestamp, and outcome. auth-svc publishes structured audit events to NATS for consumption by audit-svc.

### Operational driver (→ BO-05, FR-06)

Maintenance windows, incident response, and emergency shutdowns require the ability to halt all new logins without redeploying code. The `maintenance_mode` feature flag in Flipt satisfies this requirement.

---

## 3. Service Responsibilities

| Responsibility | Description |
|---|---|
| **Credential verification** | Validate username+password against bcrypt-hashed stored credential. |
| **JWT issuance** | Sign RS256 access tokens; encrypt user ID in Subject claim. |
| **Refresh token lifecycle** | Issue, validate, rotate, and revoke single-use refresh tokens. |
| **Service account management** | CRUD for non-human identities (tenant, roles, active flag). |
| **API key management** | Create, list, and revoke API keys bound to service accounts. |
| **API key introspection** | Resolve opaque key hash → `ServiceAccountIdentity` (hot path). |
| **Audit event publishing** | Fire-and-forget publish to NATS on every significant action. |
| **Health & metrics exposure** | Liveness, readiness, Prometheus, OpenTelemetry spans. |

---

## 4. Bounded Context

```
┌─────────────────────────────────────────────────────┐
│                   auth-svc boundary                  │
│                                                       │
│  ┌──────────┐   ┌─────────────┐   ┌──────────────┐  │
│  │  AuthSvc │   │ APIKeySvc   │   │FeatureFlagSvc│  │
│  └──────────┘   └─────────────┘   └──────────────┘  │
│        │               │                             │
│  ┌──────────┐   ┌─────────────────────────────────┐  │
│  │TokenStore│   │   APIKeyStore (Postgres+Redis)   │  │
│  │(pluggable│   └─────────────────────────────────┘  │
│  │P/R/Mem)  │                                        │
│  └──────────┘                                        │
│                                                       │
│  Owns: users, refresh_tokens, service_accounts,      │
│         api_keys, idempotency_requests                │
│  Database: banking_auth                               │
└─────────────────────────────────────────────────────┘
         │ publishes               │ JWT (RS256 public key)
         ▼                         ▼
    NATS JetStream             account-svc, future services
         │
         ▼
      audit-svc
```

---

## 5. Actors

| Actor | Type | Description |
|---|---|---|
| **End user** | Human | Authenticates via `/auth/login`; uses JWT for subsequent API calls. |
| **Admin user** | Human (ADMIN role) | Creates service accounts and API keys via `/internal/service-accounts/*`. |
| **Machine client** | Non-human | Authenticates via API key; resolved to `ServiceAccountIdentity`. |
| **account-svc** | Internal service | Calls `/auth/apikey/introspect` to resolve API key → identity. |
| **audit-svc** | Internal service | Consumes NATS `audit.*` subject; no direct HTTP calls to auth-svc. |
| **Operations** | Human (Flipt) | Toggles `maintenance_mode` flag without deployment. |
| **Monitoring** | System | Scrapes `/metrics`; reads `/healthz/live` and `/healthz/ready`. |

---

## 6. Business Workflows

### 6.1 Human Login Flow (→ FR-01 to FR-08)

```
User → POST /auth/login {username, password}
  │
  ├─ Feature flag check: maintenance_mode?
  │   └─ YES → 503 Service Unavailable
  │
  ├─ Load user from DB (or timing-safe dummy if not found)
  ├─ bcrypt.Compare(password, stored_hash)  [always runs, even for unknown users]
  │   └─ FAIL → 401 Unauthorized; publish auth.login_failed audit event
  │
  ├─ Check user.is_active == true
  │   └─ FALSE → 401 Unauthorized
  │
  ├─ Encrypt user.ID → AES-256-GCM ciphertext → JWT Subject
  ├─ Sign JWT with RSA private key (RS256, 15 min TTL)
  ├─ Generate refresh token UUID → SHA-256 hash → persist in TokenStore (7 days TTL)
  │
  ├─ Publish auth.login_success audit event (async, non-blocking)
  └─ Return {access_token, refresh_token, expires_in}
```

### 6.2 Token Refresh Flow (→ FR-03, FR-04)

```
User → POST /auth/refresh {refresh_token}
  │
  ├─ SHA-256(refresh_token) → lookup in TokenStore
  │   └─ NOT FOUND / REVOKED / EXPIRED → 401
  │
  ├─ Revoke old token (atomic with find)
  ├─ Reload user (check still active)
  ├─ Issue new JWT + new refresh token (same as login flow)
  │
  └─ Publish auth.token_refresh audit event
```

### 6.3 API Key Authentication Flow (→ FR-13, FR-14)

```
Machine client → [Any protected endpoint on account-svc with Authorization: Bearer bp_live_xxx...]
  │
  account-svc middleware → POST /auth/apikey/introspect {hash: SHA256(key)}
  │
  auth-svc:
  ├─ Check Redis cache: hit? → return cached ServiceAccountIdentity
  │
  ├─ Cache miss → Query Postgres (api_keys JOIN service_accounts WHERE revoked_at IS NULL)
  │   ├─ NOT FOUND / REVOKED / EXPIRED / SA INACTIVE → 401
  │   └─ Found → populate cache (min(5 min, remaining TTL))
  │
  ├─ Update last_used_at async (non-blocking)
  └─ Return ServiceAccountIdentity {sa_id, tenant_id, roles, key_id}
```

### 6.4 API Key Revocation Flow (→ FR-15)

```
Admin → DELETE /internal/service-accounts/{id}/api-keys/{keyId}
  │
  ├─ Verify ADMIN role from JWT
  ├─ Set api_keys.revoked_at = NOW() in Postgres
  ├─ DELETE Redis cache key: apikey:{hash}     ← immediate invalidation
  │
  └─ Publish apikey.revoked audit event
```

### 6.5 Service Account Management (→ FR-09, FR-10)

```
Admin → POST /internal/service-accounts {name, description, tenant_id, roles}
  │
  ├─ Validate request (name required, roles valid set)
  ├─ Persist service_account to Postgres
  ├─ Publish admin.service_account_created audit event
  └─ Return ServiceAccount DTO

Admin → POST /internal/service-accounts/{id}/api-keys {name, expires_at?}
  │
  ├─ Validate service account exists and is active
  ├─ GenerateAPIKey(env) → (raw_key, hash, prefix)
  ├─ Persist api_key (hash + prefix, never raw)
  ├─ Publish apikey.created audit event
  └─ Return {raw_key, prefix, ...}  ← shown once, never again
```

---

## 7. Upstream Systems

| System | Role | Coupling |
|---|---|---|
| **PostgreSQL** (`banking_auth`) | Source of truth for users, tokens, service accounts, API keys | Strong — sync write path |
| **Redis** (`platform-redis`) | Cache for API key introspection; optional token store | Weak — fallback to Postgres on failure |
| **Flipt** (`platform-flipt`) | Feature flag evaluation (`maintenance_mode`) | Weak — defaults to flag-off if unavailable |
| **NATS** (`platform-nats`) | Audit event publishing | Weak — NoopPublisher fallback; never blocks handlers |

---

## 8. Downstream Systems

| System | What it receives | How |
|---|---|---|
| **account-svc** | JWT public key (deployed with service), API key introspection | Public key: static config; introspect: HTTP POST |
| **audit-svc** | Audit events (`audit.*` NATS subject) | NATS JetStream consumer (pull) |
| **Grafana / Prometheus** | Prometheus metrics scrape | HTTP GET `/metrics` |
| **Jaeger** | OpenTelemetry trace spans | OTLP gRPC push to `4317` |
| **Loki** | Structured slog output | Promtail sidecar reads container stdout |

---

## 9. Dependencies Map

```
auth-svc
├── pkg/audit          (NATS/HTTP/Noop publisher)
├── pkg/crypto         (AES-256-GCM subject encryption)
├── pkg/database       (GORM + Postgres)
├── pkg/errors         (domain error types)
├── pkg/featureflag    (Flipt client)
├── pkg/httpx          (HTTP request/response helpers)
├── pkg/idempotency    (dual-store idempotency)
├── pkg/logger         (slog configuration)
├── pkg/messaging      (NATS connection)
├── pkg/middleware     (JWT auth, API key auth, metrics, tracing, CORS, recovery)
├── pkg/observability  (OpenTelemetry bootstrap, health)
├── pkg/validator      (request validation)
├── github.com/go-chi/chi/v5
├── github.com/golang-jwt/jwt/v5
├── github.com/google/uuid
├── github.com/nats-io/nats.go
├── github.com/redis/go-redis/v9
├── gorm.io/gorm + gorm.io/driver/postgres
└── golang.org/x/crypto (bcrypt)
```

---

## 10. Risks

| ID | Risk | Impact | Mitigation |
|---|---|---|---|
| R-01 | RSA private key leakage | Critical — all tokens can be forged | Key injected via env var only; never logged; never committed |
| R-02 | AES subject key leakage | High — encrypted user IDs become decryptable | Same discipline as RSA key |
| R-03 | Introspect endpoint exposed beyond `banking-net` | High — API key hashes can be resolved by anyone | Network isolation; future: shared-secret header |
| R-04 | Redis cache staleness after revoke | Medium — revoked key usable for up to 5 min | Immediate cache deletion on revoke (mitigated) |
| R-05 | NATS outage suppresses audit trail | Medium — audit gap; no business impact | NoopPublisher fallback; NATS persists in JetStream |
| R-06 | bcrypt cost too low | High — passwords crackable offline | Cost ≥ 12 enforced in config validation |
| R-07 | Token store Redis data loss (no persistence) | Medium — all sessions invalidated on Redis restart | Redis `appendonly yes` in production; Postgres fallback |

---

## 11. Assumptions Revisited (from goals.md)

| ID | How it shapes implementation |
|---|---|
| A-01 | Single Postgres host — connection string per service uses different database name. Migration runs at startup per service. |
| A-02 | Redis assumed available — Redis store is the default; Postgres is the degraded fallback. |
| A-03 | NATS eventual delivery — audit events wrapped in `AsyncPublisher`; failures silently drop with slog warning. |
| A-04 | Flipt unavailability defaults flags off — `IsEnabled` returns `defaultVal` on error. |
| A-05 | Docker network isolation — introspect endpoint has no auth; safe only within `banking-net`. |
| A-06 | User seeding out of scope — migrations create the schema; initial users must be inserted manually or via a future admin service. |
