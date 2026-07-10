# Context Map

## Contexts

- [auth-svc](./services/auth-svc/CONTEXT.md) — identity, JWT/API-key issuance and verification
- [account-svc](./services/account-svc/CONTEXT.md) — account master data, balance, credit/debit
- [payment-svc](./services/payment-svc/CONTEXT.md) — transaction orchestration across payment products
- [notification-svc](./services/notification-svc/CONTEXT.md) — multi-channel notification delivery
- [audit-svc](./services/audit-svc/CONTEXT.md) — append-only audit event log

## Relationships

- **payment-svc → account-svc**: payment-svc calls account-svc's debit/credit API to move funds; account-svc owns balance state.
- **auth-svc → all services**: every service verifies caller identity via auth-svc-issued JWT (local public-key check) or API key introspection.
- **all services → audit-svc**: services publish audit events over NATS; audit-svc consumes and persists them.
- **notification-svc**: consumes events (NATS) or direct API calls from other services to trigger notifications; does not call back into them.
