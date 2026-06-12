# HANDOFF — golang-banking-platform

> Last updated: 2026-06-09
> Pick up from here in a fresh conversation.

---

## Goal

Build a production-quality Go banking platform as a monorepo with:
- `services/auth-svc` — RS256 JWT issuance, refresh tokens, logout
- `services/account-svc` — account management, credit/debit, balances
- `pkg/` — shared libraries (httpx, middleware, errors, crypto, featureflag, observability, etc.)
- Full observability stack (Jaeger, Prometheus, Grafana, Loki, Alertmanager)
- Platform stack (Redis, Flipt, NATS, Metabase)
- Datasource stack (PostgreSQL, MySQL, MongoDB)

---

## Architecture

```
golang-banking-platform/
├── services/
│   ├── auth-svc/          HTTP :8082
│   └── account-svc/       HTTP :8081
├── pkg/                   shared Go workspace module
├── datasource/            docker-compose (Postgres :5432, MySQL :3306, Mongo :27017)
├── platform/              docker-compose (Redis, Flipt, NATS, Metabase)
├── monitoring/            docker-compose (Jaeger, Prometheus, Grafana, Loki, Alertmanager)
├── docker-compose.yml     microservices compose (joins banking-net)
├── prometheus.yml         Prometheus scrape config
└── Makefile               all make targets
```

### Port Convention (IMPORTANT — enforced, do not change)
| Range | Owner |
|---|---|
| **808x** | Microservices — auth-svc=8082, account-svc=8081, next=8083... |
| **900x** | Monitoring — Grafana=9000, Prometheus=9001, Alertmanager=9002, Jaeger=9003, Loki=9004, Discord relay=9005 |
| **905x** | Platform — Redis=9050, Flipt UI=9051, Flipt gRPC=9052, NATS=9053, NATS dashboard=9054, Metabase=9055 |
| **4317/4318** | OTLP (standard wire protocol, never change) |

### Network Architecture
- `banking-net` — created by `platform/docker-compose.yml`, joined by all service containers
- `datasource_datasource_net` — created by `datasource/docker-compose.yml`, joined by datasource containers
- `ds_postgres` must be manually connected to `banking-net` after datasource-up:
  ```powershell
  docker network connect banking-net ds_postgres
  ```
- Redis lives in platform compose as `platform-redis`; services reference it by container name

### Docker Compose Override Pattern
Each service has its own `.env` file for local dev. Docker-specific values are overridden in `docker-compose.yml` `environment:` block:
```yaml
env_file:
  - services/auth-svc/.env          # local dev defaults
environment:
  DB_HOST: ds_postgres               # Docker overrides
  REDIS_ADDR: platform-redis:6379
  OTEL_EXPORTER_OTLP_ENDPOINT: banking-jaeger:4317
```

---

## Current Progress

### ✅ Completed

**Infrastructure**
- All three stacks (datasource, platform, monitoring) running and healthy
- Port scheme enforced across all compose files and prometheus.yml
- `banking-net` external network wiring documented
- Makefile cross-platform fixed: `SHELL := bash.exe` on Windows, `/bin/bash` on Mac
- `services-up` target works from PowerShell without bash guards

**auth-svc**
- RS256 JWT issuance (private key in auth-svc only)
- Refresh token storage: `TOKEN_STORE=postgres|redis|memory` (default: postgres)
- PostgreSQL token store with DAO + migration
- Redis token store with TTL
- Memory token store for tests
- `/auth/login`, `/auth/refresh`, `/auth/logout` endpoints
- `/auth/inspect` endpoint (local dev only, never register in prod)
- AES-256-GCM Subject claim encryption (`JWT_SUBJECT_ENCRYPTION_KEY`)
- Feature flag: `maintenance_mode` (blocks login via Flipt)
- HTTP_PORT=8082
- Service accounts + API key management (CRUD via `/internal/service-accounts/*`)
- API key format: `bp_test_<32 base62>` (non-prod) / `bp_live_<32 base62>` (prod) — 40 chars, SHA-256 hashed, raw key shown once and never stored
- `/auth/apikey/introspect` — resolves hash → ServiceAccountIdentity (used by downstream services)
- Redis cache-aside for API key lookups (`apikey:{hash}` → identity, 5-min TTL, immediate invalidation on revoke)

**account-svc**
- JWT verification (public key only)
- CreateAccount, GetAccount, GetBalance, Credit, Debit, ListAccounts
- Feature flag: `show_account_metadata`, `banking_operation_hours`
- Claims read from context (not inter-service inspect call)
- HTTP_PORT=8081
- `AuthenticateAny` middleware: accepts both `Authorization: Bearer <jwt>` and `X-API-Key: bp_*` / `Authorization: ApiKey <key>`
- API key lookup: Redis-first (`apikey:{hash}`) → HTTP introspect fallback → Redis write-back on miss
- Self-healing: Redis empty + services restart → lazy cache repopulation on first request
- `REDIS_ADDR` / `REDIS_PASSWORD` config; Redis unavailable = graceful fallback, no hard dependency

**pkg/httpx consolidation (DONE — single response style)**
- Both services deleted their local `transport/response.go` duplicate code
- All handlers now use `httpx.WriteSuccess`, `httpx.WriteCreated`, `httpx.WriteValidationError`, `httpx.WriteHTTPError`, `httpx.DecodeJSON`
- `pkg/httpx/response.go` has `WriteValidationError` that handles `validator.ValidationErrors`
- `services/account-svc/internal/transport/errors.go` — account-specific sentinel error mapper
- `services/*/internal/transport/response.go` — kept as empty stubs with comment

**Observability**
- Prometheus scraping auth-svc at `host.docker.internal:8082`, account-svc at `host.docker.internal:8081`
- Grafana provisioned with Prometheus, Alertmanager, Loki, Jaeger datasources
- Discord relay for Alertmanager → Discord notifications
- Loki + Promtail for log aggregation

### ⚠️ Known State

- Both services are running and reachable:
  - `http://localhost:8082/auth/login` — POST with `{"email":"...","password":"..."}`
  - `http://localhost:8081/v1/accounts` — requires Bearer JWT
- Prometheus alert "service down" was firing due to `auth-svc` scrape target being on wrong port (8080 instead of 8082) — **fixed in prometheus.yml**, hot-reload with `POST http://localhost:9001/-/reload`
- `ds_postgres` needs `docker network connect banking-net ds_postgres` after every `make datasource-up`

---

## What Worked

- **Go workspace in Docker**: `go work init ./pkg ./services/auth-svc` inside Dockerfile (never copy host go.work into Docker)
- **RSA key generation**: Only via Python `cryptography` library — `sed` corrupts base64 special chars. See CREDENTIALS.txt for script.
- **Alpine runtime image**: `FROM alpine:3.20` with `wget` installed — healthchecks work. Distroless has no shell.
- **`service_started` not `service_healthy`** in depends_on — healthcheck was intermittent on distroless
- **Claims from context** in account-svc: read `middleware.ClaimsFromContext(ctx)` — inter-service `/auth/inspect` only works locally
- **pkg/httpx as single response style**: generics + request_id + timestamp + domain error mapping all in one place

## What Didn't Work / Traps to Avoid

- ❌ **`go work edit -dropuse` in Docker** — fragile, silently breaks when services are added
- ❌ **`sed` to write base64 RSA keys** — corrupts special characters (+, /, =), use Python
- ❌ **Distroless runtime** — no shell, no wget, Docker healthchecks fail
- ❌ **`ds_redis` container name** — Redis lives in platform compose as `platform-redis`, not datasource
- ❌ **`SHELL := bash` on Windows without Git Bash** — WSL bash is at System32/bash.exe but it uses Linux paths; Git Bash (`bash.exe` from Git for Windows) is needed for Makefile targets that run Go/openssl commands
- ❌ **`{ echo ...; exit 1; }` in Makefile** — bash grouping, not cmd.exe compatible; use `(echo ... && exit 1)` or just remove the guard
- ❌ **Flipt host port conflict with auth-svc** — Flipt was on 8082, clashed with auth-svc; now Flipt is on 9051
- ❌ **ds_postgres not on banking-net** — datasource compose uses its own network; must `docker network connect` manually
- ❌ **`authClient` not assigned in NewAccountHandler** — constructor accepted it but forgot `authClient: authClient` in struct literal
- ❌ **Wrong AUTH_SVC_URL default** — config.go had `http://localhost:8080`, should be `http://localhost:8082`
- ❌ **`FLIPT_URL` missing from docker-compose.yml overrides** — `.env` has `localhost:9051` (correct for local dev) but inside Docker containers `localhost` is the container itself; docker-compose.yml must override with `http://platform-flipt:8080` for both services
- ❌ **Wrong Redis port in account-svc `.env`** — Redis host port is `9050` (not `6379`); inside Docker the override `platform-redis:6379` is correct (internal port), but local `.env` must use `localhost:9050`
- ❌ **`REDIS_PASSWORD` missing from account-svc** — Redis has `requirepass`; must set `REDIS_PASSWORD=redispassword` in `.env` and `${REDIS_PASSWORD}` override in docker-compose.yml (sourced from root `.env`)

---

## Immediate Next Steps

### 1. Verify services healthy end-to-end
```powershell
# Hot-reload Prometheus to clear stale alerts
Invoke-WebRequest -Method POST http://localhost:9001/-/reload

# Test auth flow
curl -X POST http://localhost:8082/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@bank.com","password":"password123"}'

# Check Prometheus targets — both should be UP
# http://localhost:9001/targets
```

### 2. Git for Windows (required for `make build`, `make test`, `make migrate-*`)
Install from https://git-scm.com — choose **"Git from the command line and 3rd-party software"** so `bash.exe` lands in PATH. Then all Makefile targets work from PowerShell.

### 3. Pending features (priority order)
- [ ] **API key cache warm-up** — on auth-svc startup, pre-populate `apikey:{hash}` in Redis for all active keys from PostgreSQL; avoids cold-start under load when both services restart simultaneously
- [ ] **Kubernetes migration** — was discussed but not started; services are Docker-only right now
- [ ] **payment-svc** — next microservice; port 8083, same Dockerfile pattern as auth-svc/account-svc
- [ ] **notification-svc** — consumes NATS events from account-svc; port 8084
- [ ] **Rate limiting middleware** — `pkg/middleware` has the interface, wire it into routers
- [ ] **Integration tests** — `services/*/tests/integration/` scaffold exists, no tests written yet
- [ ] **k6 performance tests** — `performance-test-k6/` scripts exist, run with `make k6-smoke`

### 4. Adding a new microservice (pattern)
1. `cp -r services/account-svc services/new-svc`
2. Port: next available 808x (e.g., 8083)
3. Dockerfile: same `go work init ./pkg ./services/new-svc` pattern
4. Add to `docker-compose.yml` following account-svc block
5. Add scrape target to `prometheus.yml` at `host.docker.internal:808x`
6. Add port to the port scheme table at top of this file

---

## Key File Locations

| File | Purpose |
|---|---|
| `services/auth-svc/.env` | auth-svc local dev config (HTTP_PORT=8082, TOKEN_STORE, JWT keys) |
| `services/account-svc/.env` | account-svc local dev config (HTTP_PORT=8081, JWT public key) |
| `CREDENTIALS.txt` | RSA keypair + Python regeneration script |
| `docker-compose.yml` | microservices stack |
| `platform/docker-compose.yml` | Redis, Flipt, NATS, Metabase |
| `monitoring/docker-compose.infra.yml` | Jaeger, Prometheus, Grafana, Loki, Alertmanager |
| `datasource/docker-compose.yml` | Postgres, MySQL, MongoDB |
| `prometheus.yml` | Prometheus scrape targets + alertmanager connection |
| `pkg/httpx/` | canonical HTTP response helpers (all services use this) |
| `pkg/errors/` | domain error types (IsNotFound, IsConflict, etc.) |
| `pkg/middleware/` | JWT auth middleware, claims context, request ID, logger |

---

## Make Targets Quick Reference

```powershell
make datasource-up     # Start Postgres, MySQL, MongoDB
make platform-up       # Start Redis, Flipt, NATS, Metabase
make monitoring-up     # Start Jaeger, Prometheus, Grafana, Loki, Alertmanager
make services-up       # Build + start auth-svc, account-svc
make stack-up          # All of the above
make stack-down        # Stop everything
make services-logs     # Tail microservice logs
make migrate           # Run all SQL migrations (requires Git Bash)
make gen-keys          # Generate new RSA keypair (requires Git Bash + openssl)
```
