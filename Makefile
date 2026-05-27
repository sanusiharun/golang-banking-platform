.PHONY: build test lint generate docker-up docker-down infra-up infra-down infra-logs services-up services-down services-logs migrate tidy fmt proto help

# ─── Variables ────────────────────────────────────────────────────────────────
GOWORK_FILE := go.work
SERVICES    := services/account-svc
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

# ─── Docker — infrastructure (Jaeger, Prometheus, Alertmanager, Grafana) ──────
infra-up: ## Start observability infrastructure
	docker compose -f docker-compose.infra.yml up -d

infra-down: ## Stop observability infrastructure
	docker compose -f docker-compose.infra.yml down

infra-logs: ## Tail infra logs
	docker compose -f docker-compose.infra.yml logs -f

# ─── Docker — microservices ────────────────────────────────────────────────────
services-up: ## Build and start all microservices (requires infra-up first)
	docker compose up -d --build

services-down: ## Stop all microservices
	docker compose down

services-logs: ## Tail microservice logs
	docker compose logs -f

# ─── Docker — full stack ───────────────────────────────────────────────────────
docker-up: infra-up services-up ## Start everything (infra + microservices)

docker-down: ## Stop everything
	docker compose down
	docker compose -f docker-compose.infra.yml down

# ─── Database ─────────────────────────────────────────────────────────────────
migrate: ## Apply SQL migrations (requires psql in PATH and DB vars set)
	@echo "→ Running migrations on $(DB_HOST):$(DB_PORT)/$(DB_NAME) ..."
	@for f in services/account-svc/internal/infrastructure/postgres/migrations/*.sql; do \
		echo "  Applying $$f ..."; \
		PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d $(DB_NAME) -f $$f; \
	done

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
	cd services/account-svc && go run ./cmd/server/... 2>&1 | tee ../../logs/account-svc.log

run-auth-svc: ## Run auth-svc locally, tee logs to ./logs/auth-svc.log
	@mkdir -p logs
	@echo "→ auth-svc  http://localhost:8082  (logs → ./logs/auth-svc.log)"
	cd services/auth-svc && go run ./cmd/server/... 2>&1 | tee ../../logs/auth-svc.log

run-all: ## Run both services locally with log capture (opens two background processes)
	@mkdir -p logs
	@echo "→ Starting account-svc and auth-svc locally..."
	@echo "→ Logs: ./logs/account-svc.log and ./logs/auth-svc.log"
	cd services/auth-svc    && go run ./cmd/server/... 2>&1 | tee ../../logs/auth-svc.log &
	cd services/account-svc && go run ./cmd/server/... 2>&1 | tee ../../logs/account-svc.log

logs-follow: ## Tail all local service logs
	@tail -f logs/*.log 2>/dev/null || echo "No log files yet. Run: make run-auth-svc or make run-account-svc"

# ─── Air (hot reload) ─────────────────────────────────────────────────────────
dev: ## Start account-svc with hot reload using Air (logs to ./logs/account-svc.log)
	@mkdir -p logs
	cd services/account-svc && air 2>&1 | tee ../../logs/account-svc.log

# ─── Generate ─────────────────────────────────────────────────────────────────
generate: proto ## Run all code generators
