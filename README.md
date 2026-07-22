# Golang Banking Platform

A production-grade banking platform built as a Go microservices monorepo — RS256 JWT auth, account ledgering, payment orchestration, notifications, audit logging, and KYC verification, with a full observability stack.

## Architecture

```
Transport (HTTP handlers)
    ↓
Services (business logic)
    ↓
Repository (interface)
    ↓
DAO (database structs)
```

- `pkg/` — shared Go workspace module (`httpx`, `middleware`, `audit`, `errors`, `observability`, ...) — never imports a service
- Services never import each other — all inter-service communication is over HTTP or NATS
- Each service owns its own Postgres database
- `banking-net` Docker bridge network joins all containers
- [`CONTEXT-MAP.md`](CONTEXT-MAP.md) documents each service's bounded context and cross-service relationships

## Services

| Service | Port | Role |
|---|---|---|
| `auth-svc` | 8082 | RS256 JWT issuance, refresh, logout, API key management |
| `account-svc` | 8081 | Account CRUD, credit/debit, balance |
| `audit-svc` | 8083 | NATS consumer → Postgres audit log |
| `notification-svc` | 8084 | Multi-channel notification delivery, templates, scheduler |
| `payment-svc` | 8085 | Transaction orchestration across payment products |
| `kyc-svc` | — | Customer identity verification (KTP OCR extraction and scoring) |

## Port Scheme

| Range | Owner |
|---|---|
| `80` | Traefik gateway — single HTTP entry point |
| `8080` | Traefik dashboard (dev only) |
| `808x` | Microservices (direct access) |
| `900x` | Monitoring — Grafana=9000, Prometheus=9001, Alertmanager=9002, Jaeger=9003, Loki=9004 |
| `905x` | Platform — Redis=9050, Flipt UI=9051, Flipt gRPC=9052, NATS=9053, NATS dashboard=9054, Metabase=9055 |
| `4317/4318` | OTLP (standard wire protocol) |

## Getting Started

Prerequisites:
- Go 1.26+
- Docker Desktop
- Git for Windows (provides `bash.exe`, required for Makefile targets on Windows)

```bash
# start datasource, platform, and monitoring stacks
docker compose -f datasource/docker-compose.yml up -d
docker compose -f platform/docker-compose.yml up -d
docker compose -f monitoring/docker-compose.yml up -d

# start the services
docker compose up -d
```

Full fresh-machine setup, secrets handling, and troubleshooting notes live in [`HANDOFF.md`](HANDOFF.md).

## Development Conventions

Coding conventions, error handling, HTTP response helpers, logging, and import ordering are defined in [`CLAUDE.md`](CLAUDE.md).

## Testing

```bash
make test              # unit tests
make test-integration  # integration tests (build tag: integration)
```

## Observability

Grafana, Prometheus, Alertmanager, Jaeger, and Loki are provisioned under `monitoring/`.

