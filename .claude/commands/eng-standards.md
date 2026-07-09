# Microservice Standards

Define the folder structure, layering, naming, error handling, API, testing, and code style conventions for all microservices in this repository. Derived from `auth-svc`, `account-svc`, and `audit-svc`. Apply when building, reviewing, or extending any service.

## When to Activate

- Creating a new microservice or backend service
- Reviewing a PR that adds or changes service code
- Asking "how should I structure this service?"
- Checking whether existing code follows project conventions
- Onboarding to a service for the first time

---

## 1. Canonical Folder Structure

Every service follows this layout exactly. Do not invent new top-level folders.

```
services/{service-name}/
├── cmd/server/
│   ├── main.go           ← entry point only: config, logger, container, server, shutdown
│   ├── container.go      ← dependency wiring (DI): build everything, return container struct
│   └── migrate.go        ← embedded SQL migration runner (runs at startup)
├── config/
│   └── config.go         ← typed config struct; load from env; fail-fast on missing required vars
├── internal/
│   ├── domain/
│   │   ├── dao/          ← database structs with GORM tags; one file per entity
│   │   └── dto/          ← request/response types; one file per domain area
│   ├── services/         ← business logic; one file per service; test file alongside
│   ├── repository/       ← storage interfaces + implementations; one file per interface + impl
│   └── transport/
│       ├── routes.go     ← chi router setup; middleware chain registration
│       ├── {noun}_handler.go  ← one handler file per domain group
│       └── errors.go     ← service-specific HTTP error helpers (if needed)
├── migrations/
│   ├── {NNN}_{description}.up.sql
│   ├── {NNN}_{description}.down.sql
│   └── migrations.go     ← embed directive for migration files
├── tests/
│   ├── integration/      ← build tag: //go:build integration
│   └── unit/             ← package unit; mocks.go, helpers.go, {name}_test.go
├── Dockerfile
├── go.mod
├── go.sum
├── .env.example
└── Makefile
```

**Invariants:**
- `internal/` is never imported by other services (Go visibility enforced).
- `cmd/server/` contains only wiring — no business logic.
- `config/` contains only env-var loading — no business logic.
- There is no `response.go` inside `transport/` — all HTTP helpers live in `pkg/httpx`.

---

## 2. Layering

```
Transport (HTTP handlers)
    ↓  calls
Service (business logic, orchestration)
    ↓  calls
Repository Interface (storage abstraction)
    ↓  implemented by
Repository Implementation (Postgres, Redis, Memory)
    ↓  uses
DAO (database/wire structs)
```

**Rules:**
- A layer only imports the layer immediately below it.
- Handlers never access repositories directly.
- Services never import `transport/`.
- `pkg/` is import-only — services import it, it never imports services.
- No circular imports anywhere in the workspace.

---

## 3. Service Boundaries

- Each service owns exactly one database / schema.
- Services never access another service's database, even if it is on the same host.
- Inter-service communication is HTTP only (no shared memory, no gRPC at current stage).
- A service's `internal/` package is invisible to other services by Go convention.

---

## 4. Dependency Injection Pattern

```go
// cmd/server/container.go
type container struct {
    server  *http.Server
    otel    *observability.Provider
    nc      *nats.Conn
    // add other resources that need cleanup
}

func build(cfg *config.Config) (*container, error) {
    // 1. OTEL bootstrap
    // 2. Parse crypto keys
    // 3. Run DB migrations
    // 4. Connect to Postgres
    // 5. Connect to Redis (if used)
    // 6. Build repositories / stores
    // 7. Build services
    // 8. Build external clients (feature flags, audit publisher)
    // 9. Build router + middleware chain
    // 10. Create http.Server
    return &container{...}, nil
}
```

No global state. No `init()` side effects. All dependencies threaded explicitly through constructors.

---

## 5. Config Pattern

```go
// config/config.go
type Config struct {
    // Required — validate at startup
    DBHost     string
    DBPassword string
    // Optional with defaults
    Port       string // default "8082"
    LogLevel   string // default "info"
}

func Load() (*Config, error) {
    loadDotEnv() // custom loader, not godotenv
    cfg := &Config{
        DBHost:     mustEnv("DB_HOST"),
        DBPassword: mustEnv("DB_PASSWORD"),
        Port:       envOrDefault("PORT", "8082"),
    }
    return cfg, cfg.validate()
}

func (c *Config) validate() error {
    // Return error if any required field is empty
}
```

**Rules:**
- Required vars call `mustEnv()` (panics or returns error on missing).
- Optional vars use `envOrDefault()`.
- `validate()` is called in `Load()`; the service refuses to start with invalid config.
- Never access `os.Getenv` outside `config/config.go`.

---

## 6. Error Handling

### Wrapping pattern
```go
// Always wrap with caller context
return nil, fmt.Errorf("account_repository.GetByID: %w", err)
return nil, fmt.Errorf("auth_service.Login: %w", err)
```

### Domain errors (from `pkg/errors`)
```go
return nil, errors.NewNotFound("account not found")
return nil, errors.NewConflict("username already exists")
return nil, errors.NewUnauthorized("invalid credentials")
return nil, errors.NewValidation("email is required")
```

### Handler error response
```go
// Never manually write error JSON in handlers
httpx.WriteError(w, r, err)         // maps domain error → correct HTTP status
httpx.WriteHTTPError(w, r, httpErr) // explicit status + error code
```

### Rules
- Never swallow errors silently unless the operation is explicitly fire-and-forget (audit events only).
- Never return raw `errors.New("something failed")` from service or repository layer — always wrap with context.
- `pkg/errors` domain types are the only way to communicate business-rule violations across layers.

---

## 7. HTTP Handler Pattern

Handlers must be thin. No business logic in handlers.

```go
func (h *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
    id := chi.URLParam(r, "id")

    account, err := h.service.GetByID(r.Context(), id)
    if err != nil {
        httpx.WriteError(w, r, err)
        return
    }

    httpx.WriteSuccess(w, r, account)
}
```

**The only things a handler may do:**
1. Extract path params / query params / headers.
2. Decode + validate request body via `httpx.DecodeJSON`.
3. Call exactly one service method.
4. Map the result to an HTTP response via `pkg/httpx`.
5. Fire a fire-and-forget audit goroutine (if applicable).

---

## 8. HTTP Response Conventions

Always use `pkg/httpx`. Never write `w.WriteHeader` or `json.Encode` directly in handlers.

```go
httpx.WriteSuccess(w, r, data)           // 200
httpx.WriteCreated(w, r, data)           // 201
httpx.WriteNoContent(w)                  // 204
httpx.WriteError(w, r, err)              // domain error → correct HTTP status
httpx.WriteHTTPError(w, r, httpErr)      // explicit status + error code
httpx.WriteValidationError(w, r, err)    // 422 with field-level errors
httpx.DecodeJSON(r, &req)                // decode + validate request body
```

**Response envelope:**
```json
// Success
{"data": <payload>, "request_id": "<uuid>"}

// Error
{"error": "<ERROR_CODE>", "message": "<string>", "request_id": "<uuid>"}

// Validation
{"error": "VALIDATION_ERROR", "details": [{"field": "...", "message": "..."}], "request_id": "<uuid>"}
```

---

## 9. Repository Pattern

```go
// Interface in repository/{entity}_repository.go
type AccountRepository interface {
    GetByID(ctx context.Context, id string) (*dao.Account, error)
    Create(ctx context.Context, account *dao.Account) error
    Update(ctx context.Context, account *dao.Account) error
    Delete(ctx context.Context, id string) error
}

// Implementation in repository/postgres_{entity}_repository.go
type PostgresAccountRepository struct {
    db *gorm.DB
}

func NewPostgresAccountRepository(db *gorm.DB) *PostgresAccountRepository {
    return &PostgresAccountRepository{db: db}
}
```

**Rules:**
- Every storage backend is hidden behind an interface.
- Constructors accept `*gorm.DB` (or `*redis.Client`), not connection strings.
- Repository methods wrap errors: `fmt.Errorf("account_repository.GetByID: %w", err)`.
- Sentinel errors (`ErrNotFound`, `ErrRevoked`, etc.) are defined in the interface file.
- Never call GORM directly from service layer — always through the interface.

---

## 10. Naming Conventions

| Thing | Convention | Example |
|---|---|---|
| Interfaces | Noun or Noun+er | `AccountRepository`, `TokenStore`, `Publisher` |
| Postgres implementation | `Postgres{Noun}` | `PostgresAccountRepository` |
| Redis implementation | `Redis{Noun}` | `RedisTokenStore` |
| Memory implementation | `Memory{Noun}` | `MemoryTokenStore` |
| Constructors | `New{Type}` | `NewAccountHandler`, `NewAuthService` |
| Handlers | `{Noun}Handler` | `AccountHandler`, `AuthHandler` |
| Services | `{Noun}Service` | `AuthService`, `APIKeyService` |
| DTOs (request) | `{Noun}Request` | `LoginRequest`, `CreateAccountRequest` |
| DTOs (response) | `{Noun}Response` | `LoginResponse`, `AccountResponse` |
| DAOs | Singular noun | `Account`, `User`, `RefreshToken` |
| Integration tests | build tag `integration` | `//go:build integration` |
| Table-driven tests | `TestXxx_*` sub-tests | `TestAuthService_Login_ValidCredentials` |

---

## 11. Import Order

Use `goimports`. Four groups, separated by blank lines:

```go
import (
    // 1. stdlib
    "context"
    "fmt"
    "net/http"

    // 2. external packages
    "github.com/go-chi/chi/v5"
    "github.com/google/uuid"

    // 3. internal pkg (shared module)
    "github.com/sanusi/banking/pkg/errors"
    "github.com/sanusi/banking/pkg/httpx"

    // 4. service-local packages (same service)
    "github.com/sanusi/banking/services/account-svc/internal/domain/dao"
    "github.com/sanusi/banking/services/account-svc/internal/repository"
)
```

---

## 12. Logging

Use `slog` everywhere. Never `fmt.Println`, never `log.Printf`.

```go
slog.InfoContext(ctx, "account created", "account_id", id)
slog.WarnContext(ctx, "redis cache miss", "key", hash)
slog.ErrorContext(ctx, "db query failed", "error", err, "account_id", id)
```

**Rules:**
- Always use `Context` variants (`InfoContext`, `WarnContext`, `ErrorContext`) so OTEL trace IDs propagate.
- Key names are snake_case strings.
- Never log secrets, tokens, raw API keys, or passwords.
- Error level for infrastructure failures; Warn for degraded-mode fallbacks; Info for significant business events.

---

## 13. Context Rules

- Context is always the first argument: `func (s *AuthService) Login(ctx context.Context, ...) (...)`.
- Context is never stored in structs.
- Never use `context.Background()` inside a handler — propagate `r.Context()`.
- `context.Background()` is acceptable only in fire-and-forget goroutines (audit events).

---

## 14. Audit Events

```go
// Fire-and-forget — never block the handler
go func() {
    _ = h.audit.Publish(context.Background(), pkgaudit.AuditEvent{
        Action:     pkgaudit.ActionAuthLogin,
        Status:     pkgaudit.StatusSuccess,
        ActorType:  "user",
        ActorID:    userID,
        ResourceID: accountID,
        Metadata:   map[string]any{"ip": r.RemoteAddr},
    })
}()
```

**Rules:**
- Audit events are always fire-and-forget.
- Audit failure must never propagate to the user-facing response.
- Always use constants from `pkg/audit` for `Action` and `Status` values.
- `ActorID` must be the resolved user ID or service account ID — never a raw token or hash.

---

## 15. Testing Standards

See [[eng-testing]] skill (`/eng-testing`) for TDD workflow, mocks, helpers, and coverage targets. Summary: tests live in `services/{name}/tests/unit/` and `tests/integration/`, never alongside source.

---

## 16. What to Avoid

| Anti-pattern | Reason |
|---|---|
| Heavy frameworks (Gin, Echo, Fiber) | chi + stdlib is sufficient; avoids framework lock-in |
| Global mutable state | Makes testing unreliable; hidden coupling |
| `init()` side effects | Unpredictable execution order; hard to test |
| Fat controllers | Business logic in handlers is untestable without HTTP context |
| Circular imports | Go compilation error; signals wrong architecture |
| `fmt.Println` for app output | Use slog — structured, levelled, context-aware |
| Local `response.go` in transport | All HTTP helpers live in `pkg/httpx` |
| Cross-service DB access | Breaks service isolation; creates hidden coupling |
| Committing `.env` or `CREDENTIALS.txt` | Security violation |
| `go work edit -dropuse` in Dockerfile | Silently breaks when new services are added |
| `sed` to write base64 RSA keys | Corrupts `+`, `/`, `=` characters; use Python |

---

## 17. Service Implementation Checklist

Use this when creating a new service or reviewing an existing one:

- [ ] Folder structure matches canonical layout
- [ ] `config.go` validates all required vars at startup
- [ ] `container.go` wires all dependencies; no business logic
- [ ] Layering respected: transport → service → repository → DAO
- [ ] All responses use `pkg/httpx`
- [ ] All errors wrapped with `fmt.Errorf("layer.Method: %w", err)`
- [ ] Domain errors use `pkg/errors` types
- [ ] Logging uses `slog` with context variants
- [ ] Audit events are fire-and-forget
- [ ] `/healthz/live` and `/healthz/ready` endpoints present
- [ ] `/metrics` endpoint present
- [ ] DB migrations embedded and run at startup
- [ ] `.env.example` documents all env vars
- [ ] Unit tests for all service methods
- [ ] No secrets committed
- [ ] Port follows the scheme defined in CLAUDE.md
