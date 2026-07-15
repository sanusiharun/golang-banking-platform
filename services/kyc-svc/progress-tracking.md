# kyc-svc — Progress Tracking

**Last Updated:** 2026-07-14 (initial plan)  
**Current Phase:** Planning → Implementation (E1–E3 ready to start)

---

## Legend

| Symbol | Meaning |
|---|---|
| ⬜ | Not started |
| 🟡 | In progress |
| ✅ | Complete (tested + merged) |
| 🚫 | Blocked (see Current Blockers table) |
| 🔄 | On hold / pending external dependency |

---

## Epic Dependency Graph

```
E1 (Auth & API Keys)  ──┐
                        ├─→ E4 (HTTP API)     ──→ E5 (Integration)
E2 (Verification Logic)──┤                         ↑
                        ├─→ E3 (Storage)     ─────┘
E6 (Observability) ─────→ All epics (cross-cutting)
E7 (Benchmark) ────────→ E2 (OCR engine selection)
```

**Critical Path:** E1 → E2 → E3 → E4 → E5 (sequential)  
**Parallel:** E6 (observability) can be instrumented alongside E1–E5  
**Gate:** E7 (benchmark) must complete before finalizing E2 OCR engine selection

---

## Epics & Tasks

### E1: Auth & API Key Management

**Goal:** Implement kyc-svc-owned API key generation, validation, and revocation (independent of auth-svc).

**Status:** ⬜ Not started  
**Owner:** —  
**Duration Estimate:** 3–4 days

| Task ID | Task | Status | Acceptance Criteria | Notes |
|---|---|---|---|---|
| **E1-T01** | Design API key format & storage strategy | ⬜ | Key format `bp_kyc_<32 base62>` decided; SHA-256 hash approach approved; in-memory cache (5-min TTL) + DB fallback designed. | Follow auth-svc pattern (see HANDOFF.md); reference: `services/auth-svc/internal/service/apikey_service.go` |
| **E1-T02** | Create `api_keys` table schema & migrations | ⬜ | `migrations/002_create_api_keys.up.sql` written; schema includes (id, service, key_prefix, key_hash, is_active, is_revoked, created_at, revoked_at); index on `is_active`. | Location: `services/kyc-svc/migrations/002_create_api_keys.up.sql` |
| **E1-T03** | Implement APIKeyService interface | ⬜ | `internal/service/apikey_service.go` exports: `Generate(ctx, service) (plaintext, hash, err)`, `Validate(ctx, hash) (identity, err)`, `Revoke(ctx, hash) err`. | Depends on E1-T02 (schema). No external dependencies. |
| **E1-T04** | Implement PostgresAPIKeyRepository | ⬜ | `internal/infra/postgres/apikey_repo.go` implements: `Create(ctx, key)`, `GetByHash(ctx, hash)`, `Update(ctx, key)`, `Delete(ctx, id)`. All queries use prepared statements. | Depends on E1-T02 (schema). Query path: SELECT by key_hash, UPDATE is_revoked. |
| **E1-T05** | Implement API key cache-aside lookup | ⬜ | In-memory cache (`sync.Map` or `lru.Cache`) with 5-min TTL. Cache key: `apikey:{hash}`. On miss, query DB; on hit, return. On revoke, invalidate entry. | `internal/service/apikey_service.go` or separate cache module. Consider race conditions (key revoked while in-flight). |
| **E1-T06** | Implement auth middleware (X-API-Key validation) | ⬜ | `internal/middleware/auth.go` exports `ValidateAPIKey(next http.Handler)`. Middleware extracts header, hashes key, calls `APIKeyService.Validate()`, stores identity in context. Returns 401 on failure. | Uses `pkg/middleware` pattern. Context key: `pkg/middleware.ContextKeyServiceIdentity` or similar. |
| **E1-T07** | Unit tests for APIKeyService & Repository | ⬜ | Unit tests in `tests/unit/apikey_service_test.go` and `tests/unit/apikey_repo_test.go`. Mock database; test: generate, validate, revoke flows. Coverage: ≥ 90%. | TDD: write tests first (see `/eng-testing`). No DB mocking library specified yet (use `sqlc.Querier` interface or manual mock). |
| **E1-T08** | Integration tests for auth middleware | ⬜ | Integration test `tests/integration/auth_middleware_test.go` (uses real Postgres). Test: missing key (401), invalid key (401), valid key (200), revoked key (401), cache hit vs. miss. | Depends on E1-T06. Use `testing.T` with `httptest.Server`. |

**Blockers:** None initially. E1 is independent (self-contained auth).

---

### E2: Core Verification Logic

**Goal:** Implement OCR engine abstraction, image quality scoring, field validation, and verification state machine.

**Status:** ⬜ Not started  
**Owner:** —  
**Duration Estimate:** 5–7 days (depends on E7 benchmark results for OCR selection)

| Task ID | Task | Status | Acceptance Criteria | Notes |
|---|---|---|---|---|
| **E2-T01** | Design OCREngine interface & pluggable architecture | ⬜ | `internal/service/ocr_engine.go` defines `type OCREngine interface { Extract(ctx context.Context, image []byte) (Extraction, error) }`. Two implementations: `TesseractEngine`, `SidecarEngine`. Runtime selection via `OCR_ENGINE` env var. | Decision point: benchmark (E7) must run first to select which engine to prioritize in implementation. |
| **E2-T02** | Implement Tesseract OCR integration via gosseract | ⬜ | `internal/infra/ocr/tesseract.go` wraps `github.com/otiai10/gosseract`. Supports: Extract fields (9 v1 fields), timeout (10s), error handling (non-fatal → state="rejected"). | Requires gosseract cgo (Alpine Linux compatibility check in Dockerfile). Document build requirements. |
| **E2-T03** | Implement Python sidecar OCR client (HTTP) | ⬜ | `internal/infra/ocr/sidecar.go` makes HTTP POST to Python sidecar (default `http://ocr-sidecar:8000/extract`). Supports: timeout (15s), retry (3x backoff), fallback to Tesseract on failure. | Configuration: `OCR_SIDECAR_URL` env var. HTTP client timeout: 15s. Fallback logic in `VerificationService`, not here. |
| **E2-T04** | Implement image quality scoring | ⬜ | `internal/service/scoring/image_quality.go` exports `ComputeImageQuality(image []byte) (score float64, details string)`. Analyzes: blur, glare, crop, resolution. Score 0–100. Non-blocking (< 500ms). | Pure Go (no external dependencies for v1; OpenCV optional later). Thresholds: < 60 = low, > 80 = high. Test with sample images. |
| **E2-T05** | Implement field validity scoring | ⬜ | `internal/service/scoring/field_validity.go` exports `ValidateFields(extraction map[string]string) (score float64, reasons []string)`. Checks: NIK format (16 digits + checksum), DOB parses + plausible, required fields non-empty, province/regency valid. Score 0–100. | Reference: CONTEXT.md field list (9 v1 fields). NIK checksum: implement standard Indonesian NIK validation. Reasons: ["nik_invalid", "dob_unparseable", ...] for debugging. |
| **E2-T06** | Implement verification state machine | ⬜ | `internal/service/verification_service.go` exports `Verify(ctx, request) (VerificationResult, error)`. State transitions: processing → (verified | needs_review | rejected). Combines three scores into state via thresholds (image_quality >= 60, ocr_confidence >= 70 per field, field_validity >= 80). | Thresholds are configurable via env vars (`IMAGE_QUALITY_THRESHOLD`, etc.). Service returns all scores; caller decides pass/fail. |
| **E2-T07** | Unit tests for verification logic | ⬜ | Tests in `tests/unit/verification_service_test.go`. Table-driven tests for scoring functions. Mock OCR engine (return predetermined extractions). Test state transitions: all scores high (verified), one low (needs_review), OCR fails (rejected). Coverage: ≥ 90%. | Use table-driven tests (see `/eng-testing`). Mock OCREngine interface. Test fixtures: sample KTP extractions. |
| **E2-T08** | Benchmark harness for OCR engine selection (offline, non-production) | 🔄 | `benchmark/` directory with: (1) labeled KTP dataset loader, (2) Tesseract + Python engine runners, (3) accuracy metrics (F1, precision, recall per field). Output: comparison report. | This is E7, gate on E2. Harness can run independently but results inform E2 engine selection. See E7 below. |

**Blockers:**
- E7 (benchmark harness + dataset) must complete before finalizing OCR engine selection in E2-T06.

---

### E3: Storage & Retrieval

**Goal:** Implement Postgres schema (verifications, audit_log tables), MinIO integration for image archival, and repository layer.

**Status:** ⬜ Not started  
**Owner:** —  
**Duration Estimate:** 4–5 days

| Task ID | Task | Status | Acceptance Criteria | Notes |
|---|---|---|---|---|
| **E3-T01** | Create database schema migrations | ⬜ | `migrations/001_create_verifications.up.sql` (verifications table, indices), `migrations/003_create_audit_log.up.sql` (audit_log table). Schema per architecture.md. Run migrations on `make services-up`. | Location: `services/kyc-svc/migrations/`. Use `golang-migrate/migrate` (already a dep). Include rollback `.down.sql` files. |
| **E3-T02** | Implement VerificationRepository interface | ⬜ | `internal/repository/verification_repo.go` defines interface: `Create(ctx, verification)`, `GetByID(ctx, id)`, `GetByAccountID(ctx, accountID)`, `Update(ctx, verification)`, `GetExpiredVerifications(ctx)` (for retention). | Pure interface; implementations in `infra/postgres/` and (future) `infra/mongodb/`. |
| **E3-T03** | Implement PostgresVerificationRepository | ⬜ | `internal/infra/postgres/verification_repo.go` implements CRUD operations using `pgx`. Handles encryption/decryption of `nik_encrypted` field (delegates to `pkg/crypto`). Uses prepared statements, connection pooling. | Depends on E3-T01 (schema). Query path: all queries prepared + cached. Test with real Postgres (integration). |
| **E3-T04** | Implement AuditLogRepository | ⬜ | `internal/infra/postgres/audit_log_repo.go` exports `Append(ctx, event)` (INSERT only, append-only log). Indexes on (account_id, timestamp) and (action, timestamp). | Location: `internal/repository/audit_log_repo.go` interface + `infra/postgres/audit_log_repo.go` implementation. Never UPDATE or DELETE audit_log. |
| **E3-T05** | Implement MinIO image archival | ⬜ | `internal/infra/minio/image_store.go` exports: `Upload(ctx, verificationID, imageBytes)`, `Delete(ctx, objectKey)`. Handles: bucket creation (or assume exists), error handling, timeout (10s for delete). | Configuration: `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_BUCKET`. Use official AWS S3 SDK (`aws-sdk-go-v2` or `minio-go`). Non-blocking for uploads (fire-and-forget async). |
| **E3-T06** | Implement encrypted NIK storage | ⬜ | In `PostgresVerificationRepository.Create()`, encrypt NIK before INSERT using `pkg/crypto.Encrypt()`. In `GetByID()`, decrypt using `pkg/crypto.Decrypt()`. Response DTO: mark NIK field as redacted/reference-only. | Uses `pkg/crypto` (RSA/AES). Never log plaintext NIK. Test: encrypt → store → retrieve → decrypt → verify matches original. |
| **E3-T07** | Unit & integration tests for repositories | ⬜ | Tests in `tests/unit/verification_repo_test.go`, `tests/integration/verification_repo_test.go`. Mock Postgres for unit tests; use real Postgres for integration. Test: CRUD, encryption/decryption, index lookups. Coverage: ≥ 85%. | Mocking strategy: define `repository.VerificationRepository` interface; mock implementation for unit tests. Real DB for integration. |
| **E3-T08** | Integration test for MinIO archival | ⬜ | Test in `tests/integration/minio_test.go`. Use MinIO local container (or moto mock). Test: upload, retrieve, delete, error handling (bucket not found, permission denied). | Depends on docker-compose having MinIO container running (platform/docker-compose.yml). May use MinIO test container for unit/integration tests. |

**Blockers:** None initially. E3 is independent; can proceed in parallel with E1–E2.

---

### E4: HTTP API

**Goal:** Implement REST endpoint (POST /v1/verify), error responses, request/response marshaling, middleware integration.

**Status:** ⬜ Not started  
**Owner:** —  
**Duration Estimate:** 3–4 days

| Task ID | Task | Status | Acceptance Criteria | Notes |
|---|---|---|---|---|
| **E4-T01** | Define request/response DTOs | ⬜ | `internal/transport/dto.go` defines: `VerifyRequest` (image base64, type, account_id), `VerificationResult` (id, state, three scores, extraction). Extraction excludes raw NIK; marks nik_encrypted as reference-only. | DTO tags: `json:"field_name"`. Validation: non-empty image, valid verification_type enum, account_id UUID format (or opaque string). |
| **E4-T02** | Implement POST /v1/verify handler | ⬜ | `internal/transport/handlers.go` exports `HandleVerify(w http.ResponseWriter, r *http.Request)`. Flow: decode request, call service, encode response. Returns 200 on success, structured error responses on failure (400/401/422/429/500). | Handler is thin; delegates to service layer. Uses `httpx.WriteSuccess()` for 200, `httpx.WriteError()` for errors (per CLAUDE.md convention). |
| **E4-T03** | Implement error response mapping | ⬜ | Map domain errors to HTTP status codes: NotFound → 404, Unauthorized → 401, Validation → 422, RateLimited → 429, Internal → 500. Use `httpx.WriteError()` for all responses. | See `pkg/errors` and `pkg/httpx`. Ensure all service errors are domain types (from `pkg/errors`), not raw Go errors. |
| **E4-T04** | Implement rate limiting middleware | ⬜ | `internal/middleware/rate_limit.go` enforces 100 req/min per API key (in-memory counter). Returns 429 with `Retry-After` header on exceed. Counter reset on startup (acceptable for v1). | Per-key tracking: `map[keyHash]RateLimitState`. Cleanup: TTL-based or periodic reset. Alternative: use `golang.org/x/time/rate.Limiter` per key. |
| **E4-T05** | Implement structured request logging | ⬜ | `internal/middleware/logging.go` logs request summary (verification_id, account_id, verification_type) and response (state, scores). Never log raw extraction or image. Uses `slog` only. | Middleware wraps handler, logs before + after. Use `slog.InfoContext` for happy path, `slog.ErrorContext` for errors. |
| **E4-T06** | Implement tracing (Jaeger spans) | ⬜ | `internal/middleware/tracing.go` creates span per request, injects attributes (verification_id, account_id, state at end). Uses OpenTelemetry SDK. | Depends on platform's `pkg/observability` (assumed already set up in auth-svc, account-svc). |
| **E4-T07** | Unit tests for handler | ⬜ | Tests in `tests/unit/handlers_test.go`. Use `httptest.Server` + mock service. Test: valid request (200), missing key (401), invalid image (400), rate limit (429). Coverage: ≥ 80%. | Mock VerificationService; verify correct handler behavior without full stack. |
| **E4-T08** | Integration test for full request/response flow | ⬜ | Test in `tests/integration/api_test.go`. Full stack: real Postgres, real service, handler. POST /v1/verify with real KTP image, verify response structure. | Uses `httptest.Server` with real chi router + middleware stack. Slow, but validates integration. |

**Blockers:** Depends on E1 (auth middleware), E2 (service layer), E3 (repository layer).

---

### E5: Integration & Lifecycle

**Goal:** Implement account closure webhook, retention tail countdown, automated image cleanup.

**Status:** ⬜ Not started  
**Owner:** —  
**Duration Estimate:** 3–4 days

| Task ID | Task | Status | Acceptance Criteria | Notes |
|---|---|---|---|---|
| **E5-T01** | Implement RetentionService | ⬜ | `internal/service/retention_service.go` exports: `StartRetentionTail(ctx, accountID)` (idempotent), `DeleteExpiredImages(ctx)` (background job). | Idempotent: calling StartRetentionTail twice for same account_id is safe (UPDATE with SET retention_started_at=NOW() where NULL only, or check if already set). |
| **E5-T02** | Implement POST /internal/account-closed webhook handler | ⬜ | `internal/transport/handlers.go` exports `HandleAccountClosed(w http.ResponseWriter, r *http.Request)`. Receives { account_id, closed_at }, calls `RetentionService.StartRetentionTail()`. Returns 204. Idempotent. | Handler: decode JSON, validate account_id, call service (async is optional for v1), return 204. No error response (failure is logged, not returned). |
| **E5-T03** | Secure /internal/account-closed endpoint | ⬜ | Add authentication: static shared secret header (`X-Internal-Token`) or IP allowlist (Docker network). **TODO (pre-production):** implement HMAC-SHA256 signature. | For v1: just IP allowlist (Docker internal network only). Document in Pending section of HANDOFF.md. |
| **E5-T04** | Implement background image deletion job | ⬜ | `internal/service/retention_service.go` defines `DeleteExpiredImages()`. Query Postgres for verifications where `retention_started_at + 5y < NOW()`. For each, delete from MinIO. Update `image_deleted_at` in DB. Log results. | Runs every 1 hour (configurable via `RETENTION_JOB_INTERVAL` env var). Non-blocking; can fail gracefully (retry next hour). |
| **E5-T05** | Wire background job to container lifecycle | ⬜ | `cmd/main.go` or `internal/container.go` starts background goroutine for deletion job. Use `time.Ticker` for periodic execution. Graceful shutdown: context cancellation. | Job should start after Postgres is ready. Stop job on `SIGTERM` (context cancellation). |
| **E5-T06** | Integration test for account closure flow | ⬜ | Test in `tests/integration/retention_test.go`. Simulate: create verification, close account (POST /internal/account-closed), verify retention_started_at set, wait, trigger background job, verify image deleted. | Use short retention tail in test (1 second instead of 5 years) to validate flow. Mock time.Now() if possible, or use real delays. |
| **E5-T07** | Unit test for RetentionService | ⬜ | Tests in `tests/unit/retention_service_test.go`. Mock repository. Test: idempotency of StartRetentionTail, expired query logic, deletion flow. | Mock time.Now() for expiry checks (or use dependency injection for time provider). |

**Blockers:** Depends on E3 (repository layer, database).

---

### E6: Observability (Metrics, Logging, Tracing)

**Goal:** Instrument all layers with Prometheus metrics, structured logging, and Jaeger tracing.

**Status:** ⬜ Not started  
**Owner:** —  
**Duration Estimate:** 4–5 days (cross-cutting; can run in parallel with E1–E5)

| Task ID | Task | Status | Acceptance Criteria | Notes |
|---|---|---|---|---|
| **E6-T01** | Implement Prometheus metrics | ⬜ | Register metrics in `internal/observability/metrics.go`: verification latency histogram, OCR confidence histogram, image quality histogram, field validity histogram, request rate counter, error rate counter, API key validation latency, MinIO latency, OCR engine latency, retention job metrics. | Use `prometheus/client_golang`. Register metrics in `init()` or `NewMetrics()` constructor. Defer recording to service/middleware. |
| **E6-T02** | Instrument service layer with latency metrics | ⬜ | Wrap `VerificationService.Verify()` with timing code. Record histogram buckets: [0.1, 0.5, 1, 2, 3, 5, 10] seconds. Labels: `type`, `state`, `engine`. | Use middleware or decorator pattern; don't clutter service logic. Consider `prometheus.Timer` wrapper. |
| **E6-T03** | Instrument OCR engine calls | ⬜ | Time Tesseract and sidecar HTTP calls. Record `kyc_ocr_engine_call_latency_seconds` with labels `engine`, `success`. | In `tesseract.go` and `sidecar.go`, wrap Extract() calls. Record both success and failure cases. |
| **E6-T04** | Instrument MinIO operations | ⬜ | Time Upload, Delete operations. Record `kyc_minio_upload_latency_seconds`, `kyc_minio_delete_latency_seconds`. Labels: `success`, `operation`. | In `minio/image_store.go`, wrap all I/O calls. Record errors (bucket not found, permission denied) as metric labels. |
| **E6-T05** | Instrument API key validation | ⬜ | Time cache lookups vs. DB lookups. Record `kyc_apikey_validation_latency_seconds` with label `cache_hit`. | In `apikey_service.go` or middleware, time cache.Get() and DB query separately. |
| **E6-T06** | Configure structured logging (slog) | ⬜ | All `fmt.Println` removed. All output via `slog.InfoContext`, `slog.WarnContext`, `slog.ErrorContext`. JSON format. | Middleware/service/repository: log at appropriate levels. No sensitive data (NIK, extraction, image). Test: run service, check logs for JSON format and no secrets. |
| **E6-T07** | Configure Jaeger tracing | ⬜ | All services and repositories create spans. `pkg/observability` already provides `trace.SpanFromContext()` and `trace.NewSpan()`. Decorate request handlers, service methods, DB calls. | Depends on platform's observability setup (assumed ready). Reuse patterns from auth-svc/account-svc. |
| **E6-T08** | Configure health checks (/healthz/live, /healthz/ready) | ⬜ | `internal/transport/handlers.go` exports `HandleHealthzLive()` and `HandleHealthzReady()`. Live: always 200. Ready: check Postgres, MinIO, OCR engine. Return 503 if any dependency unhealthy. | Depends on E1–E5 to be ready (dependencies must exist). Health check logic: query test on each dependency, timeout 500ms–1s. |
| **E6-T09** | Redaction & security logging | ⬜ | Code review: no logs contain extraction JSON, NIK, base64 image, or full account details. Only log: verification_id, account_id (opaque string), state, numeric scores. | Static analysis or grep: search for "extraction", "nik", "image" in logging statements. Document redaction rules in comment block. |

**Blockers:** None; can proceed in parallel with E1–E5. E6-T08 (health checks) depends on E1–E5 readiness.

---

### E7: Offline Benchmark (OCR Engine Selection)

**Goal:** Build benchmark harness to compare OCR engines (Tesseract, PaddleOCR, docTR) and select production engine.

**Status:** 🔄 Pending (on hold until dataset acquired)  
**Owner:** —  
**Duration Estimate:** 5–7 days (includes dataset curation)

| Task ID | Task | Status | Acceptance Criteria | Notes |
|---|---|---|---|---|
| **E7-T01** | Acquire labeled KTP dataset (100+ images with ground truth) | 🔄 | Dataset obtained: JPEG/PNG images of real Indonesian KTPs, ground truth extractions (NIK, name, DOB, etc.) for each. Dataset format: `dataset/{image_id}.jpg` + `dataset/{image_id}.json` (ground truth). | **Gate item:** must be done before E7-T02. Source: internal photos, synthetic data, or third-party dataset. Security: ensure no real sensitive data in version control (store as private artifact). |
| **E7-T02** | Build benchmark dataset loader | ⬜ | `benchmark/dataset/loader.go` exports `LoadDataset(path string) []TestCase`. TestCase = { ImageID, ImageBytes, GroundTruth map[field]value }. Validation: all images loadable, ground truth parseable. | Location: `benchmark/` (outside `internal/`). Can be standalone CLI or library. Produces list of test cases. |
| **E7-T03** | Implement Tesseract benchmark runner | ⬜ | `benchmark/runners/tesseract_runner.go` exports `RunTesseract(dataset []TestCase) Results`. For each test case: call Tesseract Extract(), compare output vs. ground truth, compute accuracy (F1, precision, recall per field, overall). Measure latency. | Reuse `internal/infra/ocr/tesseract.go` (or import from benchmark). Run on benchmark dataset. Output: per-field and overall accuracy, latency distribution. |
| **E7-T04** | Implement PaddleOCR sidecar benchmark runner | ⬜ | `benchmark/runners/paddleocr_runner.go` exports `RunPaddleOCR(dataset []TestCase) Results`. Starts Python sidecar (Flask/FastAPI running PaddleOCR), calls it for each test case, compares vs. ground truth. | Requires sidecar image to exist. Runner should handle sidecar startup/shutdown (or assume running). Output: same metrics as Tesseract. |
| **E7-T05** | Implement docTR sidecar benchmark runner | ⬜ | `benchmark/runners/doctr_runner.go` exports `RunDocTR(dataset []TestCase) Results`. Same as PaddleOCR (run sidecar, compare, measure). | Optional for v1; can be added later if needed. For now: implement if resources available. |
| **E7-T06** | Build benchmark CLI & reporting | ⬜ | `benchmark/main.go` runs all enabled engines, compares results, produces CSV/JSON report. Report includes: per-field accuracy (Tesseract vs. PaddleOCR vs. docTR), latency distribution, confidence distribution. Recommendation: select engine with highest F1 score. | CLI flags: `--dataset-path`, `--engines tesseract,paddleocr,doctr`, `--output report.csv`. Report format: human-readable + machine-parseable. |
| **E7-T07** | Run benchmark, document results, update architecture.md | ⬜ | Benchmark executed on labeled dataset. Results documented: which engine won, why, trade-offs (accuracy vs. latency). Decision recorded in `HANDOFF.md` (kyc-svc section). Update architecture.md with selected engine. | Output: benchmark report (checked in if not sensitive, or as artifact). Decision: update `OCR_ENGINE` default in `.env.example`. |
| **E7-T08** | Integrate selected engine into production build | ⬜ | If Tesseract wins: default behavior (in E2-T02). If PaddleOCR wins: ensure sidecar deployment (docker-compose.yml updated to include Python service). If both: make configurable via env var. | Dockerfile: add Python base image for sidecar (if needed). docker-compose.yml: add ocr-sidecar service (if needed). .env: set OCR_ENGINE default. |

**Blockers:** Depends on dataset acquisition (A-04: benchmark dataset availability). E7 results gate E2-T06 (final OCR engine selection in production).

---

### E8: Testing & Coverage

**Goal:** Comprehensive unit and integration test suite per `/eng-testing` standards.

**Status:** ⬜ Not started  
**Owner:** —  
**Duration Estimate:** 4–5 days (spreads across E1–E7, but bundled here for tracking)

| Task ID | Task | Status | Acceptance Criteria | Notes |
|---|---|---|---|---|
| **E8-T01** | Set up test structure (`tests/unit`, `tests/integration`) | ⬜ | Directory structure per `/eng-testing`. `tests/unit/mocks.go` (all mock implementations), `tests/unit/helpers.go` (reusable test utils). | Location: `services/kyc-svc/tests/{unit,integration}/`. Build tags: `// +build unit` (or use filename suffix `_test.go`). |
| **E8-T02** | Write mocks for all interfaces | ⬜ | `tests/unit/mocks.go` includes: MockVerificationRepository, MockAPIKeyRepository, MockAuditLogRepository, MockOCREngine, MockMinIOStore. Each mock tracks calls (for assertions). | Use simple manual mocks (no mockgen initially; add if coverage becomes unwieldy). Mocks must be fast (no I/O). |
| **E8-T03** | Unit test coverage for service layer (target ≥ 90%) | ⬜ | All service logic tested: VerificationService, RetentionService, APIKeyService. Table-driven tests for scoring functions. Test both happy paths and error cases. | Run `go test -cover ./...` regularly. Aim for 90%+ on service, 85%+ on repo (mocked DB), 80%+ on handlers. |
| **E8-T04** | Unit test coverage for repository layer (target ≥ 85%, mocked) | ⬜ | Test CRUD operations with mocked DB (mock `pgx.Rows`, etc.). Test query logic without hitting real DB. | Mocking strategy: use `sqlc.Querier` interface (if using sqlc) or manual prepared statement mocks. Avoid test database for unit tests (slow, flaky). |
| **E8-T05** | Unit test coverage for handlers (target ≥ 80%) | ⬜ | Test HTTP request/response marshaling, error handling, middleware integration. Use `httptest.Server` with mock service. | Don't use real dependencies (Postgres, MinIO, OCR engine) in unit tests. Mock all. Fast execution (< 1s for all handler unit tests). |
| **E8-T06** | Integration tests for DB operations (real Postgres) | ⬜ | `tests/integration/db_test.go` uses real Postgres (test database). Test repository CRUD, transactions, indices, encryption/decryption. Setup/teardown: create tables before test, drop after. | Use testcontainers-go for containerized Postgres, or assume platform provides test DB. Slower (acceptable, run on CI only). |
| **E8-T07** | Integration tests for full request flow | ⬜ | `tests/integration/api_test.go` tests full stack: real Postgres, real chi router, all middleware. Send HTTP request, verify response. | httptest.Server with real handler chain. Slower but validates integration. Keep count low (< 10 integration tests; rest are unit tests). |
| **E8-T08** | CI/CD: add test step to Makefile | ⬜ | `make test` runs all unit tests. `make test-integration` runs integration tests (assumes local Postgres running). CI pipeline runs both on every commit. | Update `.github/workflows/test.yml` (or equivalent). Report coverage. Fail on coverage drop. |

**Blockers:** Depends on all other epics for test targets.

---

## Current Blockers

| Item | Impact | Affects Epic | Mitigation | Owner | ETA |
|---|---|---|---|---|---|
| **Benchmark dataset not yet acquired** | Cannot finalize OCR engine selection (E7 blocked) | E7 → blocks E2-T06 (final engine selection) | Platform to acquire 100+ labeled KTP images with ground truth. Use internal photos or public dataset. | Platform team | TBD |
| **Webhook authentication not yet designed** | /internal/account-closed endpoint insecure (E5-T03 blocked) | E5-T03 | Design shared secret or HMAC-SHA256 signature. Implement before E5-T03 completion. Document in Pending. | Engineering team | Before E5 merge |
| **MinIO deployment not confirmed** | E3-T05 (image archival) depends on MinIO availability | E3-T05, E5-T04 | Platform confirms MinIO is ready on banking-net. Confirm bucket (`kyc-verifications`) exists or auto-create. | Platform/Ops | Before E3 starts |

---

## Technical Debt Register

| ID | Title | Severity | Description | Linked Task | Notes |
|---|---|---|---|---|---|
| **TD-01** | API key cache invalidation on revoke not atomic | Medium | Current design: revoke → DB delete + cache invalidate. Race condition: key validated from cache between DB delete and cache invalidate. In-flight request may succeed with revoked key. | E1-T05 | **Mitigation for v1:** cache TTL (5 min) means max window is 5 min. Add version/epoch field to cached entry; invalidate on epoch change. **Future:** implement distributed cache invalidation (Redis pub/sub). |
| **TD-02** | Image upload to MinIO is async, no retry logic | Medium | Currently: fire-and-forget. If upload fails, image lost. Verification succeeds, but image absent. | E3-T05, E6-T04 | **Mitigation:** track failed uploads in DB, async job retries. **Future:** exponential backoff + circuit breaker. |
| **TD-03** | No webhook signature verification (auth hardcoded) | High | /internal/account-closed only secured by Docker network IP allowlist. **Must be fixed before production.** | E5-T03 | **Pre-production requirement:** implement HMAC-SHA256 signature or OAuth client credentials. **Interim:** document in Pending. |
| **TD-04** | OCR engine fallback logic not configurable | Low | Hard-coded: if sidecar fails, fall back to Tesseract. What if both should fail gracefully? | E2-T01, E2-T03 | **Future:** add fallback config (`OCR_FALLBACK_STRATEGY`: fail-closed, fall-back-to-tesseract, return-partial-score). |
| **TD-05** | No distributed rate limiting | Medium | Rate limit counter in-memory. Multi-replica deployments share no state; each replica has independent 100/min limit. De facto 100 × replicas across fleet. | E4-T04 | **Mitigation for v1:** single replica or accept higher total limit. **Future:** Redis-backed rate limiter. |
| **TD-06** | Benchmark harness is offline only (no continuous monitoring) | Low | OCR engine accuracy not monitored post-deployment. If real-world KTPs differ from benchmark dataset, no alert. | E7 | **Future:** add accuracy metrics to prod (comparison against manually-verified sample), trigger alerts if accuracy drops. |
| **TD-07** | No transaction-level encryption for batch operations | Low | If batch `DeleteExpiredImages()` partially fails, some images deleted, others not. Inconsistent state. | E5-T04 | **Mitigation:** log all deletions to audit_log first, then execute. If failure, retry from audit log. **Future:** distributed transaction (Saga pattern). |
| **TD-08** | Encrypted NIK not decryptable if `pkg/crypto` key rotates | Medium | Single encryption key in `pkg/crypto`. If key rotated, old NIKs unrecoverable. | E3-T06 | **Mitigation for v1:** no key rotation (freeze key). **Future:** implement versioned encryption (store key version with ciphertext), support multiple keys during transition. |

---

## Summary

### Progress at a Glance

| Phase | Epics | Status | Start | End |
|---|---|---|---|---|
| **Planning** | E1–E8 | ✅ (goals.md + architecture.md + this tracking doc) | — | 2026-07-14 |
| **Implementation Phase 1** | E1, E3 | ⬜ | 2026-07-15 | 2026-07-22 (est.) |
| **Implementation Phase 2** | E2, E6 | ⬜ | 2026-07-22 | 2026-07-29 (est.) |
| **Implementation Phase 3** | E4, E5 | ⬜ | 2026-07-29 | 2026-08-05 (est.) |
| **Review & Testing** | E8, E7 (if needed) | ⬜ | 2026-08-05 | 2026-08-12 (est.) |
| **Production Ready** | All ✅ | ⬜ | — | 2026-08-15 (est.) |

### Critical Path (Must-Do Order)

```
E1 (Auth) → E2 (Verification) → E3 (Storage) → E4 (API) → E5 (Lifecycle)
  ↓             ↓                    ↓           ↓           ↓
 3–4d         5–7d*                4–5d        3–4d        3–4d
              (* E7 benchmark)
```

**Legend:** E6 and E8 are parallel (cross-cutting). E7 is optional-ish (can use Tesseract default if benchmark incomplete).

### Next Steps

1. **Assign owners** to each epic (E1–E8).
2. **Kick off E1 & E3** (independent, can run in parallel).
3. **Run E7 benchmark** (concurrent with implementation; results inform E2-T06).
4. **Update this document** after each work session (mark ✅, record blockers, adjust ETA).
5. **Write review.md** after all epics complete (assessment against goals.md acceptance criteria).

---

## Document Maintenance

- **Updated:** Each work session, mark tasks in-progress or complete.
- **Blockers:** Add new blockers as they arise; resolve and remove.
- **TD items:** Add debt as discovered (don't hide it; ownership is healthy).
- **ETA:** Adjust based on actual progress and blocking dependencies.
