# Go Testing Standards — Golang Banking Platform

Unified testing strategy for all services in this repository. Enforces clean test structure, consistent patterns, and high coverage targets.

---

## Folder Structure

### Standard Layout

Each service follows this test structure:

```
service-name/
├── cmd/
├── config/
├── internal/
│   ├── domain/
│   ├── repository/
│   ├── services/
│   └── transport/
├── migrations/
├── tests/              ← All unit tests live here
│   ├── unit/
│   │   ├── helpers.go                  (test utilities)
│   │   ├── mocks.go                    (mock implementations)
│   │   ├── auth_service_test.go        (auth service tests)
│   │   ├── apikey_service_test.go      (api key service tests)
│   │   └── ...
│   └── integration/    (future: integration tests)
├── .env
├── Dockerfile
├── Makefile
└── go.mod
```

**Key points:**
- All unit tests live in `tests/unit/` package
- Shared mocks and helpers live in `mocks.go` and `helpers.go`
- Each service or domain gets its own test file: `{name}_test.go`
- Never put test files alongside source (`internal/**/*_test.go`)

---

## Package Structure

All unit tests use a single `package unit` declaration:

```go
// services/auth-svc/tests/unit/auth_service_test.go
package unit

import (
    "context"
    "testing"
    // ... other imports
)

// Test functions defined here
```

**Benefits:**
- Clear separation: `internal` contains production code, `tests/unit` contains tests
- Tests can import any internal package without circular dependencies
- Easy to distinguish test packages from production packages

---

## Mock Objects

### Pattern: Exported Mocks in `mocks.go`

Create a `mocks.go` file containing all mock implementations for your service:

```go
// tests/unit/mocks.go
package unit

import (
    "context"
    "github.com/sanusi/banking/services/auth-svc/internal/repository"
    "github.com/sanusi/banking/services/auth-svc/internal/domain/dao"
)

// ── Mock UserRepository ────────────────────────────────────

type MockUserRepo struct {
    User *dao.User
    Err  error

    FindByUsernameCalls int
    FindByIDCalls       int
}

func (m *MockUserRepo) FindByUsername(_ context.Context, _ string) (*dao.User, error) {
    m.FindByUsernameCalls++
    return m.User, m.Err
}

func (m *MockUserRepo) FindByID(_ context.Context, _ string) (*dao.User, error) {
    m.FindByIDCalls++
    return m.User, m.Err
}

var _ repository.UserRepository = (*MockUserRepo)(nil)
```

**Naming:**
- `Mock{InterfaceName}` — e.g., `MockUserRepo` for `UserRepository` interface
- All fields exported (capitalized)
- Include call counters for behavior verification
- Use `var _ Interface = (*Mock)(nil)` to enforce implementation

### Mock Behavior

```go
// In tests, configure mocks by setting fields:
mock := &MockUserRepo{
    User: &dao.User{ID: "usr-001", IsActive: true},
    Err:  nil,
}

// For error cases:
mock := &MockUserRepo{
    Err: repository.ErrUserNotFound,
}

// Verify interactions:
if mock.FindByUsernameCalls != 1 {
    t.Errorf("expected 1 call, got %d", mock.FindByUsernameCalls)
}
```

---

## Test Helpers

### Pattern: Exported Helpers in `helpers.go`

Create a `helpers.go` file containing reusable test utilities:

```go
// tests/unit/helpers.go
package unit

import (
    "crypto/rand"
    "crypto/rsa"
    "testing"
    "golang.org/x/crypto/bcrypt"
    "github.com/sanusi/banking/services/auth-svc/internal/services"
)

// NewTestRSAKey generates a test RSA private key
func NewTestRSAKey(t *testing.T) *rsa.PrivateKey {
    t.Helper()
    key, err := rsa.GenerateKey(rand.Reader, 2048)
    if err != nil {
        t.Fatalf("generate RSA key: %v", err)
    }
    return key
}

// HashPassword creates a bcrypt hash for testing
func HashPassword(t *testing.T, password string) string {
    t.Helper()
    h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
    if err != nil {
        t.Fatalf("bcrypt: %v", err)
    }
    return string(h)
}

// NewTestAuthService creates a configured service for testing
func NewTestAuthService(t *testing.T, userRepo repository.UserRepository, tokenStore repository.TokenStore) services.AuthService {
    t.Helper()
    key := NewTestRSAKey(t)
    return services.NewAuthService(userRepo, tokenStore, services.AuthConfig{
        PrivateKey:      key,
        Issuer:          "test",
        AccessTokenTTL:  15 * time.Minute,
        RefreshTokenTTL: 24 * time.Hour,
        BCryptCost:      bcrypt.MinCost,
    })
}
```

**Naming conventions:**
- `New{Type}` — constructors, e.g., `NewTestRSAKey`, `NewTestAuthService`
- `{Action}` — utility functions, e.g., `HashPassword`
- Always use `t.Helper()` to mark helper functions
- Document with comments above each function

---

## Writing Tests

### Table-Driven Pattern

Use table-driven tests for comprehensive coverage:

```go
func TestLogin(t *testing.T) {
    tests := []struct {
        name      string
        password  string
        isActive  bool
        setupMock func(*MockUserRepo)
        wantErr   error
    }{
        {
            name:     "success with valid credentials",
            password: "Secret@123",
            isActive: true,
            setupMock: func(m *MockUserRepo) {
                m.User = &dao.User{
                    ID:           "usr-001",
                    Username:     "admin",
                    PasswordHash: HashPassword(t, "Secret@123"),
                    IsActive:     true,
                }
            },
            wantErr: nil,
        },
        {
            name:     "wrong password",
            password: "wrong",
            isActive: true,
            setupMock: func(m *MockUserRepo) {
                m.User = &dao.User{
                    ID:           "usr-001",
                    PasswordHash: HashPassword(t, "correct"),
                    IsActive:     true,
                }
            },
            wantErr: services.ErrInvalidCredentials,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mock := &MockUserRepo{}
            tt.setupMock(mock)
            
            svc := NewTestAuthService(t, mock, &MockTokenStore{})
            resp, err := svc.Login(context.Background(), &dto.LoginRequest{
                Username: "admin",
                Password: tt.password,
            })

            if !errors.Is(err, tt.wantErr) {
                t.Errorf("expected %v, got %v", tt.wantErr, err)
            }
            if tt.wantErr == nil && resp == nil {
                t.Error("expected response, got nil")
            }
        })
    }
}
```

### Single-Scenario Tests

For complex scenarios with many assertions, use focused single tests:

```go
func TestLogin_Success(t *testing.T) {
    password := "Secret@123"
    user := &dao.User{
        ID:           "usr-001",
        TenantID:     "tenant-1",
        Username:     "admin",
        PasswordHash: HashPassword(t, password),
        Roles:        dao.StringArray{"ADMIN"},
        IsActive:     true,
    }

    userRepo := &MockUserRepo{User: user}
    tokenStore := &MockTokenStore{}
    svc := NewTestAuthService(t, userRepo, tokenStore)

    resp, err := svc.Login(context.Background(), &dto.LoginRequest{
        Username: "admin",
        Password: password,
    })

    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if resp.AccessToken == "" {
        t.Error("expected non-empty access token")
    }
    if resp.RefreshToken == "" {
        t.Error("expected non-empty refresh token")
    }
    if resp.UserID != user.ID {
        t.Errorf("expected user ID %q, got %q", user.ID, resp.UserID)
    }
    if userRepo.FindByUsernameCalls != 1 {
        t.Errorf("expected 1 FindByUsername call, got %d", userRepo.FindByUsernameCalls)
    }
}
```

---

## Coverage Targets

| Code Type | Target | Note |
|-----------|--------|------|
| Service business logic | 90%+ | Core domain logic, error handling |
| Repository interfaces | 85%+ | Mocked in unit tests; integration tests cover DB |
| Handlers | 80%+ | Use httptest; focus on happy path + errors |
| DAO/DTOs | 0% | Acceptable (simple data containers) |
| Config/Init | 0% | Acceptable (tested via integration tests) |

**How to measure:**

```bash
# Generate coverage report
go test -coverprofile=coverage.out ./...

# View by function
go tool cover -func=coverage.out

# View in browser
go tool cover -html=coverage.out
```

---

## Error Testing

### Test All Error Paths

Every error return must have at least one test:

```go
func TestLogin_UserNotFound(t *testing.T) {
    svc := NewTestAuthService(t,
        &MockUserRepo{Err: repository.ErrUserNotFound},
        &MockTokenStore{},
    )

    _, err := svc.Login(context.Background(), &dto.LoginRequest{
        Username: "nobody",
        Password: "any",
    })

    if !errors.Is(err, services.ErrInvalidCredentials) {
        t.Errorf("expected ErrInvalidCredentials, got %v", err)
    }
}

func TestLogin_TokenStoreSaveError(t *testing.T) {
    svc := NewTestAuthService(t,
        &MockUserRepo{User: &dao.User{...}},
        &MockTokenStore{SaveErr: errors.New("db error")},
    )

    _, err := svc.Login(context.Background(), &dto.LoginRequest{
        Username: "admin",
        Password: "pass",
    })

    if !errors.Is(err, errors.New("db error")) {
        t.Errorf("expected db error, got %v", err)
    }
}
```

**Pattern:**
- Use `errors.Is()` for sentinel error comparison
- Test wrapped errors with `fmt.Errorf("%w", err)`
- Test both expected user-facing errors and internal errors

---

## Test Naming

### Test Function Names

```
TestFunction_Scenario
TestFunction_Scenario_Detail
```

Examples:
- `TestLogin_Success`
- `TestLogin_WrongPassword`
- `TestLogin_InactiveUser`
- `TestRefresh_TokenNotFound`
- `TestRefresh_UserDeletedAfterTokenIssue`

### Subtest Names

```go
t.Run("user not found", func(t *testing.T) { ... })
t.Run("password incorrect", func(t *testing.T) { ... })
t.Run("token expired", func(t *testing.T) { ... })
```

**Principle:** Test name should describe the scenario being tested, not implementation details.

---

## Running Tests

```bash
# Run all tests in a service
go test ./services/auth-svc/...

# Run tests with coverage
go test -cover ./services/auth-svc/tests/unit

# Run tests with verbose output
go test -v ./services/auth-svc/tests/unit

# Run a specific test
go test -run TestLogin_Success ./services/auth-svc/tests/unit

# Run tests in parallel with race detection
go test -race -parallel 4 ./services/auth-svc/tests/unit

# Generate coverage report
go test -coverprofile=coverage.out ./services/auth-svc/tests/unit
go tool cover -html=coverage.out -o coverage.html
```

---

## Common Mistakes to Avoid

❌ **Don't:** Put `*_test.go` files alongside source code
```
// BAD:
services/auth-svc/internal/services/auth_service.go
services/auth-svc/internal/services/auth_service_test.go  ← Wrong location
```

✅ **Do:** Put tests in dedicated test package
```
// GOOD:
services/auth-svc/internal/services/auth_service.go
services/auth-svc/tests/unit/auth_service_test.go        ← Correct location
```

---

❌ **Don't:** Use unexported mocks
```
type mockUserRepo struct { ... }  // ← Can't be used in other test files
```

✅ **Do:** Export mock types
```
type MockUserRepo struct { ... }  // ← Reusable across tests
```

---

❌ **Don't:** Duplicate test helpers
```
// auth_service_test.go
func newTestRSAKey(t *testing.T) { ... }

// apikey_service_test.go
func newTestRSAKey(t *testing.T) { ... }  // Duplicate!
```

✅ **Do:** Centralize in `helpers.go`
```
// helpers.go
func NewTestRSAKey(t *testing.T) { ... }  // Shared by all tests
```

---

❌ **Don't:** Ignore error cases
```
func TestLogin(t *testing.T) {
    // Only testing success path
}
```

✅ **Do:** Test all error paths
```
func TestLogin_Success(t *testing.T) { ... }
func TestLogin_WrongPassword(t *testing.T) { ... }
func TestLogin_UserNotFound(t *testing.T) { ... }
func TestLogin_InactiveUser(t *testing.T) { ... }
```

---

## Checklist for New Tests

When adding tests for a new service/function:

- [ ] Create `tests/unit/` folder with `mocks.go` and `helpers.go`
- [ ] All mocks are exported (`Mock{Name}`) and satisfy interfaces
- [ ] Common helpers extracted to `helpers.go` (RSA keys, password hashing, service creation)
- [ ] Test function naming follows `Test{Function}_{Scenario}` pattern
- [ ] At least one test per error path
- [ ] Call tracking in mocks to verify interactions (for side effects)
- [ ] Use `t.Helper()` in all helper functions
- [ ] Coverage target: 85%+ for services, 90%+ for critical paths
- [ ] All tests passing: `go test ./services/{name}/tests/unit`
- [ ] No `t.Skip()` or `t.SkipNow()` unless temporary with tracking issue

---

## Next Steps

**Apply this standard to:**
1. ✅ `auth-svc` — Already implemented
2. `account-svc` — Migrate existing tests to `tests/unit/`
3. `audit-svc` — Set up structure, write tests from scratch

**Future extensions:**
- Integration tests in `tests/integration/`
- Contract tests for HTTP handlers
- Load testing with k6 in `tests/load/`
