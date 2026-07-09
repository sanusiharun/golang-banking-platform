# Testing Standards

Canonical TDD workflow and test structure for all services in this repository. Supersedes any "test file alongside source" guidance — this repo puts tests in `tests/`.

## When to Activate

- Writing a new service method or handler (write the test first)
- Adding test coverage to existing code
- Reviewing whether a PR includes adequate tests
- Setting up a new service's test scaffolding

---

## 1. RED-GREEN-REFACTOR

```
RED      → write a failing test for the behavior you want
GREEN    → write the minimum code to pass it
REFACTOR → clean up, keep tests green
```

Write the test before the implementation. Don't write both in one pass.

---

## 2. Folder Structure

Tests never live alongside source. Every service uses:

```
services/{service-name}/
├── internal/...              ← production code only, no _test.go files here
└── tests/
    ├── unit/
    │   ├── mocks.go           ← exported mocks: Mock{Interface}
    │   ├── helpers.go         ← exported helpers: New{Type}, t.Helper()
    │   └── {name}_service_test.go
    └── integration/           ← //go:build integration
```

- Single `package unit` for all unit tests in a service — lets tests import any `internal/` package without circular deps.
- One test file per service/domain: `auth_service_test.go`, `apikey_service_test.go`.

---

## 3. Mocks — `tests/unit/mocks.go`

```go
type MockUserRepo struct {
    User *dao.User
    Err  error

    FindByUsernameCalls int
}

func (m *MockUserRepo) FindByUsername(_ context.Context, _ string) (*dao.User, error) {
    m.FindByUsernameCalls++
    return m.User, m.Err
}

var _ repository.UserRepository = (*MockUserRepo)(nil)
```

- Name: `Mock{InterfaceName}`, all fields exported.
- Add call counters when a test needs to verify interaction, not just outcome.
- `var _ Interface = (*Mock)(nil)` enforces the mock satisfies the interface.

---

## 4. Helpers — `tests/unit/helpers.go`

```go
func NewTestRSAKey(t *testing.T) *rsa.PrivateKey {
    t.Helper()
    key, _ := rsa.GenerateKey(rand.Reader, 2048)
    return key
}
```

Centralize here — never duplicate a helper across test files.

---

## 5. Table-Driven Tests

```go
func TestAuthService_Login(t *testing.T) {
    tests := []struct {
        name     string
        username string
        password string
        wantErr  bool
    }{
        {"valid credentials", "alice", "correct", false},
        {"wrong password", "alice", "wrong", true},
        {"unknown user", "nobody", "anything", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // ...
        })
    }
}
```

Naming: `Test{Function}_{Scenario}` for standalone tests, `t.Run("scenario", ...)` for table entries.

---

## 6. Integration Tests

- Build tag `//go:build integration`, in `tests/integration/`.
- Run against real Postgres/Redis (the local Docker stack) — no mocks for DB logic.

---

## 7. Coverage Targets

| Code Type | Target |
|---|---|
| Service business logic | 90%+ |
| Repository (mocked in unit; DB covered by integration) | 85%+ |
| Handlers (httptest) | 80%+ |
| DAO/DTOs, config/init | 0% acceptable |

```bash
go test -coverprofile=coverage.out ./services/{name}/...
go tool cover -func=coverage.out
```

---

## 8. What Not to Test

- `pkg/httpx` response helpers — trivial wrappers.
- `container.go` wiring — verified by running the service.
- Config validation — one or two cases only.

---

## 9. Checklist for New Tests

- [ ] `tests/unit/mocks.go` and `tests/unit/helpers.go` exist for the service
- [ ] Mocks exported, satisfy their interface (`var _ Interface = ...`)
- [ ] `Test{Function}_{Scenario}` naming
- [ ] Happy path + every error path covered
- [ ] `t.Helper()` in all helpers
- [ ] `go test ./services/{name}/tests/unit` passes