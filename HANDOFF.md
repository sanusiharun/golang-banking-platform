# HANDOFF — golang-banking-platform

> Last updated: 2026-06-19
> Read this first in every new session. CLAUDE.md covers coding conventions only.

---

## Project Goal

Production-grade Go banking platform monorepo:
- `auth-svc` (8082) — RS256 JWT, refresh tokens, logout, service accounts, API keys
- `account-svc` (8081) — account CRUD, credit/debit, balance
- `audit-svc` (8083) — NATS consumer → Postgres audit log (scaffolded, not wired yet)
- `notification-svc` (8084) — centralised notification platform: multi-channel delivery, templates, scheduler
- `pkg/` — shared Go workspace module (httpx, middleware, audit, errors, observability, etc.)

---

## Repo Structure

```
golang-banking-platform/
├── services/
│   ├── auth-svc/              HTTP :8082
│   ├── account-svc/           HTTP :8081
│   ├── audit-svc/             HTTP :8083 (scaffolded)
│   └── notification-svc/      HTTP :8084 (multi-channel, templates, scheduler)
├── pkg/                   shared Go module (github.com/sanusi/banking/pkg)
├── datasource/            docker-compose — Postgres :5432, MySQL :3306, Mongo :27017
├── platform/              docker-compose — Redis :9050, Flipt :9051/:9052, NATS :9053, Metabase :9055
├── monitoring/            docker-compose — Grafana :9000, Prometheus :9001, Alertmanager :9002, Jaeger :9003, Loki :9004
├── docker-compose.yml     microservices (joins banking-net)
├── prometheus.yml         Prometheus scrape config
├── CREDENTIALS.txt        RSA keys + all secrets (gitignored — local only)
└── Makefile
```

---

## Port Scheme (enforced — never change)

| Range | Owner |
|---|---|
| `808x` | Microservices: auth-svc=8082, account-svc=8081, audit-svc=8083, notification-svc=8084, next=8085… |
| `900x` | Monitoring: Grafana=9000, Prometheus=9001, Alertmanager=9002, Jaeger=9003, Loki=9004, Discord=9005 |
| `905x` | Platform: Redis=9050, Flipt UI=9051, Flipt gRPC=9052, NATS=9053, NATS UI=9054, Metabase=9055 |
| `4317/4318` | OTLP — standard wire protocol, never change |

---

## Fresh Laptop Setup

### Prerequisites
- Go 1.25+
- Docker Desktop
- Git for Windows (provides `bash.exe` — required for Makefile targets): https://git-scm.com → choose "Git from the command line and 3rd-party software"
- `CREDENTIALS.txt` — copy from your other machine (gitignored, contains all RSA keys + passwords)

### First time only — get secrets
Copy `CREDENTIALS.txt` from your other laptop (USB / private share). It contains:
- RSA private key (`JWT_PRIVATE_KEY_B64`) for auth-svc
- RSA public key (`JWT_PUBLIC_KEY_B64`) for account-svc and audit-svc
- AES key (`JWT_SUBJECT_ENCRYPTION_KEY`) for subject encryption
- All DB passwords and Redis password
- Python script to regenerate keys if needed

Then copy the values into each service's `.env` file — templates are in each service's `.env.example`.

### Start the full stack
```powershell
make datasource-up     # Postgres, MySQL, MongoDB
# then connect Postgres to banking-net (required after every datasource-up):
docker network connect banking-net ds_postgres

make platform-up       # Redis, Flipt, NATS, Metabase
make monitoring-up     # Jaeger, Prometheus, Grafana, Loki, Alertmanager
make services-up       # Build + start auth-svc, account-svc

# Or all at once (does NOT run the network connect step):
make stack-up
```

### Verify everything is healthy
```powershell
# Service health
curl http://localhost:8082/healthz/ready
curl http://localhost:8081/healthz/ready

# Auth flow
curl -X POST http://localhost:8082/auth/login `
  -H "Content-Type: application/json" `
  -d '{"email":"admin@bank.com","password":"password123"}'

# Prometheus targets — all should be UP
# http://localhost:9001/targets

# Grafana dashboards
# http://localhost:9000  (admin / admin)
```

---

## Network Architecture

- `banking-net` — created by `platform/docker-compose.yml`, joined by all service containers
- `datasource_datasource_net` — internal to datasource compose
- `ds_postgres` must be manually connected to `banking-net` after every `make datasource-up`:
  ```powershell
  docker network connect banking-net ds_postgres
  ```
- Redis container name inside Docker: `platform-redis` (port 6379 internally, 9050 on host)

## Docker Compose Override Pattern
```yaml
env_file:
  - services/auth-svc/.env        # local dev defaults (localhost URLs)
environment:
  DB_HOST: ds_postgres             # Docker overrides (container names)
  REDIS_ADDR: platform-redis:6379
  FLIPT_URL: http://platform-flipt:8080
  OTEL_EXPORTER_OTLP_ENDPOINT: banking-jaeger:4317
```

---

## What's Done

### auth-svc
- RS256 JWT issuance (private key here only), refresh, logout
- Token stores: `TOKEN_STORE=postgres|redis|memory`
- AES-256-GCM subject encryption (`JWT_SUBJECT_ENCRYPTION_KEY`)
- Feature flag: `maintenance_mode` via Flipt
- Service accounts + API key CRUD (`/internal/service-accounts/*`)
- API key format: `bp_test_<32 base62>` / `bp_live_<32 base62>` — SHA-256 hashed, raw shown once
- `POST /auth/apikey/introspect` — resolves hash → `ServiceAccountIdentity` for downstream services
- Redis cache-aside for API keys: `apikey:{hash}` → identity, 5-min TTL, invalidated on revoke
- Async audit publishing via NATS → NoopPublisher fallback

### account-svc
- JWT verification (public key only), CreateAccount, GetAccount, GetBalance, Credit, Debit, ListAccounts
- `AuthenticateAny` middleware: accepts `Authorization: Bearer <jwt>` and `X-API-Key: bp_*`
- API key lookup: Redis-first → HTTP introspect fallback → write-back on miss (self-healing)
- Feature flags: `show_account_metadata`, `banking_operation_hours`
- Async audit publishing via NATS → NoopPublisher fallback

### audit-svc (port 8083 — fully wired)
- NATS consumer → `banking_audits` Postgres DB
- HTTP query API for audit events
- Wired into `docker-compose.yml`, `prometheus.yml`, alert rules, and Promtail
- `waitForNATS()` in container.go — polls `nc.IsConnected()` before calling `EnsureStream` so standalone (`make run-audit-svc`) doesn't fail when NATS hasn't fully connected yet
- Migration still needs to run: `datasource/postgres/04_setup_banking_audits.sql` + `001_create_audit_events.up.sql`

### pkg/
- `pkg/audit` — `Publisher` interface, `NATSPublisher`, `NoopPublisher`, `HTTPPublisher`
- `pkg/httpx` — canonical response helpers (all services use this, no local duplicates)
- `pkg/middleware` — JWT auth, `AuthenticateAny`, `RequireRole`, rate limit, tracing, metrics
- `pkg/errors`, `pkg/observability`, `pkg/featureflag`, `pkg/idempotency`

### Observability
- Prometheus scrapes all three services via `host.docker.internal:808x`
- `/metrics` and `/healthz/*` excluded from HTTP metrics and request logs in all services
- Grafana Service Logs panel filters `/metrics` and `/healthz/*` at Loki query level
- Grafana: Prometheus, Loki, Jaeger, Alertmanager datasources provisioned
- Discord relay: Alertmanager → Discord notifications
- Loki + Promtail: log aggregation from Docker containers and local `./logs/*.log`

### notification-svc

- Full 5-document lifecycle written (reverse-engineered from code, 2026-06-19)
- `docs/goals.md` — 17 FR, 9 NFR, 10 constraints, 17 acceptance criteria, service boundaries
- `docs/context.md` — domain overview, bounded context diagram, 6 business workflows, actor table, risk register, upstream/downstream systems
- `docs/architecture.md` — Mermaid diagrams (high-level + sequence), layering rules, package tree, SQL DDL for 3 tables, full API table, security/observability/reliability design
- `docs/progress-tracking.md` — 7 epics, 48 tasks (most ✅), 10 tech debt items (TD-01–TD-10)
- `docs/review.md` — FR compliance (16/17 pass, FR-02 partial — stubs), NFR (3 unverified: latency/throughput), 10 TD items, immediate/short/medium recommendations
- Key gaps: EMAIL/SMS/PUSH/WHATSAPP are stubs (TD-01–TD-04); no integration tests; scheduler not distributed (TD-06); no retry backoff (TD-05)

---

## Session 2026-06-16 — Documentation & Skills

- Created `docs/` with full auth-svc reverse-engineering documentation:
  - `docs/goals.md` — business objectives, FR/NFR/constraints, acceptance criteria
  - `docs/context.md` — domain overview, bounded context, actors, workflows, system integrations
  - `docs/architecture.md` — Mermaid diagrams, component design, API, storage DDL, security/observability design
  - `docs/progress-tracking.md` — epics, tasks with current status, blocker table, tech debt register
  - `docs/review.md` — requirement compliance (24/24 FR pass), architecture compliance, security posture, recommendations
- Created `skills/` with three reusable engineering methodology documents:
  - `skills/backend-delivery-framework.md` — documentation lifecycle, traceability rules, delivery workflow, engineering governance
  - `skills/microservice-standards.md` — folder structure, layering, naming, error handling, testing standards
  - `skills/monitoring-observability-standards.md` — logging, metrics, tracing, alerting, SLO, health checks, operational readiness checklist

---

## Pending (priority order)

- [ ] **Run audit DB migration** — `04_setup_banking_audits.sql` + `001_create_audit_events.up.sql`
- [ ] **Introspect endpoint security** — `POST /auth/apikey/introspect` has no auth; add shared-secret header or IP allowlist (currently relies on Docker network isolation only)
- [ ] **Logout ActorID** — logs raw `RefreshToken` as ActorID; replace with `pkgmiddleware.UserIDFromContext(ctx)`
- [ ] **API key cache warm-up** — pre-populate Redis on auth-svc startup to avoid cold-start miss under load
- [ ] **payment-svc** — port 8085, same Dockerfile pattern as auth-svc/account-svc
- [ ] **Integration tests** — scaffold exists in `services/*/tests/integration/`, no tests written
- [ ] **k6 load tests** — scripts in `performance-test-k6/`, run with `make k6-smoke`

---

## Traps to Avoid

- ❌ `datasource/mongo/keyfile` must be a file, not a directory — if Git or Docker creates it as a directory, MongoDB fails with `cp: -r not specified`; delete and regenerate with `openssl rand -base64 756 > datasource/mongo/keyfile`
- ❌ `go work edit -dropuse` in Docker — fragile, breaks silently when services are added
- ❌ `sed` to write base64 RSA keys — corrupts `+`, `/`, `=`; use Python (script in CREDENTIALS.txt)
- ❌ Distroless Docker runtime — no shell/wget, healthchecks fail; use `alpine:3.20` with `wget`
- ❌ `service_healthy` in `depends_on` — use `service_started` (healthcheck was intermittent)
- ❌ `ds_redis` container name — Redis is in platform compose as `platform-redis`
- ❌ Wrong Redis port in `.env` — host port is `9050`; Docker internal is `6379`
- ❌ Missing `REDIS_PASSWORD` in account-svc — Redis has `requirepass`, must set in both `.env` and compose override
- ❌ `FLIPT_URL=localhost:9051` inside Docker — containers can't reach host `localhost`; override with `http://platform-flipt:8080`
- ❌ `ds_postgres` not on `banking-net` — must `docker network connect` manually after datasource-up
- ❌ Wrong `AUTH_SVC_URL` default — should be `http://localhost:8082`, not `8080`
- ❌ Flipt port conflict — Flipt UI is on `9051`, not `8082` (clashed with auth-svc before)

---

## What Worked

- **Go workspace in Docker**: `go work init ./pkg ./services/auth-svc` inside Dockerfile — never copy host `go.work`
- **RSA key generation**: Python `cryptography` library only — `sed` corrupts base64 special chars
- **Alpine runtime**: `FROM alpine:3.20` + `wget` for healthchecks
- **Claims from context**: `middleware.ClaimsFromContext(ctx)` in account-svc — no inter-service inspect call needed
- **pkg/httpx as single response style**: generics + request_id + timestamp + domain error mapping

---

## Adding a New Microservice

1. `cp -r services/account-svc services/new-svc`
2. Update module name in `go.mod` → `github.com/sanusi/banking/services/new-svc`
3. Add `./services/new-svc` to `go.work`
4. Set `HTTP_PORT` to next available `808x`
5. Add service block to `docker-compose.yml` following account-svc pattern
6. Add scrape target to `prometheus.yml` at `host.docker.internal:808x`
7. Update port table in this file

---

## Make Targets

```powershell
make datasource-up     # Postgres, MySQL, MongoDB
make platform-up       # Redis, Flipt, NATS, Metabase
make monitoring-up     # Jaeger, Prometheus, Grafana, Loki, Alertmanager
make services-up       # Build + start microservices
make stack-up          # All of the above
make stack-down        # Stop everything
make services-logs     # Tail microservice logs
make migrate           # Run all SQL migrations (requires Git Bash)
make gen-keys          # Generate new RSA keypair (requires Git Bash + openssl)
make test              # Unit tests (race + cover)
make lint              # golangci-lint
```

---

## Key Files

| File | Purpose |
|---|---|
| `CREDENTIALS.txt` | All secrets — RSA keys, DB passwords, Redis password (gitignored) |
| `services/auth-svc/.env` | auth-svc local config |
| `services/account-svc/.env` | account-svc local config |
| `services/audit-svc/.env` | audit-svc local config |
| `docker-compose.yml` | microservices stack |
| `platform/docker-compose.yml` | Redis, Flipt, NATS, Metabase |
| `monitoring/docker-compose.infra.yml` | Jaeger, Prometheus, Grafana, Loki, Alertmanager |
| `datasource/docker-compose.yml` | Postgres, MySQL, MongoDB |
| `prometheus.yml` | scrape targets + alertmanager |
| `pkg/httpx/` | canonical HTTP response helpers |
| `pkg/audit/` | audit Publisher interface + NATS/HTTP/Noop/Async implementations |
| `pkg/messaging/` | generic NATS JetStream Publisher + NATSConsumer — base layer for pkg/audit and future services |
| `pkg/middleware/` | JWT auth, AuthenticateAny, RequireRole, rate limit, tracing |
