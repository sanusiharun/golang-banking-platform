# notification-svc — Progress Tracking

## Legend

| Symbol | Meaning |
|---|---|
| ✅ | Complete |
| 🔄 | In progress |
| ⬜ | Not started |
| 🚫 | Blocked |
| 💸 | Technical debt |

---

## Epics

### E1: Framework & Documentation
| Task | Status | Notes | Location |
|---|---|---|---|
| E1-T01 | ✅ | goals.md | `docs/notification-svc/goals.md` |
| E1-T02 | ✅ | context.md | `docs/notification-svc/context.md` |
| E1-T03 | ✅ | architecture.md | `docs/notification-svc/architecture.md` |
| E1-T04 | ✅ | progress-tracking.md (this file) | `docs/notification-svc/progress-tracking.md` |
| E1-T05 | ⬜ | review.md (post-delivery) | `docs/notification-svc/review.md` |

### E2: Service Scaffolding & Config
| Task | Status | Notes | Location |
|---|---|---|---|
| E2-T01 | ✅ | go.mod | `services/notification-svc/go.mod` |
| E2-T02 | ✅ | config/config.go | `services/notification-svc/config/config.go` |
| E2-T03 | ✅ | Dockerfile | `services/notification-svc/Dockerfile` |
| E2-T04 | ✅ | Makefile | `services/notification-svc/Makefile` |
| E2-T05 | ✅ | .env.example | `services/notification-svc/.env.example` |
| E2-T06 | ✅ | go.work updated | `go.work` |
| E2-T07 | ✅ | docker-compose.yml updated | `docker-compose.yml` |
| E2-T08 | ✅ | prometheus.yml updated | `prometheus.yml` |

### E3: Database & Migrations (FR-16, FR-03, FR-20)
| Task | Status | Notes | Location |
|---|---|---|---|
| E3-T01 | ✅ | 001_create_notifications.up.sql | `migrations/` |
| E3-T02 | ✅ | 002_create_templates.up.sql | `migrations/` |
| E3-T03 | ✅ | 003_create_schedules.up.sql | `migrations/` |
| E3-T04 | ✅ | migrations.go (embed FS) | `migrations/` |
| E3-T05 | ✅ | cmd/server/migrate.go | `cmd/server/migrate.go` |

### E4: Domain Layer (FR-16, FR-03, FR-20)
| Task | Status | Notes | Location |
|---|---|---|---|
| E4-T01 | ✅ | dao/notification.go | `internal/domain/dao/` |
| E4-T02 | ✅ | dao/template.go | `internal/domain/dao/` |
| E4-T03 | ✅ | dao/schedule.go | `internal/domain/dao/` |
| E4-T04 | ✅ | dto/notification.go | `internal/domain/dto/` |
| E4-T05 | ✅ | dto/template.go | `internal/domain/dto/` |
| E4-T06 | ✅ | dto/schedule.go | `internal/domain/dto/` |

### E5: Channel Abstraction (FR-01, FR-02)
| Task | Status | Notes | Location |
|---|---|---|---|
| E5-T01 | ✅ | channel.go — Channel interface | `internal/channel/` |
| E5-T02 | ✅ | registry.go — Registry | `internal/channel/` |
| E5-T03 | ✅ | email/email.go — stub | `internal/channel/email/` |
| E5-T04 | ✅ | sms/sms.go — stub | `internal/channel/sms/` |
| E5-T05 | ✅ | push/push.go — stub | `internal/channel/push/` |
| E5-T06 | ✅ | whatsapp/whatsapp.go — stub | `internal/channel/whatsapp/` |
| E5-T07 | ✅ | webhook/webhook.go — real HTTP POST impl | `internal/channel/webhook/` |

### E6: Template Engine (FR-04, FR-05, FR-07)
| Task | Status | Notes | Location |
|---|---|---|---|
| E6-T01 | ✅ | template/engine.go | `internal/template/` |

### E7: Repository Layer (FR-16, FR-18, FR-19)
| Task | Status | Notes | Location |
|---|---|---|---|
| E7-T01 | ✅ | notification_repository.go | `internal/repository/` |
| E7-T02 | ✅ | template_repository.go | `internal/repository/` |
| E7-T03 | ✅ | schedule_repository.go | `internal/repository/` |

### E8: Service Layer (FR-08–FR-15, FR-25–FR-27)
| Task | Status | Notes | Location |
|---|---|---|---|
| E8-T01 | ✅ | notification_service.go | `internal/services/` |
| E8-T02 | ✅ | template_service.go | `internal/services/` |
| E8-T03 | ✅ | scheduler_service.go | `internal/services/` |

### E9: Worker Layer (FR-10–FR-13, FR-24, NFR-01, NFR-02)
| Task | Status | Notes | Location |
|---|---|---|---|
| E9-T01 | ✅ | worker/dispatcher.go | `internal/worker/` |
| E9-T02 | ✅ | worker/scheduler.go | `internal/worker/` |

### E10: Transport Layer (FR-08, FR-25–FR-27)
| Task | Status | Notes | Location |
|---|---|---|---|
| E10-T01 | ✅ | transport/notification_handler.go | `internal/transport/` |
| E10-T02 | ✅ | transport/template_handler.go | `internal/transport/` |
| E10-T03 | ✅ | transport/schedule_handler.go | `internal/transport/` |
| E10-T04 | ✅ | transport/consumer.go (NATS) | `internal/transport/` |
| E10-T05 | ✅ | transport/routes.go | `internal/transport/` |
| E10-T06 | ✅ | transport/errors.go | `internal/transport/` |

### E11: Entry Point & DI (NFR-05, NFR-09)
| Task | Status | Notes | Location |
|---|---|---|---|
| E11-T01 | ✅ | cmd/server/main.go | `cmd/server/` |
| E11-T02 | ✅ | cmd/server/container.go | `cmd/server/` |

---

## Dependency Graph

```
E1 (docs) → no code dependencies
E2 (scaffold) → E1
E3 (DB) → E2
E4 (domain) → E3
E5 (channel) → E4
E6 (template) → E4
E7 (repository) → E4
E8 (service) → E5, E6, E7
E9 (worker) → E8
E10 (transport) → E8
E11 (entry point) → E9, E10
```

---

## Current Blockers

None.

---

## Technical Debt Register

| ID | Description | Severity | Linked Task |
|---|---|---|---|
| TD-01 | Worker does not reset stale PROCESSING notifications (no processing deadline field). If a worker crashes mid-delivery, the notification is stuck in PROCESSING forever. | High | E9-T01 |
| TD-02 | Channel provider implementations are stubs. Real SMTP/SMS/FCM/WhatsApp SDK integration is required for production. | High | E5-T03 – E5-T06 |
| TD-03 | Scheduler on multiple instances may create duplicate notification records for the same schedule tick (no distributed lock). next_run_at update is not atomic across instances. | Medium | E9-T02 |
| TD-04 | No user notification preference management. All notifications are sent regardless of user opt-out. | Medium | Out of scope v1 |
| TD-05 | No rate limiting per recipient. High-volume callers can trigger floods. | Medium | Out of scope v1 |
| TD-06 | Template deletion is soft-delete only. No garbage collection of inactive templates. | Low | E8-T02 |
