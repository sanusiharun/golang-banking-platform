# progress-tracking.md — auth-svc

> **Purpose:** Convert architecture and context into actionable work. Every item traces back to [goals.md](goals.md), [context.md](context.md), and [architecture.md](architecture.md).

---

## Legend

| Symbol | Meaning |
|---|---|
| ✅ | Complete |
| 🔄 | In progress |
| ⬜ | Not started |
| ⚠️ | Blocked |
| 🔧 | Technical debt |

---

## Epic 1 — Foundation & Infrastructure

> Satisfies: NFR-13, NFR-16, NFR-17, C-01 to C-07

| ID | Task | Status | Notes |
|---|---|---|---|
| E1-T01 | Go workspace configured (`go.work`) | ✅ | `pkg` + 3 services |
| E1-T02 | `pkg/` shared module scaffolded | ✅ | All packages present |
| E1-T03 | `banking_auth` Postgres database provisioned | ✅ | `datasource/postgres/02_setup_banking_auth.sql` |
| E1-T04 | DB migrations embedded and run at startup | ✅ | `migrations.go` + `cmd/server/migrate.go` |
| E1-T05 | `.env.example` documented with all required vars | ✅ | All vars with comments |
| E1-T06 | Docker Compose integration (banking-net, env overrides) | ✅ | `docker-compose.yml` |
| E1-T07 | Dockerfile (multi-stage, distroless-style alpine) | ✅ | `services/auth-svc/Dockerfile` |
| E1-T08 | Global slog logger configured with OTEL context | ✅ | `pkg/logger/` |
| E1-T09 | OpenTelemetry bootstrap (tracer, metrics, logs) | ✅ | `pkg/observability/otel.go` |
| E1-T10 | Graceful shutdown (30 s drain, NATS flush) | ✅ | `cmd/server/main.go` |

---

## Epic 2 — Human Authentication (→ FR-01 to FR-08, BO-01)

| ID | Task | Status | Notes |
|---|---|---|---|
| E2-T01 | `User` DAO + migration `001_create_users` | ✅ | `internal/domain/dao/user.go` |
| E2-T02 | `UserRepository.FindByUsername` | ✅ | `internal/repository/user_repository.go` |
| E2-T03 | `RefreshToken` DAO + migration `004_create_refresh_tokens` | ✅ | `internal/domain/dao/refresh_token.go` |
| E2-T04 | `TokenStore` interface | ✅ | `internal/repository/token_store.go` |
| E2-T05 | `PostgresTokenStore` implementation | ✅ | `internal/repository/token_store_postgres.go` |
| E2-T06 | `RedisTokenStore` implementation | ✅ | `internal/repository/token_store_redis.go` |
| E2-T07 | `MemoryTokenStore` implementation (testing) | ✅ | `internal/repository/token_store_memory.go` |
| E2-T08 | `AuthService.Login` (bcrypt, JWT, refresh token) | ✅ | `internal/services/auth_service.go` |
| E2-T09 | Timing-safe dummy bcrypt for unknown users | ✅ | Part of `AuthService.Login` |
| E2-T10 | AES-256-GCM Subject encryption | ✅ | `pkg/crypto/cipher.go` |
| E2-T11 | `AuthService.Refresh` (rotate token pair) | ✅ | `internal/services/auth_service.go` |
| E2-T12 | `AuthService.Logout` (revoke refresh token) | ✅ | `internal/services/auth_service.go` |
| E2-T13 | `AuthHandler` (login, refresh, logout routes) | ✅ | `internal/transport/auth_handler.go` |
| E2-T14 | Flipt `maintenance_mode` gate in `Login` | ✅ | `internal/services/auth_service.go` |
| E2-T15 | Unit tests: `AuthService` | ✅ | `internal/services/auth_service_test.go` |
| E2-T16 | Integration tests: token store (Postgres + Redis) | ⬜ | `tests/integration/` — not yet written |

---

## Epic 3 — Service Accounts & API Keys (→ FR-09 to FR-16, BO-02)

| ID | Task | Status | Notes |
|---|---|---|---|
| E3-T01 | `ServiceAccount` DAO + migration `005_create_service_accounts` | ✅ | `internal/domain/dao/service_account.go` |
| E3-T02 | `APIKey` DAO + migration `006_create_api_keys` | ✅ | `internal/domain/dao/api_key.go` |
| E3-T03 | Partial index `idx_api_keys_active_hash` for hot path | ✅ | `migrations/006_create_api_keys.up.sql` |
| E3-T04 | `APIKeyStore` interface | ✅ | `internal/repository/apikey_store.go` |
| E3-T05 | `PostgresAPIKeyStore` (JOIN query, partial index) | ✅ | `internal/repository/apikey_store_postgres.go` |
| E3-T06 | `RedisAPIKeyStore` (cache-aside wrapper) | ✅ | `internal/repository/apikey_store_redis.go` |
| E3-T07 | `APIKeyService` CRUD for service accounts | ✅ | `internal/services/apikey_service.go` |
| E3-T08 | `APIKeyService.CreateAPIKey` (generate, hash, return raw once) | ✅ | `internal/services/apikey_service.go` |
| E3-T09 | `APIKeyService.RevokeAPIKey` (Postgres + Redis invalidation) | ✅ | `internal/services/apikey_service.go` |
| E3-T10 | `APIKeyService.IntrospectAPIKey` (cache-aside, async last_used) | ✅ | `internal/services/apikey_service.go` |
| E3-T11 | `pkg/middleware/apikey.go` (`AuthenticateAPIKey`, `AuthenticateAny`) | ✅ | Shared middleware |
| E3-T12 | `APIKeyHandler` (admin CRUD routes) | ✅ | `internal/transport/apikey_handler.go` |
| E3-T13 | `POST /auth/apikey/introspect` handler | ✅ | `internal/transport/apikey_handler.go` |
| E3-T14 | Unit tests: `APIKeyService` | ✅ | `internal/services/apikey_service_test.go` |
| E3-T15 | Integration tests: `RedisAPIKeyStore` | ✅ | `internal/repository/apikey_store_integration_test.go` |
| E3-T16 | Introspect endpoint security: shared-secret header (`SERVICE_SECRET` / `X-Service-Secret`) | ✅ | `pkg/middleware/sharedsecret.go` |
| E3-T17 | API key cache warm-up on startup | ⬜ | Nice-to-have for cold-start latency |

---

## Epic 4 — Middleware & Cross-Cutting (→ FR-20, FR-21, FR-24, NFR-01 to NFR-09)

| ID | Task | Status | Notes |
|---|---|---|---|
| E4-T01 | `pkg/middleware/auth.go` — JWT validation, RBAC | ✅ | RS256, decrypt Subject |
| E4-T02 | `pkg/middleware/metrics.go` — Prometheus histogram | ✅ | Per-endpoint labels |
| E4-T03 | `pkg/middleware/tracing.go` — OTEL spans | ✅ | Per-request span |
| E4-T04 | `pkg/middleware/logger.go` — structured request logging | ✅ | slog with context |
| E4-T05 | `pkg/middleware/recovery.go` — panic recovery | ✅ | Returns 500, logs stack |
| E4-T06 | `pkg/middleware/requestid.go` — request ID propagation | ✅ | Added to response headers |
| E4-T07 | `pkg/middleware/cors.go` | ✅ | Configurable origins |
| E4-T08 | `pkg/middleware/ratelimit.go` | ✅ | Token bucket (Redis-backed) |
| E4-T09 | `pkg/middleware/timeout.go` | ✅ | Per-request deadline |
| E4-T10 | `pkg/idempotency/` dual-store | ✅ | Redis + Postgres |
| E4-T11 | Idempotency migration `007_create_idempotency_requests` | ✅ | Wired in container |
| E4-T12 | `pkg/middleware/idempotency.go` — middleware | ✅ | `Idempotency-Key` header |
| E4-T13 | Unit tests: `pkg/middleware/auth_test.go` | ✅ | |
| E4-T14 | Unit tests: `pkg/middleware/apikey_test.go` | ✅ | |

---

## Epic 5 — Observability (→ FR-22, FR-23, NFR-18, BO-04)

| ID | Task | Status | Notes |
|---|---|---|---|
| E5-T01 | `pkg/audit/` publisher interface + implementations | ✅ | NATS, HTTP, Noop, Async |
| E5-T02 | Audit events on login, refresh, logout | ✅ | `auth_handler.go` |
| E5-T03 | Audit events on API key create, revoke, introspect | ✅ | `apikey_handler.go` |
| E5-T04 | Audit events on service account CRUD | ✅ | `apikey_handler.go` |
| E5-T05 | `/healthz/live` + `/healthz/ready` | ✅ | `pkg/observability/health.go` |
| E5-T06 | `/metrics` Prometheus endpoint | ✅ | chi route in `routes.go` |
| E5-T07 | Prometheus scrape config for auth-svc | ✅ | `prometheus.yml` |
| E5-T08 | Alerting rules: `monitoring/alerting/rules/auth-svc.yml` | ✅ | Created |
| E5-T09 | Grafana dashboard for auth-svc | 🔄 | Provisioning folder exists; dashboard JSON pending |
| E5-T10 | Loki / Promtail log collection | ✅ | Docker logging driver config |
| E5-T11 | Audit-svc DB migration run | ⬜ | **HANDOFF.md blocker** — `04_setup_banking_audits.sql` |
| E5-T12 | Fix logout `ActorID`: use `UserIDFromContext()` | ⬜ | **HANDOFF.md bug** |

---

## Epic 6 — Testing & Quality

| ID | Task | Status | Notes |
|---|---|---|---|
| E6-T01 | Unit: `AuthService` | ✅ | |
| E6-T02 | Unit: `APIKeyService` | ✅ | |
| E6-T03 | Unit: `pkg/middleware/auth` | ✅ | |
| E6-T04 | Unit: `pkg/middleware/apikey` | ✅ | |
| E6-T05 | Unit: `pkg/idempotency/dual_store` | ✅ | |
| E6-T06 | Integration: `APIKeyStore` (Redis + Postgres) | ✅ | `apikey_store_integration_test.go` |
| E6-T07 | Integration: `TokenStore` (Postgres + Redis) | ⬜ | `tests/integration/` — scaffolded, not written |
| E6-T08 | k6 smoke test: login + refresh + logout | ⬜ | `performance-test-k6/` — scaffolded |
| E6-T09 | k6 load test: API key introspection (500 VU) | ⬜ | Required for NFR-03 verification |
| E6-T10 | `.golangci.yml` lint checks pass | ✅ | Config present |

---

## Epic 7 — Planned / Future Work

| ID | Task | Priority | Rationale |
|---|---|---|---|
| E7-T01 | `payment-svc` scaffold on port 8084 | High | HANDOFF.md — next service |
| E7-T02 | Introspect endpoint shared-secret auth | High | Security — R-03 in context.md |
| E7-T03 | User create/update admin endpoint | Medium | A-06 in context.md |
| E7-T04 | RBAC fine-grained permission model | Medium | Roles are coarse today (ADMIN / USER) |
| E7-T05 | API key cache warm-up at startup | Low | NFR-03 cold-start gap |
| E7-T06 | Redis persistence (`appendonly yes`) for token store | Medium | R-07 in context.md |
| E7-T07 | Grafana auth-svc dashboard JSON | Medium | E5-T09 above |

---

## Dependency Graph

```
E1 (Foundation)
  └─► E2 (Human Auth) ──► E6 (Testing)
  └─► E3 (API Keys) ───► E6
  └─► E4 (Middleware) ──► E2, E3
  └─► E5 (Observability) ─► E2, E3, E7
```

---

## Current Blockers

| Blocker | Affects | Owner | Resolution |
|---|---|---|---|
| `04_setup_banking_audits.sql` migration not run | E5-T11, audit-svc end-to-end | Ops | Run manually or wire into datasource bootstrap |

---

## Technical Debt Register

| ID | Description | Severity | Linked Task |
|---|---|---|---|
| TD-01 | Introspect endpoint accessible to any container on banking-net without auth | High | E3-T16 |
| TD-02 | Logout ActorID is refresh token string, not user ID | Medium | E5-T12 |
| TD-03 | Integration tests for token stores not written | Medium | E6-T07 |
| TD-04 | k6 load tests not executed | Medium | E6-T08, E6-T09 |
| TD-05 | Redis token store has no persistence config reminder | Low | E7-T06 |
| TD-06 | Grafana auth-svc dashboard JSON not committed | Low | E7-T07 |
