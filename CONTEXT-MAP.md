# Context Map

## Contexts

- [auth-svc](./services/auth-svc/CONTEXT.md) — identity, JWT/API-key issuance and verification
- [account-svc](./services/account-svc/CONTEXT.md) — account master data, balance, credit/debit
- [payment-svc](./services/payment-svc/CONTEXT.md) — transaction orchestration across payment products
- [notification-svc](./services/notification-svc/CONTEXT.md) — multi-channel notification delivery
- [audit-svc](./services/audit-svc/CONTEXT.md) — append-only audit event log
- [kyc-svc](./services/kyc-svc/CONTEXT.md) — customer identity verification (KTP OCR extraction and scoring)

## Relationships

- **payment-svc → account-svc**: payment-svc calls account-svc's debit/credit API to move funds; account-svc owns balance state.
- **auth-svc → all services**: every service verifies caller identity via auth-svc-issued JWT (local public-key check) or API key introspection. **Exception: kyc-svc**, which issues and validates its own service accounts/API keys — see [kyc-svc ADR 0001](./services/kyc-svc/docs/adr/0001-self-contained-auth-and-audit.md).
- **all services → audit-svc**: services publish audit events over NATS; audit-svc consumes and persists them. **Exception: kyc-svc**, which owns its own audit trail (see same ADR) and may additionally emit to the platform audit bus.
- **notification-svc**: consumes events (NATS) or direct API calls from other services to trigger notifications; does not call back into them.
- **account-svc → kyc-svc**: account-svc calls kyc-svc during onboarding to verify a customer's KTP; kyc-svc returns extracted fields plus a three-part score (OCR confidence, field validity, image quality).
- **account-svc → kyc-svc (closure signal)**: account-svc notifies kyc-svc when an account closes, starting the 5-year statutory retention tail on the archived KTP image — see [kyc-svc ADR 0002](./services/kyc-svc/docs/adr/0002-image-storage-and-pii-retention.md).
