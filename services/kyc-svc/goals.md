# kyc-svc — Goals

## Service Identity

| Attribute | Value |
|---|---|
| **Name** | kyc-svc |
| **Port** | 8084 |
| **Database** | `banking_kyc` (Postgres) |
| **Owner Domain** | Customer Identity Verification / Compliance |
| **Criticality Tier** | High — blocks account onboarding; mandatory for regulatory compliance |
| **Deployed On** | Docker, `banking-net` bridge network |

---

## Business Objectives

| ID | Objective |
|---|---|
| **BO-01** | Enable customer identity verification as a foundational capability for account onboarding, reducing fraud risk and meeting Indonesian banking compliance requirements. |
| **BO-02** | Provide a pluggable verification framework that starts with KTP OCR but allows additional verification types (facial recognition, address verification, etc.) without architectural changes. |
| **BO-03** | Maintain a complete audit trail of all verification requests and results for compliance, dispute resolution, and operational debugging. |
| **BO-04** | Preserve the option to extract `kyc-svc` into a standalone product for external customers, independent of this platform's account or authentication infrastructure. |
| **BO-05** | Ensure PII is handled with encryption, retention tied to account lifecycle, and automated cleanup to minimize compliance risk. |

---

## Functional Requirements

### Verification & Scoring

| ID | Requirement | Details |
|---|---|---|
| **FR-01** | Accept KTP image upload and verification request | Service receives a JPEG/PNG image (base64 or multipart) and verification type (`ktp_ocr`). Validates image is readable before processing. |
| **FR-02** | Extract structured data from KTP OCR | OCR engine extracts: NIK, Nama, Tempat/Tgl Lahir, Jenis Kelamin, Alamat (RT/RW, Kel/Desa, Kecamatan), Agama, Status Perkawinan, Kewarganegaraan, Berlaku Hingga. All fields returned in extraction JSON (whether all pass validity checks or not). |
| **FR-03** | Compute image quality score before OCR | Analyzes input image for blur, glare, crop, resolution, legibility. Score 0–100. Influences verification outcome. |
| **FR-04** | Report OCR confidence per field | OCR engine provides per-field confidence (0–100). Returned unmodified; reflects engine certainty, not ground-truth accuracy. |
| **FR-05** | Compute field validity score | Apply business rules independent of OCR engine: NIK 16-digit format + valid province/regency code, birth date parses and is plausible, required fields non-empty. Score 0–100. Identifies structurally invalid extractions. |
| **FR-06** | Return verification state | Emit state: `processing` (transient), `verified` (all scores above threshold, all fields valid), `needs_review` (at least one score below threshold), `rejected` (no usable data extracted). |
| **FR-07** | Return all three scores in response | Always return `{ ocr_confidence, image_quality_score, field_validity_score }` so caller can apply custom thresholds. Never collapse to a single boolean. |
| **FR-08** | Persist verification request and result | Store in `banking_kyc` database: request metadata, verification state, all three scores, extracted fields (plaintext), timestamp. Retrievable by verification ID or account ID. |

### PII Handling & Retention

| ID | Requirement | Details |
|---|---|---|
| **FR-09** | Store extracted NIK in secure column | NIK is sensitive PII. Store in database as plaintext or ciphertext (encryption handled by platform via Postgres column-level encryption or similar). kyc-svc does not encrypt/decrypt; platform infrastructure ensures at-rest encryption. Never return raw NIK in API response. |
| **FR-10** | Archive raw KTP image in MinIO | Store base64-decoded image in MinIO (S3-compatible). Do not store as Postgres BYTEA. Store reference (S3 path) in verification record. MinIO enforces server-side encryption (ops responsibility). |
| **FR-11** | Tie image retention to account lifecycle | Retain raw image as long as account is open. On account closure signal from `account-svc`, start 5-year statutory tail. Delete image after tail expires. NIK record persists in database (tied to verification record lifecycle). |
| **FR-12** | Implement automated image cleanup | Background job queries verifications with closed accounts + expired tails, deletes from MinIO, logs deletion. No manual intervention. |
| **FR-13** | Never log sensitive data | Slog output must not include raw image, extracted fields, NIK, or field extractions. Log only: verification ID, account ID, verification type, outcome state, scores (numeric, not sensitive). |

### API & Authentication

| ID | Requirement | Details |
|---|---|---|
| **FR-14** | Issue and validate service API keys | Generate unique API keys for authorized callers (e.g., `account-svc`). Validate key on every request. Use X-API-Key header. No JWT; no auth-svc dependency. |
| **FR-15** | Accept POST /v1/verify requests | Endpoint: `POST /v1/verify`. Body: `{ image: "base64...", type: "ktp_ocr", account_id: "uuid" }`. Response: `{ id, state, ocr_confidence, image_quality_score, field_validity_score, extraction }` (extraction excludes raw NIK; extraction.nik is encrypted reference only). |
| **FR-16** | Return structured error responses | 400 Bad Request (invalid image format), 401 Unauthorized (missing/invalid API key), 422 Unprocessable Entity (image too small, unreadable, etc.), 429 Too Many Requests (rate limit), 500 Internal Server Error. All errors include `code` and `message` fields. |
| **FR-17** | Accept account-closure notifications | Consume signal from `account-svc` (webhook or NATS subscription) when account is closed. Triggers retention tail countdown for all verifications linked to that account. |

### Benchmark & Engine Selection

| ID | Requirement | Details |
|---|---|---|
| **FR-18** | Support offline OCR benchmark harness | Non-production utility to compare multiple OCR engines (language/platform agnostic: Go, Python, Java, etc.) against a labeled KTP dataset. Measure accuracy per engine; select single winning engine for production. Accuracy is priority; ignore engineering convenience. |
| **FR-19** | Deploy selected OCR engine for production | After benchmark, deploy winning engine (whether Tesseract, PaddleOCR, docTR, or other). If engine requires sidecar/external process (e.g., Python), include in docker-compose and deployment. kyc-svc calls engine via stable interface (in-process or HTTP). |

---

## Non-Functional Requirements

### Performance

| ID | Requirement | Target |
|---|---|---|
| **NFR-01** | Verification latency (P50) | < 1s (from request arrival to response sent). Covers image quality score + OCR + field validation. Excludes MinIO write latency (async). |
| **NFR-02** | Verification latency (P99) | < 3s. Accounts for OCR engine variability and image preprocessing. |
| **NFR-03** | API key validation latency | < 10ms. Cached in memory; no DB lookup per request. |
| **NFR-04** | Throughput | Handle 100 concurrent verification requests without latency degradation. Measured on 2-CPU Docker container. |

### Availability & Reliability

| ID | Requirement | Target |
|---|---|---|
| **NFR-05** | Service availability | 99.9% uptime. Measured over 30-day rolling window. Excludes planned maintenance. |
| **NFR-06** | Database failover | Postgres replication + automatic failover (handled by platform RDS/Patroni setup). No service-level retry logic needed. |
| **NFR-07** | OCR engine fallback | If sidecar engine unavailable, gracefully degrade to built-in Tesseract (if benchmark selects sidecar as primary). Return result with degraded state noted in observability, never fail request. |
| **NFR-08** | Image storage resilience | MinIO backed by persistent storage (managed platform infra). Verify bucket exists and is writable before serving requests. Health check includes MinIO connectivity. |

### Security & Compliance

| ID | Requirement | Details |
|---|---|---|
| **NFR-09** | Encryption at rest | NIK stored in secure database column (platform-managed encryption via Postgres column-level or similar, not app-level). Raw KTP image stored in MinIO (S3-compatible); server-side encryption enabled on MinIO bucket. Database backups inherit encryption from Postgres. |
| **NFR-10** | Authentication & authorization | Only callers with valid X-API-Key can invoke /v1/verify. API key validation is synchronous, no token introspection delay. No bearer token or JWT. |
| **NFR-11** | Audit logging | Every verification request and result logged to `banking_kyc.audit_log` table: requester (API key issuer), timestamp, request summary, result summary, scores. Retained for 7 years (Indonesian banking audit requirement). |
| **NFR-12** | PII data minimization | Extraction JSON stored in database but NIK encrypted and image stored remotely. Response API never includes raw NIK or full image reference. Logged output excludes sensitive fields. |
| **NFR-13** | Rate limiting | API key holder limited to 100 verifications/minute. Limit is per-key, enforced in memory (no external rate limiter). Exceed returns 429 with Retry-After header. |
| **NFR-14** | CORS & API security | CORS headers allow account-svc and platform admin tools only. API key scoped to operation (read/write verifications, manage keys). |

### Observability

| ID | Requirement | Details |
|---|---|---|
| **NFR-15** | Structured logging | All app output via `slog` only, never `fmt.Println`. Log level: INFO (happy path summary), WARN (score below threshold, degraded state), ERROR (OCR failure, DB error, missing image in MinIO). JSON format for machine parsing. |
| **NFR-16** | Metrics instrumentation | Prometheus metrics: (1) verification latency histogram (per verification type), (2) OCR confidence distribution (histogram), (3) image quality score distribution, (4) field validity score distribution, (5) request rate (per type), (6) error rate (per type), (7) MinIO write latency, (8) API key validation latency. |
| **NFR-17** | Distributed tracing | Jaeger spans for: (1) full request lifecycle, (2) image quality scoring, (3) OCR engine call (or sidecar HTTP), (4) field validity check, (5) database write, (6) MinIO upload, (7) account-closure webhook processing. Attributes include verification ID, account ID, verification type, result state. No sensitive data in spans. |
| **NFR-18** | Health checks | Liveness: service responds to GET /health within 100ms. Readiness: database reachable, MinIO bucket writable, OCR engine reachable (or cached result available if engine is sidecar). |

### Scalability

| ID | Requirement | Details |
|---|---|---|
| **NFR-19** | Horizontal scalability | Service is stateless (no in-memory session state, no sticky routing). API key cache is local but backed by database; invalidation propagated via TTL + background refresh. Multiple replicas can run without coordination. |
| **NFR-20** | Database scalability | Postgres single-instance for `banking_kyc`. Connection pooling at application layer (e.g., `pgx`). No cross-service joins; kyc-svc owns all tables. Index on `(account_id, created_at)` for retention queries. |
| **NFR-21** | Image storage scalability | MinIO bucket can hold 10M+ images (platform infra responsibility, not service-level tuning). Service assumes bucket exists and is writable; does not create/manage lifecycle policy (ops team manages MinIO). |

---

## Constraints

| ID | Constraint | Rationale |
|---|---|---|
| **C-01** | Pure Go service (except sidecar OCR engines) | Consistency with platform tech stack. Python/Java forbidden for app logic; only allowed for offline benchmark sidecar. |
| **C-02** | No auth-svc or audit-svc dependency | kyc-svc must be extractable into external product later. Self-contained auth (API keys) + audit (own database table). See ADR-0001. |
| **C-03** | Use chi router only | Consistency with platform (chi + stdlib, no heavy frameworks like Gin/Echo). Rationale: minimal overhead, explicit routing, easy to test. |
| **C-04** | Postgres for structured data | banking_kyc database must be Postgres (consistency with account-svc, auth-svc). No SQLite, no MongoDB. Schema versioning via SQL migrations. |
| **C-05** | MinIO for binary images | No Postgres BYTEA for KTP images. Rationale: MinIO scales for binary blobs; BYTEA bloats backups. See ADR-0002. |
| **C-06** | OCR engine language/platform agnostic | Benchmark may evaluate engines in any language (Go, Python, Java, etc.). Production runs engine that wins accuracy benchmark, regardless of language/technology. If engine requires external process or sidecar, include in deployment. Language choice based on accuracy + maintainability, not platform consistency. |
| **C-07** | Account ID is opaque string | Do not assume UUID format or any specific account-svc schema. Account ID is passed by caller; kyc-svc treats it as opaque foreign key string. |
| **C-08** | No external identity provider | kyc-svc does not call third-party KYC services (e.g., clearview.ai, idemia). OCR + validation is in-house only. Future work may integrate external providers; out of scope for v1. |

---

## Assumptions

| ID | Assumption | Risk if Invalid |
|---|---|---|
| **A-01** | account-svc will signal account closure to kyc-svc | Retention tail countdown cannot begin; images persist indefinitely, compliance risk. Mitigation: kyc-svc implements account closure listener (webhook or NATS subscription); account-svc integration is FR-17. |
| **A-02** | MinIO will be available on banking-net | Image archival will fail; verification requests blocked. Mitigation: health check verifies MinIO writability before serving; readiness probe includes MinIO. |
| **A-03** | Platform encryption (Postgres column-level, etc.) is transparent to app | kyc-svc reads/writes NIK as plaintext; platform infrastructure encrypts automatically. Mitigation: platform manages encryption key lifecycle; kyc-svc has no encryption/decryption logic to maintain. |
| **A-04** | OCR benchmark dataset exists (labeled KTPs) | Cannot reliably select winning engine; may choose suboptimally. Mitigation: platform obtains labeled dataset before implementation. Benchmark is FR-18 blocker. |
| **A-05** | Python sidecar (if used in production) will have stable deployment story | If sidecar is fragile or hard to operate, production complexity increases. Mitigation: ops team evaluates sidecar stability; if Tesseract is accurate enough (FR-18), select pure-Go instead. |
| **A-06** | Verification requests are idempotent (same image + account → same result) | If OCR engine introduces randomness per run, retries produce different results. Mitigation: select deterministic OCR engine in benchmark (FR-18); document any non-determinism. |
| **A-07** | NTP is synchronized across services | Retention tail countdown depends on clock accuracy. Mitigation: platform runs time-sync service; kyc-svc assumes clock skew < 1s. |

---

## Acceptance Criteria

### Verification & Scoring

| AC ID | Linked to | Criterion | Verification Method |
|---|---|---|---|
| **AC-01** | FR-01, FR-02 | Service accepts and processes a valid KTP image, extracts 9+ fields | Run e2e test with sample KTP image; assert extraction JSON contains NIK, Nama, DOB, etc. |
| **AC-02** | FR-03 | Image quality score is computed and returned in 0–100 range | Unit test: low-quality images (blurred, cropped) score < 50; sharp images score > 80. |
| **AC-03** | FR-04 | OCR confidence per field is reported in 0–100 range | Unit test: OCR engine returns confidence for each extracted field; response includes all values. |
| **AC-04** | FR-05 | Field validity score penalizes structurally invalid data (e.g., 5-digit NIK, unparseable DOB) | Unit test: NIK "12345" scores < 30; valid NIK "1234567890123456" scores > 80. Malformed DOB scores 0. |
| **AC-05** | FR-06, FR-07 | Verification state transitions correctly: `processing` → `verified/needs_review/rejected`; all three scores returned | Happy path: all scores > 70, state = verified. Degraded: image_quality < 50, state = needs_review. Failure: OCR empty, state = rejected. |
| **AC-06** | FR-08 | Verification request and result persisted to database; queryable by ID and account ID | Insert verification → select by id → assert all fields match. Query by account_id → assert all verifications for that account returned. |

### PII Handling & Retention

| AC ID | Linked to | Criterion | Verification Method |
|---|---|---|---|
| **AC-07** | FR-09 | Extracted NIK is stored securely (platform encryption); not readable in plaintext backup | Insert verification → query raw backup (if possible) → assert NIK not readable as plaintext. Verification record queryable via app → app reads NIK as plaintext (platform decrypts transparently). |
| **AC-08** | FR-10 | KTP image is archived in MinIO, not in Postgres BYTEA | Insert verification → assert Postgres `image_reference` points to S3 path. Query MinIO → assert image retrieved. |
| **AC-09** | FR-11, FR-12 | On account closure, retention tail starts; image deleted after tail expires (tested with 1-second tail in test) | Simulate account closure → verify database marks retention_started_at. Wait tail expiry → background job deletes from MinIO → verify deletion logged, image unavailable. |
| **AC-10** | FR-13 | No sensitive data (NIK, extraction, raw image reference) appears in application logs | Run verification → grep logs for account_id and nik → assert nik NOT found. Extraction JSON NOT logged. Image reference only appears in debug log (if at all). |

### API & Authentication

| AC ID | Linked to | Criterion | Verification Method |
|---|---|---|---|
| **AC-11** | FR-14 | API key is issued, validated, and rejected when invalid | Generate key for account-svc → POST /v1/verify with key → assert 200. POST without key → assert 401. POST with wrong key → assert 401. |
| **AC-12** | FR-15 | POST /v1/verify accepts valid request and returns complete response | POST `{ image: base64, type: "ktp_ocr", account_id: uuid }` → assert 200, response includes id, state, all three scores, extraction (nik encrypted/redacted). |
| **AC-13** | FR-16 | Error responses include correct HTTP status and error code | Bad image → 400. Invalid account_id format → 422. Rate limit exceeded → 429. Auth failure → 401. DB error → 500. |
| **AC-14** | FR-17 | Service consumes account closure signal and updates retention state | Simulate account-svc webhook: `POST /internal/account-closed { account_id }` → verify database flags retention_started_at for all verifications. (Or: subscribe to NATS topic and assert message consumed.) |

### Benchmark & Engine Selection

| AC ID | Linked to | Criterion | Verification Method |
|---|---|---|---|
| **AC-15** | FR-18, FR-19 | Benchmark harness compares Tesseract and Python engines on labeled dataset | Run benchmark offline. Output: accuracy table (Tesseract vs. PaddleOCR vs. docTR). Select winner. Document choice. |
| **AC-16** | FR-19 | OCREngine interface allows hot-swap between Tesseract and sidecar | Inject Tesseract impl → run verification → assert correctness. Inject sidecar impl → run same verification → assert same result (or documented difference, e.g., sidecar is more accurate but slower). |

### Performance

| AC ID | Linked to | Criterion | Verification Method |
|---|---|---|---|
| **AC-17** | NFR-01, NFR-02 | Verification latency P50 < 1s, P99 < 3s | Load test: 100 concurrent requests, measure latency distribution. Assert P50 < 1s, P99 < 3s. |
| **AC-18** | NFR-03 | API key validation < 10ms | Unit/integration test: validate 1000 keys in memory, measure mean time. Assert < 10ms. |
| **AC-19** | NFR-04 | Service handles 100 concurrent requests without latency regression | Load test: ramp to 100 concurrent, hold 60s, measure latency. Assert P99 latency does not increase > 20% vs. sequential. |

### Security & Compliance

| AC ID | Linked to | Criterion | Verification Method |
|---|---|---|---|
| **AC-20** | NFR-09 | NIK is encrypted at rest using pkg/crypto RSA/AES | Code review: assert all NIK storage uses Encrypt(). Attempt to read nik_encrypted from DB as plaintext → assert gibberish, not readable. |
| **AC-21** | NFR-11 | Every verification request is logged to audit_log table with requester, timestamp, scores | Insert verification → query audit_log → assert entry exists with requester (API key holder), timestamp, scores. |
| **AC-22** | NFR-13 | Rate limit enforced: > 100 requests/min from same key returns 429 | Load test: send 150 requests/min from same key → assert first 100 succeed, next 50 return 429. |

### Observability

| AC ID | Linked to | Criterion | Verification Method |
|---|---|---|---|
| **AC-23** | NFR-15, NFR-16, NFR-17 | Metrics, traces, and logs are produced for a full verification request | Run verification, scrape Prometheus, query Jaeger, grep logs → assert all three signals present with correct values (latency, scores, verification ID, account ID, state). |
| **AC-24** | NFR-18 | Health check returns 200 OK with liveness + readiness status | GET /health → assert 200, body includes `{ "live": true, "ready": true }`. Ready = DB + MinIO + OCR engine reachable. |

---

## Service Boundaries

### **kyc-svc Owns:**
- ✅ Verification request lifecycle (accept, validate, OCR, score, store, return result)
- ✅ OCR engine abstraction (pluggable: Tesseract, sidecar, others)
- ✅ Image quality and field validity scoring
- ✅ API key issuance and validation (independent of auth-svc)
- ✅ Encrypted NIK storage
- ✅ MinIO image archival and lifecycle
- ✅ Retention tail countdown and automated image cleanup
- ✅ Verification audit log (independent of audit-svc, tied to banking_kyc)
- ✅ Offline OCR benchmark harness (non-production utility)

### **kyc-svc Does NOT Own:**
- ❌ Account creation, deactivation, or balance management (account-svc owns)
- ❌ User authentication for the broader platform (auth-svc owns JWT issuance; kyc-svc uses API keys only)
- ❌ Platform-level audit bus (audit-svc owns NATS contract; kyc-svc may emit events later, but canonical audit log is local to kyc-svc)
- ❌ KYC decision logic (this is account-svc's choice: accept/reject account based on verification result + custom rules)
- ❌ External identity providers or fraud scoring services (out of scope; may be future integration point)
- ❌ Customer communication or notification (account-svc or separate notification-svc owns)

---

## Approval & Sign-Off

**Document Version:** 1.0  
**Date:** 2026-07-13  
**Author:** Engineering Team  
**Status:** Draft — Ready for Review

---

## Legend

- **BO** — Business Objective (why the service exists)
- **FR** — Functional Requirement (observable behaviour)
- **NFR** — Non-Functional Requirement (quality attribute with measurable target)
- **C** — Constraint (hard limit that shapes decisions)
- **A** — Assumption (presumed true, risks noted)
- **AC** — Acceptance Criterion (testable condition confirming requirement met)
- **R** — Risk (threat to success; mitigated or accepted)