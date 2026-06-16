# review.md — auth-svc

> **Purpose:** Evaluate auth-svc against the requirements and architecture defined in [goals.md](goals.md), [context.md](context.md), and [architecture.md](architecture.md). Every finding references the original requirement.

---

## 1. Requirement Compliance

### 1.1 Functional Requirements

| FR ID | Requirement | Status | Evidence / Finding |
|---|---|---|---|
| FR-01 | Login returns JWT + refresh token | ✅ Pass | `AuthService.Login` issues RS256 JWT + UUID refresh token |
| FR-02 | Access token expires ≤ 15 min | ✅ Pass | `JWT_EXPIRY_MINUTES` default 15; enforced in token Claims |
| FR-03 | Refresh token single-use, 7-day TTL | ✅ Pass | Revoked atomically on first use in `AuthService.Refresh` |
| FR-04 | Refresh endpoint issues new pair | ✅ Pass | New JWT + refresh token returned; old token revoked |
| FR-05 | Logout revokes refresh token | ✅ Pass | `TokenStore.Revoke(hash)` called in `AuthService.Logout` |
| FR-06 | Maintenance mode blocks login (503) | ✅ Pass | Flipt gate in `AuthService.Login`; 503 returned |
| FR-07 | Timing-safe for unknown usernames | ✅ Pass | Dummy bcrypt always runs; response timing indistinguishable |
| FR-08 | Inactive users rejected | ✅ Pass | `user.is_active` check after bcrypt |
| FR-09 | Admin CRUD for service accounts | ✅ Pass | `APIKeyService` + `APIKeyHandler` admin routes |
| FR-10 | Admin create/revoke API keys | ✅ Pass | `CreateAPIKey`, `RevokeAPIKey` in service + handler |
| FR-11 | Raw API key shown once, never stored | ✅ Pass | Only hash + prefix persisted; raw returned in create response |
| FR-12 | Key prefix derivable without raw key | ✅ Pass | `key_prefix` column stores first 10 chars |
| FR-13 | Machine client auth via API key | ✅ Pass | `AuthenticateAPIKey` middleware in `pkg/middleware/apikey.go` |
| FR-14 | API key lookup cache-optimised | ✅ Pass | Redis cache-aside in `RedisAPIKeyStore` |
| FR-15 | Revoke invalidates cache immediately | ✅ Pass | `RevokeAPIKey` deletes Redis key before returning |
| FR-16 | API key expiry enforced at lookup | ✅ Pass | `expires_at` checked in both Postgres query and Redis deserialization |
| FR-17 | Token store: Postgres + memory backends | ✅ Pass | Both implemented; selected via `TOKEN_STORE` env var |
| FR-18 | Token store: Redis backend | ✅ Pass | `RedisTokenStore` implemented |
| FR-19 | All token stores enforce expiry + revocation | ✅ Pass | Each impl handles independently |
| FR-20 | Prometheus metrics on all endpoints | ✅ Pass | `pkg/middleware/metrics.go` wraps every route |
| FR-21 | OTEL spans on all endpoints | ✅ Pass | `pkg/middleware/tracing.go` + service-level spans |
| FR-22 | Audit events published (non-blocking) | ✅ Pass | Fire-and-forget goroutines in all handlers |
| FR-23 | `/healthz/live` + `/healthz/ready` | ✅ Pass | `pkg/observability/health.go` |
| FR-24 | Idempotency support | ✅ Pass | `pkg/idempotency` dual-store + middleware |

**Functional requirement compliance: 24 / 24 (100%)**

### 1.2 Non-Functional Requirements

| NFR ID | Requirement | Status | Evidence / Finding |
|---|---|---|---|
| NFR-01 | P99 login ≤ 500 ms | ⬜ Unverified | k6 load tests not yet run (TD-04) |
| NFR-02 | P99 refresh ≤ 100 ms | ⬜ Unverified | k6 load tests not yet run |
| NFR-03 | P99 introspect (cache hit) ≤ 10 ms | ⬜ Unverified | k6 load tests not yet run |
| NFR-04 | P99 introspect (cache miss) ≤ 50 ms | ⬜ Unverified | k6 load tests not yet run |
| NFR-05 | Availability ≥ 99.9% | ⬜ Unverified | No SLO measurement in place yet |
| NFR-06 | RS256; private key never leaves auth-svc | ✅ Pass | Key injected via env; account-svc has only public key |
| NFR-07 | bcrypt cost ≥ 12 | ✅ Pass | Validated in `config.go` |
| NFR-08 | JWT Subject encrypted (AES-256-GCM) | ✅ Pass | `pkg/crypto/cipher.go`; decrypted in middleware |
| NFR-09 | Tokens hashed (SHA-256); raw never persisted | ✅ Pass | `token_hash`, `key_hash` columns in schema |
| NFR-10 | API key environment prefix | ✅ Pass | `bp_live_` / `bp_test_` prefix; validated on use |
| NFR-11 | Stateless above storage | ✅ Pass | No in-process session state; all state in Redis/Postgres |
| NFR-12 | Feature flags changeable without deploy | ✅ Pass | Flipt evaluated per-request |
| NFR-13 | DB migrations at startup | ✅ Pass | `cmd/server/migrate.go` |
| NFR-14 | Audit failure never affects endpoints | ✅ Pass | Fire-and-forget goroutine; no error propagation |
| NFR-15 | Redis unavailability → Postgres fallback | ✅ Pass | Both stores have error-fallback paths |
| NFR-16 | Business logic in service layer | ✅ Pass | Handlers are thin; all logic in `internal/services/` |
| NFR-17 | Shared helpers in `pkg/`; no circular deps | ✅ Pass | `pkg/` imports nothing from services |
| NFR-18 | Full audit trail | ✅ Pass | All auth + credential events published |

**Non-functional: 14 verified pass, 5 unverified (latency/availability — pending load tests)**

---

## 2. Architecture Compliance

| Decision | Status | Finding |
|---|---|---|
| Transport → Service → Repository → DAO layering | ✅ | Strictly enforced; no layer skipping observed |
| `pkg/` never imports services | ✅ | Verified by Go workspace module boundaries |
| Services communicate via HTTP only | ✅ | No shared memory or gRPC between services |
| Database-per-service | ✅ | auth-svc only touches `banking_auth` schema |
| `pkg/httpx` for all HTTP responses | ✅ | No local `response.go` found in transport |
| `slog` only, no `fmt.Println` | ✅ | Grep confirms zero `fmt.Println` in service code |
| Audit via fire-and-forget goroutine | ✅ | Pattern consistent across all handlers |
| No heavy frameworks (Gin/Echo/Fiber) | ✅ | chi + stdlib only |

---

## 3. Code Quality

### Strengths

- **Pluggable backends** — `TokenStore` and `APIKeyStore` interfaces enable clean testing without mocks against production stores.
- **Security depth** — timing-safe bcrypt, AES-256-GCM subject encryption, SHA-256 hashing, environment-aware key prefixes all in place.
- **Fail-fast config** — required env vars validated at startup; service refuses to start with missing credentials.
- **Graceful shutdown** — 30 s drain ensures in-flight requests complete before process exit.
- **Observability from the start** — every request has a trace, metrics, and structured log entry from day one.

### Issues

| Severity | Location | Finding | Reference |
|---|---|---|---|
| High | `apikey_handler.go` logout path | `ActorID` set to refresh token string, not user ID | TD-02 |
| High | `internal/transport/apikey_handler.go` | `POST /auth/apikey/introspect` has no authentication beyond network isolation | TD-01 |
| Medium | `tests/integration/` | Integration tests for `TokenStore` (Postgres + Redis) not written | TD-03 |
| Medium | `performance-test-k6/` | k6 load tests not executed — NFR-01 to NFR-04 unverified | TD-04 |
| Low | `monitoring/grafana/` | Grafana auth-svc dashboard JSON not committed | TD-06 |

---

## 4. Maintainability

| Dimension | Assessment |
|---|---|
| **Naming conventions** | ✅ Consistent: `Postgres{Type}`, `New{Type}`, noun interfaces |
| **Import order** | ✅ goimports enforced via `.golangci.yml` |
| **Comment density** | ✅ Appropriately sparse; comments only where non-obvious |
| **Handler size** | ✅ All handlers ≤ 30 lines of logic; thin by design |
| **Test coverage (unit)** | ✅ Core services have table-driven unit tests |
| **Test coverage (integration)** | ⚠️ Incomplete — token store integration tests missing |
| **Error wrapping** | ✅ `fmt.Errorf("repo.Method: %w", err)` pattern consistent |
| **Domain errors** | ✅ `pkg/errors.NewNotFound`, `.NewUnauthorized` etc. throughout |

---

## 5. Operational Readiness

| Dimension | Status | Detail |
|---|---|---|
| Health checks | ✅ Ready | Liveness + readiness; Postgres + Redis checked |
| Metrics | ✅ Ready | Prometheus histogram per endpoint |
| Tracing | ✅ Ready | OTEL → Jaeger; spans on hot paths |
| Log aggregation | ✅ Ready | slog → Loki via Promtail |
| Alerting rules | ✅ Ready | `monitoring/alerting/rules/auth-svc.yml` committed |
| Feature flag control | ✅ Ready | `maintenance_mode` toggleable in Flipt without deploy |
| Graceful shutdown | ✅ Ready | 30 s drain on SIGTERM |
| DB migrations | ✅ Ready | Runs at startup automatically |
| Dashboard | ⚠️ Partial | Grafana datasources provisioned; dashboard JSON missing |
| Runbook | ⬜ Missing | No incident runbook documented |
| SLO definition | ⬜ Missing | NFR targets exist in goals.md but not wired to Grafana SLO panels |

---

## 6. Security Posture

| Control | Status | Finding |
|---|---|---|
| Password storage (bcrypt ≥ 12) | ✅ | Enforced and validated at startup |
| JWT signing (RS256) | ✅ | Private key never leaves auth-svc |
| Subject encryption (AES-256-GCM) | ✅ | Downstream services receive encrypted Subject |
| Refresh token hygiene (hash only) | ✅ | Raw value only in HTTP response, never persisted |
| API key hygiene (hash + prefix only) | ✅ | Raw value returned once at creation |
| Timing-safe authentication | ✅ | Dummy bcrypt always runs for unknown usernames |
| Introspect endpoint isolation | ⚠️ | No auth beyond Docker network; R-03 risk not fully mitigated |
| Rate limiting | ✅ | `pkg/middleware/ratelimit.go` in place |
| CORS | ✅ | Configured via `pkg/middleware/cors.go` |
| Secrets not committed | ✅ | `.gitignore` covers `.env`, `CREDENTIALS.txt` |
| Key leakage in logs | ✅ | No key values appear in slog output (confirmed by grep) |

**Security finding to address:** `POST /auth/apikey/introspect` relies solely on Docker network isolation. A compromised container on `banking-net` can resolve any API key hash. Recommend adding a shared-secret header (e.g. `X-Internal-Token`) checked against an env var. Tracked as TD-01.

---

## 7. Reliability Assessment

| Scenario | Behaviour | Verdict |
|---|---|---|
| Redis down | Postgres fallback for token + API key stores | ✅ Resilient |
| Flipt down | `maintenance_mode` defaults false; logins continue | ✅ Resilient |
| NATS down | NoopPublisher fallback; audit events lost until recovery | ✅ Acceptable (audit is async, not transactional) |
| Postgres down | 503; readiness probe fails; load balancer removes instance | ✅ Correct failure mode |
| Auth-svc crash | Graceful shutdown drains; NATS flushed | ✅ Clean exit |
| Redis token store restart without persistence | All sessions invalidated; users forced to re-login | ⚠️ Acceptable for dev; production needs `appendonly yes` |

---

## 8. Technical Debt Summary

| ID | Description | Severity | Recommended Action |
|---|---|---|---|
| TD-01 | Introspect endpoint no auth beyond network | High | Add `X-Internal-Token` header validation |
| TD-02 | Logout ActorID = refresh token, not user ID | Medium | Replace with `UserIDFromContext(ctx)` |
| TD-03 | Token store integration tests missing | Medium | Write table-driven integration tests with testcontainers or real DB |
| TD-04 | k6 load tests not run | Medium | Run `make k6-smoke`; establish baseline for NFR-01 to NFR-04 |
| TD-05 | Redis persistence not configured | Low | Document `appendonly yes` in prod deployment notes |
| TD-06 | Grafana dashboard JSON missing | Low | Export and commit dashboard JSON to provisioning folder |

---

## 9. Risks

| ID | Risk | Current Mitigation | Recommended Improvement |
|---|---|---|---|
| R-01 | RSA private key leakage | Env var injection; gitignored | Rotate to GCP Secret Manager or Vault in production |
| R-02 | AES subject key leakage | Env var injection | Same as R-01 |
| R-03 | Introspect endpoint exposed within banking-net | Docker network isolation | Shared-secret header (TD-01) |
| R-04 | Redis cache staleness | Immediate delete on revoke | Already mitigated; no further action |
| R-05 | Audit trail gap during NATS outage | NoopPublisher + JetStream reconnect | Add dead-letter persistent queue for missed events |
| R-07 | Redis token data loss on restart | Not configured | Enable `appendonly yes`; or switch to Postgres store in production |

---

## 10. Recommendations

### Immediate (before production)

1. **Fix TD-01** — Add shared-secret validation to `/auth/apikey/introspect`. One-line env var check.
2. **Fix TD-02** — Replace logout `ActorID` with `UserIDFromContext(ctx)`.
3. **Run audit DB migration** — `04_setup_banking_audits.sql` (HANDOFF.md blocker).
4. **Enable Redis persistence** — `appendonly yes` in `platform/docker-compose.yml` for production deployments.

### Short-term (next sprint)

5. **Write integration tests** for `TokenStore` (TD-03).
6. **Run k6 load tests** and capture baseline for NFR-01 to NFR-04 (TD-04).
7. **Commit Grafana dashboard JSON** (TD-06).

### Medium-term

8. **Move secrets to GCP Secret Manager or HashiCorp Vault** for production (R-01, R-02).
9. **Add user management endpoints** (A-06 assumption in context.md).
10. **Define SLOs in Grafana** using NFR-01 to NFR-05 as targets.
11. **Write incident runbook** covering: Redis outage, NATS outage, key rotation procedure, emergency maintenance mode activation.

---

## 11. Refactoring Opportunities

| Opportunity | Benefit | Risk |
|---|---|---|
| Extract `LoginResponse` building into a private helper | Reduces duplication between `Login` and `Refresh` | Low |
| Add context-aware key rotation support | Enables zero-downtime RSA key rotation | Medium (requires versioned token header) |
| Replace manual Redis JSON marshal/unmarshal with typed codec | Reduces risk of cache deserialization bugs | Low |
| Add `testcontainers-go` for integration tests | Removes dependency on running Docker in CI | Low |
