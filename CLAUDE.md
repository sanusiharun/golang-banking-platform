# Golang Banking Platform — CLAUDE.md

> Coding rules and conventions only. Full project context, setup, ports, and next steps are in **HANDOFF.md**.

---

## Services & Ports

| Service | Port | Role |
|---|---|---|
| `auth-svc` | 8082 | RS256 JWT issuance, refresh, logout, API key mgmt |
| `account-svc` | 8081 | Account CRUD, credit/debit, balance |
| `audit-svc` | 8083 | NATS consumer → Postgres audit log (scaffolded, not yet wired) |

**Port scheme (never change):**
- `808x` — microservices
- `900x` — monitoring (Grafana=9000, Prometheus=9001, Alertmanager=9002, Jaeger=9003, Loki=9004)
- `905x` — platform (Redis=9050, Flipt UI=9051, Flipt gRPC=9052, NATS=9053, NATS dashboard=9054, Metabase=9055)
- `4317/4318` — OTLP (never change)

---

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

- `pkg/` — shared module, never imports any service
- Services never import each other — inter-service via HTTP only
- `banking-net` Docker bridge network joins all containers
- Each service owns its own Postgres database (`banking_auth`, `banking_accounts`, `banking_audits`)

---

## Coding Conventions

### Errors
```go
return nil, fmt.Errorf("account_repository.GetByID: %w", err)

// Domain errors from pkg/errors
return nil, errors.NewNotFound("account not found")
return nil, errors.NewConflict("account already exists")
return nil, errors.NewUnauthorized("invalid credentials")
```

### Handlers — keep thin
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

### HTTP responses — always use pkg/httpx
```go
httpx.WriteSuccess(w, r, data)         // 200
httpx.WriteCreated(w, r, data)         // 201
httpx.WriteNoContent(w)                // 204
httpx.WriteError(w, r, err)            // domain error → correct status
httpx.WriteHTTPError(w, r, httpErr)    // explicit status + code
httpx.WriteValidationError(w, r, err)  // 422 validation errors
httpx.DecodeJSON(r, &req)              // decode + validate body
```

### Logging — slog only, never fmt.Println
```go
slog.InfoContext(ctx, "account created", "account_id", id)
slog.WarnContext(ctx, "db query failed", "error", err)
```

### Context — always first argument, never stored in structs

### Audit events — fire-and-forget, never block the handler
```go
go func() {
    _ = h.audit.Publish(context.Background(), pkgaudit.AuditEvent{...})
}()
```

---

## What to Avoid

- ❌ Heavy frameworks (Gin, Echo, Fiber) — use `chi` + stdlib
- ❌ Global mutable state, `init()` side effects
- ❌ Fat controllers — business logic belongs in service layer
- ❌ Circular imports — `pkg/` never imports services
- ❌ `fmt.Println` for app output — use `slog`
- ❌ Local `response.go` in transport — all helpers live in `pkg/httpx`
- ❌ Cross-service DB access — each service owns its DB
- ❌ Committing `.env` files or `CREDENTIALS.txt`
- ❌ `go work edit -dropuse` in Docker — fragile
- ❌ `sed` to write base64 RSA keys — corrupts `+`, `/`, `=`; use Python

---

## Import Order (goimports)
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

## Naming
| Thing | Convention | Example |
|---|---|---|
| Interfaces | noun or noun+er | `AccountRepository` |
| Implementations | `Postgres{Noun}` | `PostgresAccountRepository` |
| Constructors | `New{Type}` | `NewAccountHandler` |
| Integration tests | build tag `integration` | `//go:build integration` |

---

## Git Commits
- No `Co-Authored-By` line
- Always update `HANDOFF.md` and commit it together with the work
