# HANDOFF — golang-banking-platform

> Last updated: 2026-07-26 (payment-svc: InitiateRefund implemented)
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
| `80` | **Traefik gateway** — single HTTP entry point for all services |
| `8080` | **Traefik dashboard** — dev only (`http://localhost:8080`) |
| `808x` | Microservices (direct): auth-svc=8082, account-svc=8081, audit-svc=8083, notification-svc=8084, payment-svc=8085 |
| `900x` | Monitoring: Grafana=9000, Prometheus=9001, Alertmanager=9002, Jaeger=9003, Loki=9004, Discord=9005 |
| `905x` | Platform: Redis=9050, Flipt UI=9051, Flipt gRPC=9052, NATS=9053, NATS UI=9054, Metabase=9055 |
| `4317/4318` | OTLP — standard wire protocol, never change |

### Gateway Route Map

| Path prefix | Service | Port |
|---|---|---|
| `/auth/*` | auth-svc | 8082 |
| `/internal/*` | auth-svc | 8082 |
| `/v1/accounts/*` | account-svc | 8081 |
| `/debug/*` | account-svc | 8081 |
| `/v1/audit/*` | audit-svc | 8083 |
| `/v1/notifications/*` | notification-svc | 8084 |
| `/v1/templates/*` | notification-svc | 8084 |
| `/v1/schedules/*` | notification-svc | 8084 |
| `/v1/payments/*` | payment-svc | 8085 |

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
# Gateway (single entry point for all services)
curl http://localhost/auth/login -X POST -H "Content-Type: application/json" `
  -d '{"username":"admin","password":"Admin@12345"}'

# Traefik dashboard (see all routes registered)
# http://localhost:8080

# Direct service health (bypasses gateway)
curl http://localhost:8082/healthz/ready   # auth-svc
curl http://localhost:8081/healthz/ready   # account-svc
curl http://localhost:8083/healthz/ready   # audit-svc
curl http://localhost:8084/healthz/ready   # notification-svc
curl http://localhost:8085/healthz/ready   # payment-svc

# Prometheus targets — all should be UP
# http://localhost:9001/targets

# Grafana dashboards
# http://localhost:9000  (admin / admin)

# Full gateway integration test
make gateway-test
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
- `pkg/httpx` — canonical response helpers (all services use this, no local duplicates); `mapDomainError` handles all pkg/errors types including 429 RateLimited
- `pkg/middleware` — JWT auth, `AuthenticateAny`, `RequireRole`, rate limit, tracing, metrics; all error responses now via `httpx.WriteHTTPError` (no raw JSON)
- `pkg/errors`, `pkg/observability`, `pkg/featureflag`, `pkg/idempotency`
- `pkg/cli` — shared Cobra root builder (`NewRoot`). Every service's `main()` builds its root via `cli.NewRoot(name, run)`; the root **defaults to serve**, so a bare `./svc` invocation boots the HTTP server + workers exactly as before (Docker `ENTRYPOINT`/compose unchanged). Explicit `serve` subcommand also registered; the auto-generated `completion` command is hidden. Subcommands (migrate, gen-keys, worker) are additive/opt-in — see Pending.

### Error handling (unified 2026-07-06)
All services use `httpx.WriteError(w, r, err)` — no local `writeXxxError` helpers anywhere.
- Service layer wraps repository sentinel errors → `pkg/errors` domain types before returning
- Transport layer calls `httpx.WriteError` only; `httpx.WriteHTTPError` for domain-specific codes
- account-svc: `ErrAccountNotActive` → `pkgerrors.Validation`, `ErrInsufficientFunds` → `pkgerrors.Validation`, `ErrConflict` on Update → `pkgerrors.PreconditionFailed`

### Observability
- Prometheus scrapes all three services via `host.docker.internal:808x`
- `/metrics` and `/healthz/*` excluded from HTTP metrics and request logs in all services
- Grafana Service Logs panel filters `/metrics` and `/healthz/*` at Loki query level
- Grafana: Prometheus, Loki, Jaeger, Alertmanager datasources provisioned
- Discord relay: Alertmanager → Discord notifications
- Loki + Promtail: log aggregation from Docker containers and local `./logs/*.log`

### payment-svc (port 8085 — docs complete, implementation not started)

- Full 5-document lifecycle written (2026-06-19)
- `services/payment-svc/docs/goals.md` — 5 BO, 20 FR, 14 NFR, 6 constraints, 6 assumptions, 12 acceptance criteria, service boundaries
- `services/payment-svc/docs/context.md` — domain overview, bounded context diagram, 8 business workflows (BW-01–BW-08), actor table, upstream/downstream systems, 8 risks (R-01–R-08), assumptions revisited
- `services/payment-svc/docs/architecture.md` — Mermaid diagrams (high-level + request lifecycle), layered component design, full package tree, SQL DDL for 3 tables (`transactions`, `reversals`, `idempotency_records`), Redis key patterns, full API table, integration patterns, security/observability/reliability design
- `services/payment-svc/docs/progress-tracking.md` — 9 epics (E1–E9), all tasks ⬜, dependency graph, 3 tech debt items (TD-01–TD-03)
- `services/payment-svc/docs/review.md` — all criteria ⬜ Unverified; P0 recommendations: implement compensation flow and idempotency before opening any endpoints
- Key design decisions: Redis SET NX for first-writer-wins idempotency; DB unique constraint on `idempotency_key` and `reversal.original_txn_id` as backstops; circuit breaker + exponential backoff on Account Service; NATS event publishing is non-blocking fire-and-forget; NATS consumer uses queue group for exactly-once delivery across instances
- **Scaffold complete (E1+E2+E3 partial):** service boots, runs migrations, serves /healthz; transfer/merchant/fee/reverse/cancel/retry endpoints still return `errNotImplemented` (E4 pending). **Refund is implemented** (E4-T10, 2026-07-26): `InitiateRefund` validates `original_reference` exists and is `SUCCESS`, then reuses `orchestrator.executeDebitCredit` (same idempotent debit/credit + compensation seam QRIS uses). Unit tests in `internal/service/payment_service_test.go`. Idempotency here is the DB-unique-key replay, not yet the Redis `DualStore` — tracked as `TD-06`.
- Uses `pkg/idempotency.DualStore` (Redis SET NX + Postgres fallback) — no custom idempotency code
- Uses `pkg/httpclient` for Account Service calls (retry + backoff built in)
- Amount stored as `BIGINT` (minor currency units) matching account-svc convention
- Run `cd services/payment-svc && go mod tidy` before first build to generate go.sum
- Run `datasource/postgres/06_setup_banking_payments.sql` as superuser before starting the service

#### QRIS (implemented 2026-07-07)

- **EMVCo MPM codec** (`internal/qris/`) — `Encode`/`Decode` over TLV + CRC-16/CCITT-FALSE (tag 63). Pure, table-driven tested (canonical CRC vector `123456789`→`29B1`, round-trip, tamper detection). Supports **dynamic** (amount-embedded, tag 01=12) and **static** (tag 01=11) QR.
- **Merchant registry** + **qris_charges** tables via `migrations/002_create_qris_tables.up.sql`; extends the `transactions` type CHECK to allow `QRIS`. New DAOs `dao/merchant.go`, `dao/qris_charge.go`; repos in `internal/infra/postgres/`.
- **Reusable orchestration** (`internal/service/orchestration.go`) — `executeDebitCredit` runs debit(payer)→credit(merchant) with **transaction-level idempotency** (via `GetByIdempotencyKey` + unique constraint) and **compensation** (credit failure → refund payer, mark FAILED). This is the seam the future E4 transfer/merchant flows should adopt.
- **QRIS service** (`internal/service/qris_service.go`) — RegisterMerchant/GetMerchant/GenerateCharge/Decode/Pay. Merchant is credited to an internal account (simulated; no external acquirer). Amounts convert int64 minor units ↔ EMVCo major decimal.
- **Endpoints** (RBAC TELLER/ADMIN; ADMIN for merchant registration): `POST /v1/merchants`, `GET /v1/merchants/{id}`, `POST /v1/payments/qris/{generate,decode,pay}`. `pay` requires `Idempotency-Key`.
- **Config:** `QRIS_ACQUIRER_GUID` (default `ID.CO.QRIS.WWW`), `QRIS_DEFAULT_MCC`, `QRIS_CURRENCY` (`IDR`), `QRIS_CHARGE_TTL_SECONDS`.
- **Note:** used transaction-level idempotency (not the container's HTTP-oriented `DualStore`, which stays constructed-but-unused as before). CPM mode and QRIS refund/reversal are out of scope.

### notification-svc

- Full 5-document lifecycle written (reverse-engineered from code, 2026-06-19)
- `services/notification-svc/docs/goals.md` — 17 FR, 9 NFR, 10 constraints, 17 acceptance criteria, service boundaries
- `services/notification-svc/docs/context.md` — domain overview, bounded context diagram, 6 business workflows, actor table, risk register, upstream/downstream systems
- `services/notification-svc/docs/architecture.md` — Mermaid diagrams (high-level + sequence), layering rules, package tree, SQL DDL for 3 tables, full API table, security/observability/reliability design
- `services/notification-svc/docs/progress-tracking.md` — 7 epics, 48 tasks (most ✅), 10 tech debt items (TD-01–TD-10)
- `services/notification-svc/docs/review.md` — FR compliance (16/17 pass, FR-02 partial — stubs), NFR (3 unverified: latency/throughput), 10 TD items, immediate/short/medium recommendations
- Key gaps: EMAIL/SMS/PUSH/WHATSAPP are stubs (TD-01–TD-04); no integration tests; scheduler not distributed (TD-06); no retry backoff (TD-05)
- Note (2026-07-10): a stale shell-doc duplicate (`docs/notification-svc/*`, older/unfilled) existed alongside this set from an earlier scaffold pass; discarded during the docs/ → services/{name}/docs/ move — `services/notification-svc/docs/*` is the sole surviving, authoritative copy.

### account-svc

- `services/account-svc/docs/{goals,context,architecture,progress-tracking,review}.md` — scaffolded stubs (2026-07-10), live service, delivery-framework pass not yet run

### kyc-svc (port 8084 — 5-doc lifecycle complete, implementation ready to start)

- **New service** for customer identity verification (KTP OCR extraction + scoring). Designed to be extractable into standalone external product later.
- `services/kyc-svc/CONTEXT.md` — domain overview, verification types (ktp_ocr v1), outcome states, language glossary, 9 v1 fields
- `services/kyc-svc/docs/adr/` — 3 architecture decision records (self-contained auth+audit, MinIO+PII retention, sidecar OCR engines)
- `services/kyc-svc/goals.md` — **✅ complete (2026-07-14)**: 5 BO, 19 FR, 21 NFR, 8 constraints, 7 assumptions, 24 acceptance criteria
- `services/kyc-svc/architecture.md` — **✅ complete (2026-07-14)**: high-level diagram, layering model, 19 components, 3-table schema (verifications, api_keys, audit_log), MinIO design, API contract (POST /v1/verify), Prometheus/Jaeger instrumentation, security threat model, scalability/reliability considerations
- `services/kyc-svc/progress-tracking.md` — **✅ complete (2026-07-14)**: 8 epics (E1–E8, 60+ tasks), critical path (E1 auth → E2 verification → E3 storage → E4 API → E5 lifecycle), 8 tech debt items, 3 blockers (E7 dataset, E5 webhook auth, MinIO deployment), timeline 2026-07-15 through 2026-08-15 (est.)
- **Next:** architecture review, assign epic owners, kick off E1 (auth) + E3 (storage) in parallel, run E7 benchmark concurrently with E2 (verification logic)

---

## Session 2026-06-16 — Documentation & Skills

- Created `docs/` with full auth-svc reverse-engineering documentation (now `services/auth-svc/docs/`):
  - `services/auth-svc/docs/goals.md` — business objectives, FR/NFR/constraints, acceptance criteria
  - `services/auth-svc/docs/context.md` — domain overview, bounded context, actors, workflows, system integrations
  - `services/auth-svc/docs/architecture.md` — Mermaid diagrams, component design, API, storage DDL, security/observability design
  - `services/auth-svc/docs/progress-tracking.md` — epics, tasks with current status, blocker table, tech debt register
  - `services/auth-svc/docs/review.md` — requirement compliance (24/24 FR pass), architecture compliance, security posture, recommendations
- Created `skills/` with three reusable engineering methodology documents:
  - `skills/backend-delivery-framework.md` — documentation lifecycle, traceability rules, delivery workflow, engineering governance
  - `skills/microservice-standards.md` — folder structure, layering, naming, error handling, testing standards
  - `skills/monitoring-observability-standards.md` — logging, metrics, tracing, alerting, SLO, health checks, operational readiness checklist

## Session 2026-07-08 — Skills → Slash Commands

- Converted `skills/` documents into proper Claude Code project-level slash commands:
  - `/eng-delivery` — backend delivery framework (`.claude/commands/eng-delivery.md`)
  - `/eng-standards` — microservice standards (`.claude/commands/eng-standards.md`)
  - `/eng-observability` — monitoring & observability standards (`.claude/commands/eng-observability.md`)
- `skills/` folder and `.skill` archives remain as canonical source; `.claude/commands/` mirrors for project-level access
- `.claude/commands/README.md` — quick-reference for all slash commands
- Type `/` in Claude Code to autocomplete — all three skills now appear in the command palette

## Session 2026-07-27 — Skills relocated to .claude/skills

- Moved the three `.skill` archives from root `skills/` into `.claude/skills/` (canonical source now lives under `.claude/`, root `skills/` removed)
- `.claude/commands/eng-*.md` slash commands unchanged, still the project-level mirror

---

## Session 2026-08-05 — auth-svc Logout ActorID fix (GH #6)

- `POST /auth/logout` now requires a Bearer token (`pkgmiddleware.Authenticate` on the route)
- Handler logs `pkgmiddleware.UserIDFromContext(ctx)` as audit `ActorID` instead of the raw refresh token
- Updated k6 flows (`auth-flow.js`, `account-flow.js`, `orchestration-flow.js`) and Postman collection to send `Authorization: Bearer` on logout calls

## Session 2026-08-05 — secure introspect endpoint (GH #5)

- Added `pkg/middleware.RequireServiceSecret` — constant-time `X-Service-Secret` header check, no-op if the secret is empty
- `POST /auth/apikey/introspect` on auth-svc now requires it; secret set via `SERVICE_SECRET` env var
- `authclient.New(authSvcURL, serviceSecret)` sends the header on every introspect call from account-svc; `SERVICE_SECRET` must match on both services
- Added to both services' `.env.example`

## Session 2026-08-05 — account-svc test layout (GH #8)

- Moved `internal/services/account_service_test.go` → `tests/unit/account_service_test.go`, package renamed `services_test` → `unit` to match the `tests/unit/` convention used by auth-svc/audit-svc

---

## Testing Standards (2026-06-21)

All unit tests now follow a standardized structure defined in the **eng-testing** skill (`/eng-testing`).

### Structure

Each service organizes tests in a dedicated `tests/` folder:

```
service-name/
├── internal/          (production code)
├── migrations/        (SQL migrations)
├── tests/
│   ├── unit/          (unit tests in `package unit`)
│   │   ├── mocks.go             (all mock implementations)
│   │   ├── helpers.go           (reusable test utilities)
│   │   ├── auth_service_test.go
│   │   └── apikey_service_test.go
│   └── integration/   (future: integration tests)
└── Makefile
```

### Run Tests

```bash
# Run all unit tests for a service
go test ./services/auth-svc/tests/unit

# With coverage
go test -cover ./services/auth-svc/tests/unit

# Specific test
go test -run TestLogin_Success ./services/auth-svc/tests/unit
```

### Coverage Targets

| Type | Target |
|------|--------|
| Service logic | 90%+ |
| Repositories (mocked) | 85%+ |
| Handlers | 80%+ |
| DTOs/DAOs | 0% (acceptable) |

### Implemented

✅ **auth-svc** — 24 unit tests covering Login/Refresh/Logout/TokenIssuance with 80.6% coverage (external package)

### TODO

- [ ] **account-svc** — migrate existing tests to `tests/unit/` structure
- [ ] **audit-svc** — write unit tests from scratch
- [ ] **payment-svc** — write unit tests from scratch
- [ ] **Integration tests** — scaffold in `tests/integration/`

---

## Pending (priority order)

- [ ] **Run audit DB migration** — `04_setup_banking_audits.sql` + `001_create_audit_events.up.sql`
- [ ] **chi RealIP IP-spoofing (all 5 services)** — `chimiddleware.RealIP` trusts `X-Forwarded-For`/`X-Real-IP` unconditionally (GHSA-3fxj-6jh8-hvhx); since every service is also directly reachable on its own port (not just via Traefik), a caller can forge these headers when hitting a service directly. Needs a trusted-proxy-scoped IP resolver (only trust the header when the immediate peer is Traefik's known address) instead of chi's unconditional `RealIP`. Currently `//nolint:staticcheck`-suppressed with this note in each `routes.go`/`router.go`, not silently ignored.
- [ ] **API key cache warm-up** — pre-populate Redis on auth-svc startup to avoid cold-start miss under load
- [ ] **payment-svc E4 (service layer)** — `InitiateRefund` done; implement `InitiateTransfer`, `InitiateMerchantPayment`, `InitiateFee`, `Reverse`, `Cancel`, `Retry` in `internal/service/`; handlers already wired, just need the service logic
- [ ] **payment-svc TD-06** — wire `pkg/idempotency.DualStore` (Redis SET NX + Postgres fallback) into `orchestrator.executeDebitCredit` so all payment types get the Redis fast-path + `payment_idempotency_hits_total` metric, not just the DB-unique-key replay
- [ ] **payment-svc go.sum** — run `cd services/payment-svc && go mod tidy` once before first build
- [ ] **payment-svc DB setup** — run `datasource/postgres/06_setup_banking_payments.sql` as superuser
- [ ] **payment-svc ACCOUNT_SVC_API_KEY** — `services/payment-svc/.env` has `ACCOUNT_SVC_API_KEY=` empty; generate a `bp_live_*` service account key via `POST /internal/service-accounts` on auth-svc and paste it in
- [ ] **Integration tests** — scaffold exists in `services/*/tests/integration/`, no tests written
- [ ] **k6 load tests** — scripts in `performance-test-k6/`, run with `make k6-smoke`
- [ ] **CLI subcommands (Phase 2)** — `pkg/cli` foundation shipped (serve-by-default). Add `migrate` (reuse `golang-migrate/migrate/v4`, already a dep — works inside alpine where psql is absent) and `gen-keys` (RS256 in Go, avoids `sed`/base64 corruption). Optional later: `worker`-only subcommand + repoint Makefile `migrate-*`/`gen-keys` to the binary.

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
| `services/payment-svc/.env` | payment-svc local config (`ACCOUNT_SVC_API_KEY` must be filled in) |
| `docker-compose.yml` | microservices stack |
| `platform/docker-compose.yml` | Redis, Flipt, NATS, Metabase |
| `monitoring/docker-compose.infra.yml` | Jaeger, Prometheus, Grafana, Loki, Alertmanager |
| `datasource/docker-compose.yml` | Postgres, MySQL, MongoDB |
| `prometheus.yml` | scrape targets + alertmanager |
| `pkg/httpx/` | canonical HTTP response helpers |
| `pkg/audit/` | audit Publisher interface + NATS/HTTP/Noop/Async implementations |
| `pkg/messaging/` | generic NATS JetStream Publisher + NATSConsumer — base layer for pkg/audit and future services |
| `pkg/middleware/` | JWT auth, AuthenticateAny, RequireRole, rate limit, tracing |
