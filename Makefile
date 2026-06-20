# ─── Cross-platform shell ─────────────────────────────────────────────────────
# Mac/Linux : /bin/bash is always present.
# Windows   : Install Git for Windows (https://git-scm.com).
#             During install choose "Git from the command line and 3rd-party
#             software" → this adds Git's bash.exe to the system PATH.
#             NOTE: Windows System32 bash.exe is WSL — do NOT use it here.
#             Verify the right one is first with:  where git
#
# Docker-only targets (datasource-up, platform-up, services-up, monitoring-up,
# stack-up/down, *-down, *-logs) work without bash on any shell.
# bash is required for: build, test, lint, tidy, gen-keys, run-*, migrate-*
ifeq ($(OS),Windows_NT)
    # Derive bash from Git's installation to avoid picking up WSL bash.
    # e.g. C:\Program Files\Git\cmd\git.exe  →  C:\Program Files\Git\bin\bash.exe
    GIT_EXE   := $(firstword $(shell where git 2>NUL))
    GIT_ROOT  := $(if $(GIT_EXE),$(patsubst %/cmd/git.exe,%,$(subst \,/,$(GIT_EXE))),)
    GIT_BASH  := $(if $(GIT_ROOT),$(GIT_ROOT)/bin/bash.exe,)
    ifeq ($(wildcard $(GIT_BASH)),)
        # Git not found via where — fall back to bare bash.exe and hope PATH is right
        SHELL := bash.exe
    else
        SHELL := $(GIT_BASH)
    endif
else
    SHELL := /bin/bash
endif
.SHELLFLAGS := -c

.PHONY: build test test-integration lint gen-keys generate datasource-up datasource-down datasource-logs platform-up platform-down platform-logs monitoring-up monitoring-down monitoring-logs services-up services-down services-logs stack-up stack-down migrate migrate-auth migrate-account migrate-audit tidy fmt proto k6-up k6-down k6-smoke k6-load k6-stress k6-orchestration-smoke k6-orchestration-load k6-orchestration-stress k6-gateway-smoke gateway-test help

# ─── Variables ────────────────────────────────────────────────────────────────
GOWORK_FILE := go.work
SERVICES    := services/auth-svc services/account-svc services/audit-svc
PROTO_DIR   := proto

# ─── Default ──────────────────────────────────────────────────────────────────
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ─── Key generation ───────────────────────────────────────────────────────────
gen-keys: ## Generate RS256 keypair for JWT signing (copy output into services/.env)
	@echo "Generating RS256 keypair..."
	@PRIVATE=$$(openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 2>/dev/null) && \
	 PUBLIC=$$(echo "$$PRIVATE" | openssl pkey -pubout 2>/dev/null) && \
	 echo "" && \
	 echo "Add these to services/auth-svc/.env:" && \
	 echo "JWT_PRIVATE_KEY_B64=$$(echo "$$PRIVATE" | base64 | tr -d '\n')" && \
	 echo "JWT_PUBLIC_KEY_B64=$$(echo "$$PUBLIC"  | base64 | tr -d '\n')" && \
	 echo "" && \
	 echo "Add only the public key to services/account-svc/.env:" && \
	 echo "JWT_PUBLIC_KEY_B64=$$(echo "$$PUBLIC"  | base64 | tr -d '\n')"

# ─── Build ────────────────────────────────────────────────────────────────────
build: ## Build all service binaries
	@for svc in $(SERVICES); do \
		echo "→ Building $$svc ..."; \
		(cd $$svc && CGO_ENABLED=0 go build -ldflags="-w -s" -o bin/$$(basename $$svc) ./cmd/server); \
	done

# ─── Test ─────────────────────────────────────────────────────────────────────
test: ## Run unit tests for all workspace members
	go test -race -cover ./pkg/...
	@for svc in $(SERVICES); do \
		echo "→ Testing $$svc ..."; \
		(cd $$svc && go test -race -cover ./...); \
	done

test-integration: ## Run integration tests (requires running Postgres)
	@for svc in $(SERVICES); do \
		echo "→ Integration tests: $$svc ..."; \
		(cd $$svc && go test -tags=integration -race -v ./tests/integration/...); \
	done

# ─── Lint ─────────────────────────────────────────────────────────────────────
lint: ## Run golangci-lint across all modules
	golangci-lint run ./pkg/...
	@for svc in $(SERVICES); do \
		echo "→ Linting $$svc ..."; \
		(cd $$svc && golangci-lint run ./...); \
	done

# ─── Format ───────────────────────────────────────────────────────────────────
fmt: ## Format all Go source files
	gofmt -w -s ./pkg ./services
	@which goimports > /dev/null && goimports -w ./pkg ./services || true

# ─── Tidy ─────────────────────────────────────────────────────────────────────
tidy: ## Tidy go.mod / go.sum for all modules
	@for svc in $(SERVICES); do \
		echo "→ Tidy $$svc ..."; \
		(cd $$svc && go mod tidy); \
	done
	cd pkg && go mod tidy

# ─── Protobuf ─────────────────────────────────────────────────────────────────
proto: ## Generate Go code from .proto files (requires protoc + protoc-gen-go)
	@find $(PROTO_DIR) -name "*.proto" -exec \
		protoc \
			--proto_path=. \
			--go_out=. \
			--go_opt=paths=source_relative \
			--go-grpc_out=. \
			--go-grpc_opt=paths=source_relative \
			{} \;

# ─── Datasource — persistent databases (MySQL, PostgreSQL, MongoDB) ───────────
datasource-up: ## Start shared database stack
	docker compose -f datasource/docker-compose.yml up -d

datasource-down: ## Stop shared database stack
	docker compose -f datasource/docker-compose.yml down

datasource-logs: ## Tail datasource logs
	docker compose -f datasource/docker-compose.yml logs -f

# ─── Platform — shared runtime services (Redis, Flipt, NATS) ─────────────────
platform-up: ## Start platform services (Redis, Flipt feature flags, NATS messaging)
	docker compose -f platform/docker-compose.yml up -d

platform-down: ## Stop platform services
	docker compose -f platform/docker-compose.yml down

platform-logs: ## Tail platform logs
	docker compose -f platform/docker-compose.yml logs -f

# ─── Monitoring — observability infrastructure (Jaeger, Prometheus, Alertmanager, Grafana) ──────
monitoring-up: ## Start observability infrastructure
	docker compose -f monitoring/docker-compose.infra.yml up -d

monitoring-down: ## Stop observability infrastructure
	docker compose -f monitoring/docker-compose.infra.yml down

monitoring-logs: ## Tail monitoring logs
	docker compose -f monitoring/docker-compose.infra.yml logs -f

# ─── Microservices ────────────────────────────────────────────────────────────
services-up: ## Build and start all microservices (run make datasource-up + make platform-up first)
	docker compose up -d --build

services-down: ## Stop all microservices
	docker compose down

services-logs: ## Tail microservice logs
	docker compose logs -f

# ─── Full stack ───────────────────────────────────────────────────────────────
stack-up: datasource-up platform-up monitoring-up services-up ## Start everything (datasource + platform + monitoring + microservices)

stack-down: ## Stop everything
	docker compose down
	docker compose -f monitoring/docker-compose.infra.yml down
	docker compose -f platform/docker-compose.yml down
	docker compose -f datasource/docker-compose.yml down

# ─── Database migrations ──────────────────────────────────────────────────────
#
# Reads credentials from CREDENTIALS.txt / root .env:
#   auth-svc    → banking_auth     as auth_svc
#   account-svc → banking_accounts as account_svc
#
# Runs every .sql file in the service migrations/ folder in alphabetical order.
# Safe to re-run — wrap individual statements in IF NOT EXISTS where needed.
# ─────────────────────────────────────────────────────────────────────────────

AUTH_DB_HOST     ?= localhost
AUTH_DB_PORT     ?= 5432
AUTH_DB_NAME     ?= banking_auth
AUTH_DB_USER     ?= auth_svc
AUTH_DB_PASSWORD ?= auth_svc_pass_local

ACCOUNT_DB_HOST     ?= localhost
ACCOUNT_DB_PORT     ?= 5432
ACCOUNT_DB_NAME     ?= banking_accounts
ACCOUNT_DB_USER     ?= account_svc
ACCOUNT_DB_PASSWORD ?= account_svc_pass_local

AUDIT_DB_HOST     ?= localhost
AUDIT_DB_PORT     ?= 5432
AUDIT_DB_NAME     ?= banking_audits
AUDIT_DB_USER     ?= audit_svc
AUDIT_DB_PASSWORD ?= audit_svc_pass_local

migrate-auth: ## Run auth-svc SQL migrations against banking_auth
	@echo "→ Migrating auth-svc → $(AUTH_DB_HOST):$(AUTH_DB_PORT)/$(AUTH_DB_NAME)"
	@for f in $$(ls services/auth-svc/migrations/*.sql 2>/dev/null | sort); do \
		echo "  Applying $$f ..."; \
		PGPASSWORD=$(AUTH_DB_PASSWORD) psql \
			-h $(AUTH_DB_HOST) -p $(AUTH_DB_PORT) \
			-U $(AUTH_DB_USER) -d $(AUTH_DB_NAME) \
			-f $$f || exit 1; \
	done
	@echo "✓ auth-svc migrations complete"

migrate-account: ## Run account-svc SQL migrations against banking_accounts
	@echo "→ Migrating account-svc → $(ACCOUNT_DB_HOST):$(ACCOUNT_DB_PORT)/$(ACCOUNT_DB_NAME)"
	@for f in $$(ls services/account-svc/migrations/*.sql 2>/dev/null | sort); do \
		echo "  Applying $$f ..."; \
		PGPASSWORD=$(ACCOUNT_DB_PASSWORD) psql \
			-h $(ACCOUNT_DB_HOST) -p $(ACCOUNT_DB_PORT) \
			-U $(ACCOUNT_DB_USER) -d $(ACCOUNT_DB_NAME) \
			-f $$f || exit 1; \
	done
	@echo "✓ account-svc migrations complete"

migrate-audit: ## Run audit-svc SQL migrations against banking_audits
	@echo "→ Migrating audit-svc → $(AUDIT_DB_HOST):$(AUDIT_DB_PORT)/$(AUDIT_DB_NAME)"
	@for f in $$(ls services/audit-svc/migrations/*.sql 2>/dev/null | sort); do \
		echo "  Applying $$f ..."; \
		PGPASSWORD=$(AUDIT_DB_PASSWORD) psql \
			-h $(AUDIT_DB_HOST) -p $(AUDIT_DB_PORT) \
			-U $(AUDIT_DB_USER) -d $(AUDIT_DB_NAME) \
			-f $$f || exit 1; \
	done
	@echo "✓ audit-svc migrations complete"

migrate: migrate-auth migrate-account migrate-audit ## Run ALL migrations (auth + account + audit)

# ─── Performance testing — k6 ────────────────────────────────────────────────
k6-up: ## Start k6 dashboard stack (InfluxDB + Grafana at http://localhost:3001)
	docker compose -f performance-test-k6/docker-compose.yml up -d influxdb grafana

k6-down: ## Stop k6 dashboard stack
	docker compose -f performance-test-k6/docker-compose.yml down

# k6 runs entirely via Docker — no local k6 install required.
# Container-side URLs use Docker container names on banking-net.
K6_RUN      = docker compose -f performance-test-k6/docker-compose.yml run --rm k6 run
K6_GATEWAY  = -e GATEWAY_URL=http://banking-traefik
K6_DASHBOARD= -e TRAEFIK_DASHBOARD_URL=http://banking-traefik:8080
K6_AUTH     = -e AUTH_URL=http://banking-auth-svc:8082
K6_ACCOUNT  = -e ACCOUNT_URL=http://banking-account-svc:8081
K6_URLS     = $(K6_GATEWAY) $(K6_DASHBOARD) $(K6_AUTH) $(K6_ACCOUNT)

k6-smoke: ## Smoke test auth + account flows via Docker (1 VU, 1 iteration)
	$(K6_RUN) -e SCENARIO=smoke $(K6_AUTH) $(K6_ACCOUNT) /scripts/auth-flow.js
	$(K6_RUN) -e SCENARIO=smoke $(K6_AUTH) $(K6_ACCOUNT) /scripts/account-flow.js

k6-load: ## Load test auth + account flows with InfluxDB output (requires make k6-up first)
	$(K6_RUN) -e SCENARIO=load --out influxdb=http://k6-influxdb:8086/k6 $(K6_AUTH) $(K6_ACCOUNT) /scripts/auth-flow.js
	$(K6_RUN) -e SCENARIO=load --out influxdb=http://k6-influxdb:8086/k6 $(K6_AUTH) $(K6_ACCOUNT) /scripts/account-flow.js

k6-orchestration-smoke: ## Smoke test full cross-service orchestration through gateway (1 VU, 1 iteration)
	$(K6_RUN) -e SCENARIO=smoke $(K6_URLS) /scripts/orchestration-flow.js

k6-orchestration-load: ## Load test full orchestration through gateway with InfluxDB output (requires make k6-up first)
	$(K6_RUN) -e SCENARIO=load --out influxdb=http://k6-influxdb:8086/k6 $(K6_URLS) /scripts/orchestration-flow.js

k6-orchestration-stress: ## Stress test full orchestration through gateway with InfluxDB output (requires make k6-up first)
	$(K6_RUN) -e SCENARIO=stress --out influxdb=http://k6-influxdb:8086/k6 $(K6_URLS) /scripts/orchestration-flow.js

k6-gateway-smoke: ## Validate Traefik routing, security headers, and rate limiting via Docker
	$(K6_RUN) -e SCENARIO=smoke $(K6_URLS) /scripts/gateway-flow.js

k6-stress: ## Stress test auth + account flows with InfluxDB output (requires make k6-up first)
	$(K6_RUN) -e SCENARIO=stress --out influxdb=http://k6-influxdb:8086/k6 $(K6_AUTH) $(K6_ACCOUNT) /scripts/auth-flow.js
	$(K6_RUN) -e SCENARIO=stress --out influxdb=http://k6-influxdb:8086/k6 $(K6_AUTH) $(K6_ACCOUNT) /scripts/account-flow.js

# ─── Local run (logs piped to ./logs/*.log for Promtail to scrape) ───────────
#
# Usage:  make run-account-svc      (in one terminal)
#         make run-auth-svc         (in another terminal)
#
# Logs appear in Grafana → Explore → Loki within ~5 seconds.
# ─────────────────────────────────────────────────────────────────────────────

run-account-svc: ## Run account-svc locally, tee logs to ./logs/account-svc.log
	@mkdir -p logs
	@echo "→ account-svc  http://localhost:8081  (logs → ./logs/account-svc.log)"
	@set -a; [ -f services/account-svc/.env ] && . ./services/account-svc/.env; set +a; \
	 cd services/account-svc && go run ./cmd/server/... 2>&1 | tee ../../logs/account-svc.log

run-auth-svc: ## Run auth-svc locally, tee logs to ./logs/auth-svc.log
	@mkdir -p logs
	@echo "→ auth-svc  http://localhost:8082  (logs → ./logs/auth-svc.log)"
	@set -a; [ -f services/auth-svc/.env ] && . ./services/auth-svc/.env; set +a; \
	 cd services/auth-svc && go run ./cmd/server/... 2>&1 | tee ../../logs/auth-svc.log

run-audit-svc: ## Run audit-svc locally, tee logs to ./logs/audit-svc.log
	@mkdir -p logs
	@echo "→ audit-svc  http://localhost:8083  (logs → ./logs/audit-svc.log)"
	@set -a; [ -f services/audit-svc/.env ] && . ./services/audit-svc/.env; set +a; \
	 cd services/audit-svc && go run ./cmd/server/... 2>&1 | tee ../../logs/audit-svc.log

run-all: ## Run all three services locally with log capture (backgrounds auth + account, foregrounds audit)
	@mkdir -p logs
	@echo "→ Starting auth-svc, account-svc, and audit-svc locally..."
	@echo "→ Logs: ./logs/auth-svc.log  ./logs/account-svc.log  ./logs/audit-svc.log"
	@set -a; [ -f services/auth-svc/.env ] && . ./services/auth-svc/.env; set +a; \
	 cd services/auth-svc    && go run ./cmd/server/... 2>&1 | tee ../../logs/auth-svc.log &
	@set -a; [ -f services/account-svc/.env ] && . ./services/account-svc/.env; set +a; \
	 cd services/account-svc && go run ./cmd/server/... 2>&1 | tee ../../logs/account-svc.log &
	@set -a; [ -f services/audit-svc/.env ] && . ./services/audit-svc/.env; set +a; \
	 cd services/audit-svc   && go run ./cmd/server/... 2>&1 | tee ../../logs/audit-svc.log

logs-follow: ## Tail all local service logs
	@tail -f logs/*.log 2>/dev/null || echo "No log files yet. Run: make run-auth-svc or make run-account-svc"

# ─── Air (hot reload) ─────────────────────────────────────────────────────────
dev: ## Start account-svc with hot reload using Air (logs to ./logs/account-svc.log)
	@mkdir -p logs
	cd services/account-svc && air 2>&1 | tee ../../logs/account-svc.log

# ─── Generate ─────────────────────────────────────────────────────────────────
generate: proto ## Run all code generators

# ─── Traefik gateway ──────────────────────────────────────────────────────────
gateway-test: ## Run end-to-end gateway integration tests (requires make services-up)
	$(SHELL) scripts/test-gateway.sh
