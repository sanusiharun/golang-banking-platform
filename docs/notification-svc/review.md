# notification-svc — Review

> **Status:** Shell — to be completed after first production-ready release.

## Requirement Compliance

| ID | Requirement | Status | Evidence |
|---|---|---|---|
| FR-01 | Multi-channel support | ⬜ Unverified | Stub providers scaffold exists; real providers pending (TD-02) |
| FR-02 | Extensible channel abstraction | ⬜ Unverified | Channel interface implemented; verify new channel registration flow |
| FR-03–FR-07 | Template management | ⬜ Unverified | |
| FR-08–FR-15 | Notification processing | ⬜ Unverified | |
| FR-16–FR-19 | Notification history | ⬜ Unverified | |
| FR-20–FR-24 | Scheduler | ⬜ Unverified | |
| FR-25–FR-27 | APIs | ⬜ Unverified | |
| FR-28–FR-31 | Observability | ⬜ Unverified | |
| NFR-01 | At-least-once delivery | ⬜ Unverified | Requires load test + failure injection |
| NFR-02 | Concurrent workers | ⬜ Unverified | |
| NFR-03 | Idempotency | ⬜ Unverified | |
| NFR-04 | Horizontal scaling | ⬜ Unverified | Requires `FOR UPDATE SKIP LOCKED` verification |
| NFR-05 | Graceful shutdown | ⬜ Unverified | |

## Architecture Compliance

To be completed post-implementation review.

## Technical Debt Summary

See `progress-tracking.md` for the full register. Priority items for resolution before production:

1. **TD-01** — Processing deadline / stale PROCESSING recovery (High)
2. **TD-02** — Real channel provider implementations (High)
3. **TD-03** — Distributed scheduler lock (Medium)

## Recommendations

### Immediate (before production)
- Implement real channel providers (TD-02)
- Add `processing_deadline` column and stale-recovery query (TD-01)
- Load test with k6 to validate NFR-01 and NFR-02

### Short-term
- Solve scheduler distributed lock with Postgres advisory lock (TD-03)
- Add recipient rate limiting (TD-05)

### Medium-term
- User notification preference management
- Multi-provider failover per channel
- Notification analytics dashboard
