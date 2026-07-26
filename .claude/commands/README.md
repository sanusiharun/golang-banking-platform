# Project Slash Commands

This project provides engineering slash commands for Claude Code. Type `/` in the Claude Code prompt to autocomplete.

## Available Commands

| Command | Purpose | When to Use |
|---|---|---|
| `/feature-workflow` | End-to-end pipeline: requirement → 5 docs → build → review/PR | Delivering one feature/service change start to finish, automatically |
| `/eng-delivery` | 5-document delivery framework | Starting a new service, reverse-engineering docs, post-delivery review |
| `/eng-standards` | Microservice coding standards | Building/reviewing service code, checking conventions, onboarding |
| `/eng-observability` | Monitoring & observability standards | Adding metrics/logs/traces, setting up alerts, operational readiness |

## Usage

### `/feature-workflow`

Runs the whole delivery pipeline for one feature/service change, sequencing the other
commands instead of ad-hoc-ing each stage:

```
Stage 0  Intake       — restate requirement, confirm scope
Stage 1  Research     — produce/update the 5 docs in services/{name}/docs/ (via /eng-delivery)
Stage 2  Build        — code + unit tests + observability per architecture.md (via /eng-standards, /eng-observability)
Stage 3  Review       — tests/lint, commit, PR (with your go-ahead), /code-review + /security-review, fold findings into review.md
```

Stops for your confirmation before Stage 2 (architecture is expensive to unwind after
code exists) and before opening any PR.

**Example prompt:**
```
/feature-workflow payment-svc
Add idempotent refund processing with a 24h dedupe window.
```

### `/eng-delivery`

Loads the backend delivery framework — the 5-document lifecycle that governs all service development:

```
goals.md → context.md → architecture.md → progress-tracking.md → review.md
```

**Use when:**
- Starting a new microservice from scratch
- Reverse-engineering an existing service into documentation
- Reviewing whether docs are complete and traceable
- Onboarding to an unfamiliar service

**Example prompt:**
```
/eng-delivery
Reverse-engineer payment-svc into the 5-document lifecycle.
```

### `/eng-standards`

Loads the canonical microservice standards — folder structure, layering, naming, error handling, HTTP patterns, repository pattern, testing.

**Use when:**
- Creating a new service and need the canonical folder layout
- Reviewing a PR for convention compliance
- Unsure how to structure a handler, service, or repository
- Writing tests and need the project's testing patterns

**Example prompt:**
```
/eng-standards
Review notification-svc for convention compliance.
```

### `/eng-observability`

Loads monitoring and observability standards — slog logging, Prometheus metrics, OpenTelemetry tracing, Grafana dashboards, alerting rules, SLO/SLI definitions, health checks.

**Use when:**
- Adding observability to a new service
- Setting up Grafana dashboards or Prometheus alerts
- Checking operational readiness before deployment
- Debugging and deciding which signals to check

**Example prompt:**
```
/eng-observability
Check if audit-svc meets the operational readiness checklist.
```

## How It Works

Commands live in `.claude/commands/*.md`. Claude Code auto-discovers them — no configuration needed. Each file contains the full engineering methodology as a system prompt that Claude applies to your request.

## Adding New Commands

Create a new `.md` file in `.claude/commands/`:

```
.claude/commands/your-command.md
```

The filename (minus `.md`) becomes the slash command name. Keep names short and prefixed by domain (`eng-`, `ops-`, `dev-`).
