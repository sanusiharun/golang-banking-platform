# Self-contained auth and audit, not auth-svc/audit-svc

`kyc-svc` may be extracted into a standalone OCR/KYC provider product later, potentially serving external customers who have no account in this platform's `auth-svc`. Depending on `auth-svc`-issued JWTs or the shared `pkg/audit`/`audit-svc` NATS contract would tie `kyc-svc` to platform infrastructure that a future external caller can't participate in, and unwinding that dependency after the fact is a real rewrite, not a config change.

We decided `kyc-svc` issues and validates its own service accounts/API keys (own table, own middleware) and writes its own audit trail into its own database (`banking_kyc`), rather than reusing `auth-svc` or `audit-svc`. Today's only caller, `account-svc`, authenticates with a `kyc-svc`-issued API key — the same mechanism a future external customer would use. It may additionally emit events to the platform's audit bus for now, but its own audit log is canonical.

This is a deliberate deviation from every other service in this platform, which all authenticate via `auth-svc` and audit via `audit-svc`. `kyc-svc` still lives in this monorepo under `services/kyc-svc/` and reuses generic `pkg/` utilities (`httpx`, `errors`, `logger`, `observability`) — only the auth and audit *contracts* are self-contained, not the whole service.
