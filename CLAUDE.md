# Golang Banking Platform — CLAUDE.md

> **Purpose of this file:** Complete reference for any developer (or AI assistant) working in this repository. Read this before touching any code.

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Architecture Philosophy](#2-architecture-philosophy)
3. [Monorepo Structure](#3-monorepo-structure)
4. [Services](#4-services)
5. [Shared Package (`pkg/`)](#5-shared-package-pkg)
6. [Database Architecture](#6-database-architecture)
7. [Authentication & JWT](#7-authentication--jwt)
8. [Local Development — Quick Start](#8-local-development--quick-start)
9. [Environment Variables](#9-environment-variables)
10. [Ports & URLs](#10-ports--urls)
11. [Observability Stack](#11-observability-stack)
12. [Grafana Dashboards](#12-grafana-dashboards)
13. [Makefile Reference](#13-makefile-reference)
14. [Docker Compose Workflow](#14-docker-compose-workflow)
15. [CI/CD](#15-cicd)
16. [Coding Conventions](#16-coding-conventions)
17. [Testing Strategy](#17-testing-strategy)
18. [How to Add a New Microservice](#18-how-to-add-a-new-microservice)
19. [Deployment (Kubernetes / Helm / ArgoCD)](#19-deployment-kubernetes--helm--argocd)
20. [Key Design Decisions & Trade-offs](#20-key-design-decisions--trade-offs)

---

## 1. Project Overview

A **production-grade, cloud-native banking platform** built as a Go monorepo. It demonstrates enterprise patterns comparable to Stripe, Uber, and Cloudflare — designed to handle millions of requests per day with full observability, strict security boundaries, and a developer experience that scales to a large team.

**Current services:**

| Service | Port | Responsibility |
|---|---|---|
| `auth-svc` | 8082 | User registration, login, RS256 JWT issuance |
| `account-svc` | 8081 | Account CRUD, balance, credit/debit transactions |

**Observability infrastructure (Docker Compose):**

| Tool | Port | Purpose |
|---|---|---|
| Grafana | 3000 | Dashboards, alerts UI |
| Prometheus | 9090 | Metrics scraping & alert evaluation |
| Jaeger | 16686 | Distributed tracing (OTLP) |
| Loki | 3100 | Log aggregation |
| Promtail | — | Log collector (Docker SD + local file scrape) |
| Alertmanager | 9093 | Alert routing & deduplication |
| PostgreSQL | 5432 | Databases (`banking_auth`, `banking_accounts`) |

---

## 2. Architecture Philosophy

### Core Principles

- **Standard library first** — `net/http`, `log/slog`, `context`, `database/sql`. External libraries only when they add material value.
- **Explicit > magic** — no reflection-based ORMs doing implicit things. GORM is used for schema convenience but queries are always reviewable.
- **Interface-driven design** — every repository and service layer is backed by an interface. This makes testing trivial and swapping implementations safe.
- **Clean Architecture layers** — dependency direction is strictly inward:

```
Transport (HTTP handlers)
    ↓
Services (business logic)
    ↓
Repository (data access interface)
    ↓
DAO (database structs / raw models)
```

- **Domain-driven service boundaries** — each microservice owns its database, its domain models, and its migration files. No cross-service database access.
- **12-factor compliance** — config from environment variables, stateless processes, logs to stdout, backing services via URLs.
- **SOLID principles** — single responsibility per layer; open for extension by adding new services without modifying shared packages.

### What We Avoid

- Heavy frameworks (Gin, Echo, Fiber) — `chi` gives us composable middleware with zero overhead over `net/http`.
- Global mutable state — no `init()` side effects, no package-level singletons except the logger.
- Fat controllers — handlers are thin; business logic lives in the service layer.
- Circular imports — `pkg/` has no knowledge of any service; services never import each other.

---

## 3. Monorepo Structure

```
golang-banking-platform/
│
├── go.work                          # Go workspace — ties all modules together
│
├── Makefile                         # Root-level developer targets (see §13)
├── .golangci.yml                    # Linter config (golangci-lint)
├── .air.toml                        # Hot-reload config for Air (account-svc default)
│
├── pkg/                             # Shared library — imported by all services
│   ├── go.mod                       # module: github.com/sanusi/banking/pkg
│   ├── config/      config.go       # Env-var config loader
│   ├── database/    postgres.go     # pgx connection pool factory
│   ├── errors/      errors.go       # Structured domain errors
│   ├── httpx/                       # HTTP helpers
│   │   ├── request.go               # JSON decode + validate
│   │   ├── response.go              # Standard success/pagination response
│   │   └── errors.go                # Standard error response
│   ├── logger/                      # slog-based structured logger
│   │   ├── logger.go
│   │   ├── handler.go               # Custom slog handler (JSON / pretty)
│   │   └── config.go
│   ├── middleware/                  # Reusable chi middleware
│   │   ├── chain.go                 # Middleware composition helpers
│   │   ├── auth.go                  # JWT RS256 verification + RBAC
│   │   ├── cors.go
│   │   ├── logger.go                # Request/response structured logging
│   │   ├── metrics.go               # Prometheus HTTP instrumentation
│   │   ├── ratelimit.go             # Token bucket rate limiter
│   │   ├── recovery.go              # Panic recovery → 500
│   │   ├── requestid.go             # Correlation ID (X-Request-ID)
│   │   ├── timeout.go               # Per-route deadline
│   │   └── tracing.go               # OpenTelemetry span creation
│   ├── observability/
│   │   ├── otel.go                  # OTel SDK bootstrap (traces + metrics)
│   │   └── health.go                # /healthz/live + /healthz/ready handlers
│   └── validator/   validator.go    # go-playground/validator wrapper
│
├── proto/                           # Protobuf definitions (future gRPC)
│   └── banking/v1/account.proto
│
├── services/
│   ├── account-svc/                 # Account management service (port 8081)
│   │   ├── go.mod
│   │   ├── Dockerfile
│   │   ├── Makefile                 # Service-level targets
│   │   ├── .env                     # Local dev env vars (NOT committed for prod)
│   │   ├── cmd/server/
│   │   │   ├── main.go              # Entrypoint: config → container → server
│   │   │   └── container.go         # Dependency injection (wire everything)
│   │   ├── config/config.go         # Service-specific config struct
│   │   ├── internal/
│   │   │   ├── domain/
│   │   │   │   ├── dao/             # DB structs (Account, Transaction, User)
│   │   │   │   └── dto/             # Request/response DTOs
│   │   │   ├── repository/          # DB access implementing interfaces
│   │   │   ├── services/            # Business logic
│   │   │   └── transport/           # chi handlers + router
│   │   ├── migrations/              # Numbered SQL migration files
│   │   └── tests/
│   │       ├── integration/         # Requires running Postgres (build tag: integration)
│   │       └── unit/                # Pure unit tests with mocks
│   │
│   └── auth-svc/                    # Authentication service (port 8082)
│       ├── go.mod
│       ├── Dockerfile
│       ├── .env
│       ├── cmd/server/
│       │   ├── main.go
│       │   └── container.go
│       ├── config/config.go
│       ├── internal/
│       │   ├── domain/dao/user.go
│       │   ├── domain/dto/auth.go
│       │   ├── repository/user_repository.go
│       │   ├── services/auth_service.go
│       │   └── transport/           # Routes, handler, response helpers
│       └── migrations/001_create_users.sql
│
├── deploy/
│   ├── helm/account-svc/            # Helm chart (Deployment, Service, HPA, PDB)
│   └── argocd/account-svc-app.yaml  # ArgoCD Application manifest
│
├── infra/
│   ├── alerting/rules/account-svc.yml   # Prometheus alert rules
│   ├── grafana/provisioning/
│   │   ├── datasources/prometheus.yml   # Prometheus + Loki + Jaeger datasources
│   │   └── dashboards/
│   │       ├── dashboard.yml            # Grafana dashboard loader config
│   │       ├── platform-overview.json   # Cross-service executive dashboard
│   │       └── service-overview.json    # Per-service drill-down dashboard
│   ├── loki/loki-config.yml            # Loki single-binary config (tsdb v13)
│   ├── postgres/setup.sql              # DB + user bootstrap script
│   └── promtail/promtail-config.yml    # Dual-mode log collector config
│
├── logs/                               # Local service log files (Promtail reads these)
│   └── .gitkeep                        # Keeps the dir in git; *.log is gitignored
│
├── prometheus.yml                      # Prometheus scrape config
├── alertmanager.yml                    # Alertmanager routing config
├── docker-compose.yml                  # Microservices compose file
└── docker-compose.infra.yml            # Observability infra compose file
```

---

## 4. Services

### auth-svc (port 8082)

**Responsibility:** The single source of truth for identity. Issues RS256-signed JWTs. Never validates tokens — it only creates them.

**Endpoints:**

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/auth/login` | None | Exchange credentials for a JWT |
| GET | `/healthz/live` | None | Liveness probe |
| GET | `/healthz/ready` | None | Readiness probe (checks DB) |
| GET | `/metrics` | None | Prometheus metrics |

**Key design notes:**
- Holds the **private** RS256 key. No other service has this key.
- Passwords are hashed with `bcrypt`.
- Token TTL is configurable via `JWT_TOKEN_TTL` (default `24h`).
- Database: `banking_auth` (tables: `users`)

---

### account-svc (port 8081)

**Responsibility:** Account lifecycle and transaction management. Verifies JWTs but never issues them.

**Endpoints:**

| Method | Path | Required Role | Description |
|---|---|---|---|
| GET | `/v1/accounts` | ADMIN | List all accounts |
| POST | `/v1/accounts` | ADMIN | Create account |
| GET | `/v1/accounts/{id}` | TELLER, ADMIN | Get account details |
| GET | `/v1/accounts/{id}/balance` | TELLER, ADMIN | Get balance |
| POST | `/v1/accounts/{id}/credit` | TELLER, ADMIN | Credit funds |
| POST | `/v1/accounts/{id}/debit` | TELLER, ADMIN | Debit funds |
| GET | `/healthz/live` | None | Liveness probe |
| GET | `/healthz/ready` | None | Readiness probe |
| GET | `/metrics` | None | Prometheus metrics |
| GET | `/debug/*` | None (local only) | Test endpoints for traces/alerts |

**Debug endpoints** (only active when `ENVIRONMENT=local`):
- `GET /debug/ping` — INFO log + 200
- `GET /debug/warn` — WARN log + 200
- `GET /debug/error` — ERROR log + 500 (triggers alert rule)
- `GET /debug/slow` — sleeps 3s (triggers latency alert)
- `GET /debug/panic` — recovered panic (tests Recovery middleware)

**Key design notes:**
- Holds the **public** RS256 key only. Cannot issue tokens.
- RBAC enforced per route via `pkgmiddleware.RequireRole`.
- Rate limiting: token bucket (`RATE_LIMIT_RPS` / `RATE_LIMIT_BURST`).
- Request timeout applied to all `/v1/*` routes.
- Database: `banking_accounts` (tables: `accounts`, `transactions`, `users`)

---

## 5. Shared Package (`pkg/`)

The `pkg/` directory is a standalone Go module (`github.com/sanusi/banking/pkg`) imported by all services via the Go workspace `replace` directive.

**Rule: `pkg/` must never import any service package.** Dependency direction is always `service → pkg`, never `pkg → service`.

### Key packages

#### `pkg/config`
Loads all configuration from environment variables (with `.env` file fallback). Services define their own config struct and call `config.Load(&cfg)`.

#### `pkg/middleware`

All middleware is compatible with standard `net/http` and designed for `chi`:

- **RequestID** — generates or propagates `X-Request-ID` header; stores in context.
- **RequestLogger** — structured JSON log per request: method, path, status, latency, request ID, trace ID.
- **Tracing** — creates an OpenTelemetry span for each request; propagates W3C `traceparent` headers.
- **Metrics** — records `http_requests_total` (counter) and `http_request_duration_seconds` (histogram) labelled by method, path, and status.
- **Recovery** — catches panics, logs them, returns 500.
- **Authenticate** — validates RS256 JWT, injects claims into context.
- **RequireRole** — RBAC check; reads roles from JWT claims.
- **RateLimit** — token bucket per IP.
- **Timeout** — wraps request context with a deadline.
- **CORS** — configurable allowed origins.

Middleware execution order (outer → inner):
```
RealIP → RequestID → RequestLogger → Tracing → Metrics → Recovery
    → [per-group: Timeout → RateLimit → Authenticate → RequireRole]
        → Handler
```

#### `pkg/httpx`
Standard request/response envelope:

```json
// Success
{ "success": true, "data": { ... }, "meta": { "request_id": "...", "timestamp": "..." } }

// Error
{ "success": false, "error": { "code": "NOT_FOUND", "message": "account not found" } }

// Paginated
{ "success": true, "data": [...], "pagination": { "page": 1, "per_page": 20, "total": 100 } }
```

#### `pkg/logger`
Built on `log/slog`. In `ENVIRONMENT=local`, uses a pretty-printed handler. In all other environments, uses JSON. Every log line automatically includes `trace_id` and `request_id` when a tracing context is present.

#### `pkg/observability`
- **`otel.go`** — initialises the OpenTelemetry SDK: trace exporter (OTLP gRPC → Jaeger), metric exporter (Prometheus). Returns a shutdown function.
- **`health.go`** — `/healthz/live` always returns 200. `/healthz/ready` runs registered checks (e.g. DB ping) and returns 200 or 503.

---

## 6. Database Architecture

### Database-per-service isolation

Each service has its own PostgreSQL database and a dedicated PostgreSQL user. Cross-service database access is impossible at the PostgreSQL permission level.

| Service | Database | PostgreSQL User |
|---|---|---|
| auth-svc | `banking_auth` | `auth_svc` |
| account-svc | `banking_accounts` | `account_svc` |

### Setup (one-time)

```bash
# Start Postgres (via Docker Compose or locally)
make infra-up

# Create databases and users
psql -h localhost -U admin -d postgres -f infra/postgres/setup.sql

# Run migrations per service
psql -h localhost -U auth_svc -d banking_auth \
  -f services/auth-svc/migrations/001_create_users.sql

psql -h localhost -U account_svc -d banking_accounts \
  -f services/account-svc/migrations/001_create_accounts.sql \
  -f services/account-svc/migrations/002_create_transactions.sql \
  -f services/account-svc/migrations/003_create_users.sql
```

### Migrations

Migrations are plain numbered SQL files in `services/{svc}/migrations/`. Apply them in order. There is no migration framework — this is intentional (explicit > magic).

The root `Makefile` has a `migrate` target for scripted application:
```bash
DB_HOST=localhost DB_PORT=5432 DB_NAME=banking_accounts DB_USER=account_svc \
  DB_PASSWORD=account_svc_pass_local make migrate
```

### GORM usage

GORM is used for struct-based schema definitions (DAOs) and simple CRUD. Complex queries use raw SQL via `db.Raw()` / `db.Exec()`. Transactions use `db.Transaction(func(tx *gorm.DB) error { ... })`.

---

## 7. Authentication & JWT

### Key architecture

```
auth-svc                       account-svc (and any other service)
─────────────────              ────────────────────────────────────
Holds PRIVATE key (RS256)      Holds PUBLIC key only
Signs JWTs on /auth/login      Verifies JWTs on every protected request
Cannot verify → can only sign  Cannot sign → can only verify
```

### RS256 key format

Keys are PKCS#8 (private) / PKIX (public) PEM files, base64-encoded and stored as environment variables:

```bash
# Generate a fresh keypair:
openssl genrsa -out private.pem 2048
openssl pkcs8 -topk8 -nocrypt -in private.pem -out private_pkcs8.pem
openssl rsa -in private.pem -pubout -out public.pem

# Base64-encode for env vars:
base64 -w0 private_pkcs8.pem   # → JWT_PRIVATE_KEY_B64 in auth-svc/.env
base64 -w0 public.pem          # → JWT_PUBLIC_KEY_B64  in account-svc/.env (and all others)
```

### JWT claims structure

```json
{
  "sub": "<user_uuid>",
  "iss": "banking-platform",
  "roles": ["TELLER"],
  "exp": 1234567890,
  "iat": 1234567890,
  "jti": "<uuid>"
}
```

### RBAC roles

| Role | Permissions |
|---|---|
| `ADMIN` | All account operations, list all accounts |
| `TELLER` | Read account details, credit, debit |

---

## 8. Local Development — Quick Start

### Prerequisites

- Go 1.25+
- Docker Desktop
- `psql` CLI (for migrations)
- `golangci-lint` (for linting)
- `protoc` + `protoc-gen-go` (for protobuf regeneration, optional)
- `air` (for hot reload, optional): `go install github.com/air-verse/air@latest`

### Step 1 — Start observability infrastructure

```bash
make infra-up
```

This starts: Postgres, Jaeger, Prometheus, Alertmanager, Grafana, Loki, Promtail.

Wait ~10 seconds for all services to be healthy.

### Step 2 — Set up databases (first time only)

```bash
# Create databases and users
psql -h localhost -U admin -d postgres -f infra/postgres/setup.sql

# Run migrations
psql -h localhost -U auth_svc -d banking_auth \
  -f services/auth-svc/migrations/001_create_users.sql

psql -h localhost -U account_svc -d banking_accounts \
  -f services/account-svc/migrations/001_create_accounts.sql \
  -f services/account-svc/migrations/002_create_transactions.sql \
  -f services/account-svc/migrations/003_create_users.sql
```

### Step 3 — Run services locally

Each service reads its `.env` file automatically. Open two terminals:

```bash
# Terminal 1
make run-auth-svc       # → http://localhost:8082, logs → ./logs/auth-svc.log

# Terminal 2
make run-account-svc    # → http://localhost:8081, logs → ./logs/account-svc.log
```

Or run both in one command (auth-svc in background):
```bash
make run-all
```

Tail all logs:
```bash
make logs-follow
```

### Step 4 — Verify

```bash
# Auth service
curl http://localhost:8082/healthz/live
curl http://localhost:8082/healthz/ready

# Account service
curl http://localhost:8081/healthz/live
curl http://localhost:8081/healthz/ready

# Get a JWT
curl -X POST http://localhost:8082/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@bank.com","password":"secret"}'

# Use the JWT
curl http://localhost:8081/v1/accounts \
  -H "Authorization: Bearer <token>"
```

### Hot reload (Air)

```bash
make dev        # runs account-svc with Air — rebuilds on file save
```

### Prometheus reload (after editing prometheus.yml)

```bash
curl -X POST http://localhost:9090/-/reload
```

---

## 9. Environment Variables

### Common to all services

| Variable | Default | Description |
|---|---|---|
| `SERVICE_NAME` | — | Service identifier (used in traces, logs, metrics) |
| `SERVICE_VERSION` | `dev` | Semver string |
| `ENVIRONMENT` | `local` | One of: `local`, `staging`, `production` |
| `HTTP_PORT` | — | HTTP listen port |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | auto | `json` or `pretty` (auto: pretty if `ENVIRONMENT=local`) |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_NAME` | — | Database name |
| `DB_USER` | — | PostgreSQL user |
| `DB_PASSWORD` | — | PostgreSQL password |
| `DB_SSLMODE` | `disable` | `disable`, `require`, `verify-full` |
| `DB_MAX_CONNS` | `10` | Connection pool max |
| `DB_MIN_CONNS` | `2` | Connection pool min |
| `OTEL_ENABLED` | `true` | Enable OpenTelemetry |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `localhost:4317` | Jaeger OTLP gRPC endpoint |
| `OTEL_SAMPLING_RATE` | `1.0` | Trace sampling rate (0.0–1.0) |

### auth-svc specific

| Variable | Description |
|---|---|
| `JWT_PRIVATE_KEY_B64` | Base64-encoded PKCS#8 PEM private key (RS256) |
| `JWT_ISSUER` | JWT `iss` claim value (e.g. `banking-platform`) |
| `JWT_TOKEN_TTL` | Token lifetime (e.g. `24h`) |

### account-svc specific

| Variable | Description |
|---|---|
| `JWT_PUBLIC_KEY_B64` | Base64-encoded PKIX PEM public key (must match auth-svc private key) |
| `JWT_ISSUER` | Expected JWT `iss` claim (must match auth-svc) |
| `RATE_LIMIT_RPS` | Requests per second per IP (default: `1000`) |
| `RATE_LIMIT_BURST` | Burst capacity (default: `2000`) |
| `REQUEST_TIMEOUT` | Per-request timeout in seconds (default: `30`) |

### Secrets in production

Never commit `.env` files containing real secrets. In production:
- Use Kubernetes Secrets (referenced via Helm values).
- Or use HashiCorp Vault / AWS Secrets Manager with a sidecar injector.
- The `.env.example` files in each service show all required variables without values.

---

## 10. Ports & URLs

### Services (local dev)

| Service | URL | Notes |
|---|---|---|
| account-svc | http://localhost:8081 | HTTP API |
| auth-svc | http://localhost:8082 | HTTP API |

### Observability infrastructure

| Tool | URL | Credentials |
|---|---|---|
| Grafana | http://localhost:3000 | admin / admin |
| Prometheus | http://localhost:9090 | — |
| Jaeger UI | http://localhost:16686 | — |
| Loki API | http://localhost:3100 | — |
| Alertmanager | http://localhost:9093 | — |
| PostgreSQL | localhost:5432 | Per-service users (see §6) |

### Internal container network

All Docker containers share the `banking-net` bridge network. Within that network, services reference each other by container name (e.g. `jaeger:4317`, `alertmanager:9093`).

When running services **locally** (not in Docker), Prometheus uses `host.docker.internal:{port}` to reach them from inside the container.

---

## 11. Observability Stack

### Overview

```
Service logs  ──► Promtail ──► Loki ──► Grafana (Explore / Dashboards)
Service traces ──► OTLP gRPC ──► Jaeger ──► Grafana (linked from Loki)
Service metrics ──► /metrics ──► Prometheus ──► Grafana + Alertmanager
```

### Metrics (Prometheus)

Every service exposes `/metrics` in Prometheus exposition format. The `pkg/middleware/metrics.go` middleware automatically records:

- `{service}_http_requests_total` — counter labelled by `method`, `path`, `status`
- `{service}_http_request_duration_seconds` — histogram for latency percentiles

Prometheus scrapes `host.docker.internal:{port}` for locally running services and `{container-name}:{port}` for containerised services. Edit `prometheus.yml` and reload:
```bash
curl -X POST http://localhost:9090/-/reload
```

### Traces (Jaeger)

Services initialise the OTel SDK via `pkg/observability/otel.go`. Every HTTP request gets a span. Spans are exported via OTLP gRPC to Jaeger at port `4317`.

Trace context is propagated between services via W3C `traceparent` headers. The `trace_id` is also injected into every structured log line for correlation.

### Logs (Loki + Promtail)

Promtail runs in **dual mode**:

1. **Docker SD** (`banking-docker` job) — automatically discovers containers whose name starts with `banking-`. Reads stdout/stderr from Docker. Use this when running services via `docker compose up`.

2. **Static file scrape** (`banking-local` job) — reads from `/var/log/banking/*.log` inside the Promtail container, which maps to `./logs/` in the monorepo. Use this when running services via `make run-*`.

The `make run-auth-svc` and `make run-account-svc` targets pipe output through `tee` to write to `./logs/auth-svc.log` and `./logs/account-svc.log` respectively.

**Pipeline stages** (applied to all log jobs):
1. JSON parse — extracts all fields from structured log lines
2. Label promotion — `level`, `trace_id` become Loki labels
3. Drop filter — `/healthz` and `/metrics` requests are silently dropped
4. Timestamp parse — uses RFC3339Nano from the log's `time` field

### Correlation (Loki ↔ Jaeger)

**Log → Trace:** In Grafana Explore → Loki, click any `trace_id` value in a log line to jump directly to the Jaeger trace.

**Trace → Log:** In Grafana Explore → Jaeger, select a trace, click a span, choose "Logs for this span" to jump to Loki filtered by `trace_id` and the span's time window.

This is configured in `infra/grafana/provisioning/datasources/prometheus.yml`:
- Jaeger datasource has `tracesToLogsV2` pointing at Loki
- Loki datasource has `derivedFields` for `trace_id` pointing at Jaeger

### Alerting

Alert rules are defined in `infra/alerting/rules/`. Prometheus evaluates them and forwards firing alerts to Alertmanager. Alertmanager config is in `alertmanager.yml`.

Current alert rules (account-svc):
- High error rate (5xx > threshold)
- High latency (P99 > threshold)
- Service down (`up == 0`)

---

## 12. Grafana Dashboards

All dashboards are provisioned automatically on `make infra-up`. No manual import needed.

### Platform Overview (`uid: platform-overview`)

URL: http://localhost:3000/d/platform-overview

A cross-service executive dashboard. Panels:
- **Executive Summary** — global RPS, error rate, P99 latency, active goroutines, service count, alert count
- **Global Trends** — request rate over time, error rate over time, P99 latency over time (all services aggregated)
- **Service Health Grid** — table of services with up/down status, error rate, RPS; bar gauge; top erroring endpoints
- **Go Runtime** — goroutines, heap alloc, GC pause, open FDs (aggregated across all services)
- **Incident Signals** — active Prometheus alerts, recent Loki error logs

The Services Health table is dynamic — it auto-discovers all services matching `up{job=~".+-svc"}`. No dashboard edits needed when adding new services.

Clicking a service name in the Services Health table navigates to the Service Overview dashboard pre-filtered to that service.

### Service Overview (`uid: service-overview`)

URL: http://localhost:3000/d/service-overview?var-service=account-svc

A deep per-service drill-down dashboard. Template variable `$service` is auto-populated from `label_values(up{job=~".+-svc"}, job)`. All panels use `job="$service"` — fully generic.

Sections:
- **Executive Summary** — RPS, error rate, P50/90/99 latency, total requests, uptime
- **Traffic & Errors** — request rate by status, error rate over time, top endpoints by volume, top endpoints by error count, recent errors table
- **Latency Analysis** — P50/P90/P99 timeseries, latency heatmap by path, top slowest endpoints (P99)
- **Endpoint Diagnostics** — per-endpoint throughput & latency, status code breakdown table
- **Go Runtime** — goroutines, heap alloc, GC cycles, GC pause, open FDs, threads (stats + timeseries)
- **Incident Signals** — active alerts, recent Loki error logs, Jaeger traces (click to open full trace)

---

## 13. Makefile Reference

Run `make help` to see all targets with descriptions.

### Build & run

| Target | Description |
|---|---|
| `make build` | Compile all service binaries to `{svc}/bin/` |
| `make run-account-svc` | Run account-svc locally, logs → `./logs/account-svc.log` |
| `make run-auth-svc` | Run auth-svc locally, logs → `./logs/auth-svc.log` |
| `make run-all` | Run both services (auth-svc in background) |
| `make dev` | Run account-svc with Air hot reload |
| `make logs-follow` | Tail all local service logs |

### Quality

| Target | Description |
|---|---|
| `make test` | Unit tests for all modules (race + cover) |
| `make test-integration` | Integration tests (requires running Postgres) |
| `make lint` | golangci-lint across all modules |
| `make fmt` | gofmt + goimports |
| `make tidy` | `go mod tidy` for all modules |

### Infrastructure

| Target | Description |
|---|---|
| `make infra-up` | Start observability stack (Jaeger, Prometheus, Grafana, Loki, Promtail, Alertmanager, Postgres) |
| `make infra-down` | Stop observability stack |
| `make infra-logs` | Tail infra container logs |
| `make services-up` | Build + start all microservices via Docker Compose |
| `make services-down` | Stop microservices |
| `make docker-up` | Full stack: `infra-up` + `services-up` |
| `make docker-down` | Stop everything |
| `make migrate` | Apply SQL migrations (requires `DB_*` env vars) |
| `make proto` | Regenerate Go from `.proto` files |

---

## 14. Docker Compose Workflow

There are two Compose files, intentionally separated:

### `docker-compose.infra.yml` — Observability infrastructure

Contains: Postgres, Jaeger, Prometheus, Alertmanager, Grafana, Loki, Promtail.

```bash
make infra-up    # start
make infra-down  # stop
```

### `docker-compose.yml` — Microservices

Contains: account-svc, auth-svc (builds from Dockerfiles).

```bash
make services-up    # build images and start
make services-down  # stop
```

### Network

Both files join the `banking-net` bridge network (defined in `docker-compose.infra.yml` as `external: false` with `name: banking-net`). Services in `docker-compose.yml` declare `banking-net` as `external: true` so they join the existing network without recreating it.

### Prometheus target switching

When services run **locally** (`make run-*`): Prometheus targets are `host.docker.internal:{port}`.
When services run **in Docker** (`make services-up`): change targets to `{container-name}:{port}` in `prometheus.yml` and reload.

---

## 15. CI/CD

### GitHub Actions (`.github/workflows/ci.yml`)

Pipeline stages:
1. **Lint** — `golangci-lint run` across all modules
2. **Test** — `go test -race -cover ./...` for all workspace members
3. **Build** — `go build` to verify compilation
4. **Security scan** — `govulncheck` for known vulnerabilities
5. **Docker build** — builds production images (distroless base)

### ArgoCD (`deploy/argocd/`)

ArgoCD Applications are declared for each service pointing at the Helm chart in `deploy/helm/{svc}/`. ArgoCD monitors the Git repo and applies changes automatically (GitOps).

### Helm (`deploy/helm/`)

Each service has a Helm chart with:
- `Deployment` — resource limits, liveness/readiness probes, graceful shutdown (`terminationGracePeriodSeconds: 30`)
- `Service` — ClusterIP
- `HorizontalPodAutoscaler` — CPU/memory based autoscaling
- `PodDisruptionBudget` — ensures at least 1 pod stays up during rolling updates

---

## 16. Coding Conventions

### Error handling

Always wrap errors with context. Use `pkg/errors` for domain errors:

```go
// Bad
return nil, err

// Good
return nil, fmt.Errorf("account_repository.GetByID: %w", err)
```

Domain errors carry an HTTP status code and a client-safe message:
```go
return nil, errors.NewNotFound("account not found")
return nil, errors.NewConflict("account already exists")
return nil, errors.NewUnauthorized("invalid credentials")
```

### HTTP handlers

Handlers should be thin — decode request → call service → encode response:

```go
func (h *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")
    account, err := h.service.GetByID(r.Context(), id)
    if err != nil {
        httpx.WriteError(w, r, err)
        return
    }
    httpx.WriteJSON(w, http.StatusOK, account)
}
```

### Context propagation

Always thread `context.Context` as the first argument through every function call. Never store context in structs.

### Logging

Use the package-level `slog` logger. Never use `fmt.Println` for application output:

```go
slog.InfoContext(ctx, "account created", "account_id", id, "user_id", userID)
slog.WarnContext(ctx, "rate limit hit", "ip", ip, "rps", rps)
slog.ErrorContext(ctx, "db query failed", "error", err)
```

The `RequestLogger` middleware automatically logs every request. Only add additional log lines for meaningful business events.

### Naming conventions

| Thing | Convention | Example |
|---|---|---|
| Packages | lowercase, no underscores | `transport`, `repository` |
| Interfaces | noun or noun+er | `AccountRepository`, `Storer` |
| Structs implementing interfaces | `{Noun}{Impl or suffix}` | `PostgresAccountRepository` |
| Constructors | `New{Type}` | `NewAccountHandler` |
| Test files | `{name}_test.go` | `account_service_test.go` |
| Integration tests | build tag `integration` | `//go:build integration` |

### Import grouping (goimports order)

```go
import (
    // stdlib
    "context"
    "fmt"

    // external
    "github.com/go-chi/chi/v5"

    // internal pkg
    "github.com/sanusi/banking/pkg/errors"

    // service-local
    "github.com/sanusi/banking/services/account-svc/internal/domain/dao"
)
```

---

## 17. Testing Strategy

### Unit tests

Location: `services/{svc}/tests/unit/`

- No external dependencies (no DB, no network).
- Repositories mocked via interfaces.
- Use `testify/assert` and `testify/mock`.

```bash
make test           # runs all unit tests with -race
```

### Integration tests

Location: `services/{svc}/tests/integration/`

Build tag: `//go:build integration`

- Requires a running PostgreSQL instance.
- Tests the full stack from repository → DB.
- Use `testcontainers-go` for a clean Postgres container per test run.

```bash
make test-integration
```

### Benchmark tests

Use standard `testing.B` benchmarks. Run:
```bash
go test -bench=. -benchmem ./...
```

### Load tests

`k6` scripts live in `deploy/k6/` (to be added). Run against staging:
```bash
k6 run deploy/k6/account-svc.js
```

---

## 18. How to Add a New Microservice

Follow this checklist to add, e.g., `payment-svc` on port `8083`.

### 1. Scaffold the service

```bash
mkdir -p services/payment-svc/{cmd/server,config,internal/{domain/{dao,dto},repository,services,transport},migrations,tests/{unit,integration}}
cd services/payment-svc
go mod init github.com/sanusi/banking/services/payment-svc
```

Add to `go.work`:
```
use (
    ./pkg
    ./services/account-svc
    ./services/auth-svc
    ./services/payment-svc   # ← add this
)
```

### 2. Copy service boilerplate

Copy and adapt from `auth-svc` or `account-svc`:
- `cmd/server/main.go` — start OTel, connect DB, build container, serve HTTP
- `cmd/server/container.go` — dependency injection
- `config/config.go` — service-specific config struct
- `internal/transport/routes.go` — chi router with middleware stack
- `.env` — local dev env vars

Set `SERVICE_NAME=payment-svc`, `HTTP_PORT=8083`.

### 3. Add to Prometheus

In `prometheus.yml`:
```yaml
- job_name: "payment-svc"
  scrape_interval: 10s
  metrics_path: /metrics
  static_configs:
    - targets: ["host.docker.internal:8083"]
      labels:
        service: payment-svc
        team: platform
```

Reload Prometheus: `curl -X POST http://localhost:9090/-/reload`

### 4. Add to Promtail (local file scraping)

In `infra/promtail/promtail-config.yml`, under the `banking-local` job's `static_configs.targets[0].__path__` (or add another entry):
```yaml
- /var/log/banking/payment-svc.log
```

Restart Promtail: `docker restart banking-promtail`

### 5. Add Makefile targets

In the root `Makefile`, add to the `SERVICES` variable and add run targets:
```makefile
SERVICES := services/account-svc services/auth-svc services/payment-svc

run-payment-svc:
    @mkdir -p logs
    cd services/payment-svc && go run ./cmd/server/... 2>&1 | tee ../../logs/payment-svc.log
```

### 6. Add Dockerfile

Copy from `services/account-svc/Dockerfile` and update the binary name.

### 7. Add to docker-compose.yml

```yaml
payment-svc:
  build:
    context: ./services/payment-svc
    dockerfile: Dockerfile
  container_name: banking-payment-svc
  env_file: ./services/payment-svc/.env
  ports:
    - "8083:8083"
  networks:
    - banking-net
  depends_on:
    - jaeger
```

### 8. Add alert rules

Copy `infra/alerting/rules/account-svc.yml` to `infra/alerting/rules/payment-svc.yml`. Update metric names and job labels.

### 9. Grafana

Both `platform-overview` and `service-overview` dashboards auto-discover services via `label_values(up{job=~".+-svc"}, job)`. As soon as Prometheus starts scraping `payment-svc`, it appears in both dashboards automatically. No dashboard edits needed.

### 10. Add JWT public key (if service validates tokens)

Copy `JWT_PUBLIC_KEY_B64` from `account-svc/.env` into `payment-svc/.env`. The same public key works for all verifying services.

---

## 19. Deployment (Kubernetes / Helm / ArgoCD)

### Helm chart structure

```
deploy/helm/account-svc/
├── Chart.yaml           # Chart metadata
├── values.yaml          # Default values (image tag, replicas, resource limits)
└── templates/
    ├── _helpers.tpl     # Named templates
    ├── deployment.yaml  # Deployment with probes + graceful shutdown
    ├── service.yaml     # ClusterIP service
    ├── hpa.yaml         # HorizontalPodAutoscaler
    └── pdb.yaml         # PodDisruptionBudget
```

### Key Kubernetes settings

- **Liveness probe**: `GET /healthz/live` — if this fails, k8s restarts the pod.
- **Readiness probe**: `GET /healthz/ready` — if this fails, k8s removes the pod from load balancer until it recovers.
- **Graceful shutdown**: `terminationGracePeriodSeconds: 30` — services handle `SIGTERM` and drain in-flight requests.
- **Resource limits**: Set CPU/memory limits in `values.yaml`. Never run without limits in production.
- **HPA**: Scales on CPU utilisation. Configure `minReplicas: 2` in production for HA.
- **PDB**: Ensures at least 1 pod remains available during node drains/rolling updates.

### Docker images

All Dockerfiles use a multi-stage build:
1. **Build stage**: `golang:1.25-alpine` — compiles the binary with `CGO_ENABLED=0`
2. **Runtime stage**: `gcr.io/distroless/static` — minimal attack surface, no shell

### ArgoCD

Each service has an `Application` manifest in `deploy/argocd/`. ArgoCD watches the Git repo and syncs automatically on push to `main`. Manual sync: `argocd app sync account-svc`.

---

## 20. Key Design Decisions & Trade-offs

### Chi vs Gin/Echo/Fiber

**Chose chi.** It composes standard `net/http` middleware without wrapping request/response types — any standard library middleware works unchanged. Gin and Echo wrap `http.Request` in custom contexts, creating vendor lock-in. Fiber uses `fasthttp` which breaks compatibility with the stdlib ecosystem (OpenTelemetry, testify HTTP helpers, etc.). chi's performance is on par with Gin for typical API workloads.

### slog vs zap vs zerolog

**Chose slog (standard library).** Since Go 1.21, `log/slog` provides structured logging with handlers, levels, and context support. Using the stdlib avoids a dependency and means log handler code can be shared with any Go project. zerolog is marginally faster but the allocation difference is immaterial at our request volumes.

### GORM vs sqlx vs raw sql

**Chose GORM with restraint.** GORM handles migrations and simple CRUD cleanly. Complex queries use `db.Raw()` to avoid magic. This gives 80% of the ergonomics of an ORM with full control over query execution. sqlx was considered but GORM's driver ecosystem and community are stronger for Postgres.

### RS256 vs HS256 JWT

**Chose RS256.** With HS256, every service that verifies tokens must also know the secret — meaning any service can forge tokens. With RS256, only auth-svc holds the private key. Compromising account-svc does not leak signing capability.

### Database-per-service vs shared database

**Chose database-per-service.** The banking domain has strict data sovereignty requirements. A shared database creates hidden coupling through schema dependencies. Each service team can evolve its schema independently, and a DB failure in one service doesn't cascade.

### Go Workspaces vs single module

**Chose Go Workspaces.** A single `go.mod` for the whole monorepo creates a single dependency graph — a change in a leaf service's dependency could force all services to upgrade simultaneously. Workspaces give independent `go.mod` per service with shared local replacements during development. CI can test each module independently.

---

*Last updated: 2026-05-27 — reflects the state of the repository at the time of initial GitHub push.*
