.PHONY: build test lint generate datasource-up datasource-down datasource-logs monitoring-up monitoring-down monitoring-logs services-up services-down services-logs stack-up stack-down migrate migrate-auth migrate-account tidy fmt proto help

# ─── Variables ────────────────────────────────────────────────────────────────
GOWORK_FILE := go.work
SERVICES    := services/auth-svc services/account-svc
PROTO_DIR   := proto

# ─── Default ──────────────────────────────────────────────────────────────────
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

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

# ─── Datasource — shared databases (MySQL, PostgreSQL, MongoDB, Redis) ────────
datasource-up: ## Start shared database stack
	docker compose -f datasource/docker-compose.yml up -d

datasource-down: ## Stop shared database stack
	docker compose -f datasource/docker-compose.yml down

datasource-logs: ## Tail datasource logs
	docker compose -f datasource/docker-compose.yml logs -f

# ─── Monitoring — observability infrastructure (Jaeger, Prometheus, Alertmanager, Grafana) ──────
monitoring-up: ## Start observability infrastructure
	docker compose -f monitoring/docker-compose.infra.yml up -d

monitoring-down: ## Stop observability infrastructure
	docker compose -f monitoring/docker-compose.infra.yml down

monitoring-logs: ## Tail monitoring logs
	docker compose -f monitoring/docker-compose.infra.yml logs -f

# ─── Microservices ────────────────────────────────────────────────────────────
services-up: ## Build and start all microservices (requires datasource-up + monitoring-up first)
	docker compose up -d --build

services-down: ## Stop all microservices
	docker compose down

services-logs: ## Tail microservice logs
	docker compose logs -f

# ─── Full stack ───────────────────────────────────────────────────────────────
stack-up: datasource-up monitoring-up services-up ## Start everything (datasource + monitoring + microservices)

stack-down: ## Stop everything
	docker compose down
	docker compose -f monitoring/docker-compose.infra.yml down
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

migrate: migrate-auth migrate-account ## Run ALL migrations (auth + account)

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
	@set -a; [ -f .env ] && . ./.env; set +a; \
	 cd services/account-svc && go run ./cmd/server/... 2>&1 | tee ../../logs/account-svc.log

run-auth-svc: ## Run auth-svc locally, tee logs to ./logs/auth-svc.log
	@mkdir -p logs
	@echo "→ auth-svc  http://localhost:8082  (logs → ./logs/auth-svc.log)"
	@set -a; [ -f .env ] && . ./.env; set +a; \
	 cd services/auth-svc && go run ./cmd/server/... 2>&1 | tee ../../logs/auth-svc.log

run-all: ## Run both services locally with log capture (opens two background processes)
	@mkdir -p logs
	@echo "→ Starting account-svc and auth-svc locally..."
	@echo "→ Logs: ./logs/account-svc.log and ./logs/auth-svc.log"
	@set -a; [ -f .env ] && . ./.env; set +a; \
	 cd services/auth-svc    && go run ./cmd/server/... 2>&1 | tee ../../logs/auth-svc.log &
	@set -a; [ -f .env ] && . ./.env; set +a; \
	 cd services/account-svc && go run ./cmd/server/... 2>&1 | tee ../../logs/account-svc.log

logs-follow: ## Tail all local service logs
	@tail -f logs/*.log 2>/dev/null || echo "No log files yet. Run: make run-auth-svc or make run-account-svc"

# ─── Air (hot reload) ─────────────────────────────────────────────────────────
dev: ## Start account-svc with hot reload using Air (logs to ./logs/account-svc.log)
	@mkdir -p logs
	cd services/account-svc && air 2>&1 | tee ../../logs/account-svc.log

# ─── Generate ─────────────────────────────────────────────────────────────────
generate: proto ## Run all code generators
