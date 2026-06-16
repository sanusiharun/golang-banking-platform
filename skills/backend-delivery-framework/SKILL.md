---
name: backend-delivery-framework
description: Defines how backend projects are planned, designed, implemented, tracked, and reviewed using a five-document lifecycle (goals → context → architecture → progress-tracking → review) with full traceability from requirements to code.
---

# Backend Delivery Framework

Define how backend projects are planned, designed, implemented, tracked, and reviewed. This skill applies to microservices, backend APIs, greenfield projects, and reverse-engineering exercises. It is the governing methodology for all engineering work in this repository.

## When to Activate

- Starting a new backend service or microservice
- Reverse-engineering an existing service to extract documentation
- Reviewing whether a service has adequate documentation
- Onboarding a new engineer to an existing service
- Conducting a post-delivery or post-incident review
- Any time someone asks "where do I start?" for a new feature or service

---

## 1. Overview

Every backend project — new or existing — follows the same lifecycle:

```
goals.md  →  context.md  →  architecture.md  →  implementation  →  review.md
               (tracked via progress-tracking.md throughout)
```

These five documents are **not independent artefacts**. They form a connected chain where every downstream document is traceable to the one above it. The chain starts with requirements (goals) and ends with assessment (review).

---

## 2. Document Definitions

### 2.1 `goals.md` — Source of Truth

Everything originates here. It must exist before any other document is written.

**Required sections:**
- **Service identity** — name, port, database, owner domain, criticality tier
- **Business objectives** — `BO-xx` IDs, plain-language statements of what the service achieves for the business
- **Functional requirements** — `FR-xx` IDs, specific observable behaviours the service must exhibit
- **Non-functional requirements** — `NFR-xx` IDs, measurable quality attributes (latency targets, availability, security properties)
- **Constraints** — `C-xx` IDs, hard limits (language, framework, network policy, deployment platform)
- **Assumptions** — `A-xx` IDs, things presumed true that are not yet verified
- **Acceptance criteria** — `AC-xx` IDs, testable conditions that confirm each requirement is met
- **Service boundaries** — explicit list of what the service owns and what it does not

**Rules:**
- Every `FR`, `NFR`, `BO` must have a unique ID.
- Acceptance criteria must reference the FR or NFR they verify.
- Service boundaries must be explicit — list what is out of scope.

### 2.2 `context.md` — Domain Understanding

Translates requirements into implementation context. Explains *why* the service exists and *how* it fits the ecosystem.

**Required sections:**
- **Domain overview** — 2–4 sentences explaining the service's core concern
- **Business context** — why this service exists (compliance, operational need, architectural separation)
- **Service responsibilities** — table of capabilities the service owns
- **Bounded context diagram** — ASCII or Mermaid diagram showing the service boundary and its stores
- **Actors** — table of human and machine actors, their type, and how they interact
- **Business workflows** — numbered step-by-step flows for each primary use case (maps to `FR-xx`)
- **Upstream systems** — what the service reads from or calls, coupling strength
- **Downstream systems** — what reads from or calls this service
- **Dependencies map** — full dependency list (packages, clients, stores)
- **Risks** — `R-xx` IDs, threats and their mitigations
- **Assumptions revisited** — how each `A-xx` from goals.md shapes the implementation

**Rules:**
- Every workflow must reference the FR IDs it satisfies.
- Every risk must have a mitigation or a note that it is accepted with rationale.

### 2.3 `architecture.md` — Technical Design

Converts domain understanding into technical decisions. Must remain consistent with the project's established service patterns.

**Required sections:**
- **High-level architecture diagram** (Mermaid `graph TD`)
- **Service architecture** — layering diagram and rules
- **Component architecture** — entry point, handlers, services, repositories, DAOs
- **Package structure** — directory tree with one-line purpose per folder
- **Request lifecycle** — Mermaid `sequenceDiagram` for the primary happy path
- **Data flow** — decision trees for pluggable backends, caching, async publishing
- **Storage design** — SQL DDL (or equivalent) for all tables; Redis key patterns
- **API design** — all endpoints, method, path, request/response shape, auth requirement
- **Integration patterns** — how each external system is called (sync/async, fallback behaviour)
- **Security design** — authentication mechanisms table; threat → mitigation table; RBAC model
- **Observability design** — metrics (name, type, labels); tracing (which operations get spans, which attributes); logging; health checks
- **Scalability considerations**
- **Reliability considerations** — failure mode table for each external dependency

**Rules:**
- Every architectural decision must explicitly satisfy one or more `FR`, `NFR`, or constraint from goals.md.
- Mermaid diagrams are required for high-level architecture and request lifecycle. Additional diagrams are encouraged.
- If a design decision deviates from the established service pattern, the deviation must be justified.

### 2.4 `progress-tracking.md` — Implementation Tracking

Converts architecture into actionable work. Maintained throughout the implementation lifecycle.

**Required structure:**
- **Legend** — symbols for complete/in-progress/not-started/blocked/tech-debt
- **Epics** — named groups of related work aligned to a business capability or architectural concern
- **Tasks per epic** — `E{n}-T{m}` IDs; status; brief notes; file/location reference
- **Dependency graph** — shows which epics depend on other epics
- **Current blockers** — table: what is blocked, what it affects, who owns it, how to resolve
- **Technical debt register** — `TD-xx` IDs, description, severity, linked task

**Rules:**
- Every task must be traceable to at least one `FR`, `NFR`, or architectural component.
- Blocking tasks must be listed explicitly — never hidden in a task note.
- Technical debt is not shame. Register it openly with a severity rating.
- Update this file after every work session. Stale tracking is worse than no tracking.

### 2.5 `review.md` — Post-Implementation Assessment

Evaluates the service against the original requirements and architecture. Written after implementation (or during a major review milestone).

**Required sections:**
- **Requirement compliance** — table: every `FR` and `NFR` ID → pass / fail / unverified + evidence
- **Architecture compliance** — table: every architectural decision → compliant / deviated + finding
- **Code quality** — strengths and issues table (severity, location, finding, reference)
- **Maintainability** — dimensions table (naming, test coverage, error patterns, comment density)
- **Operational readiness** — table: health checks, metrics, tracing, logging, alerting, runbooks
- **Security posture** — table: each security control → status + finding
- **Reliability assessment** — failure scenario table
- **Technical debt summary** — pulled from progress-tracking.md, updated with current state
- **Risks** — updated from context.md with current mitigation status
- **Recommendations** — immediate, short-term, medium-term actions
- **Refactoring opportunities**

**Rules:**
- Every finding must reference the original `FR`, `NFR`, `BO`, or `AC` ID.
- Unverified NFRs (e.g. latency targets not yet measured) must be listed as `⬜ Unverified`, not `✅ Pass`.
- Recommendations must be actionable: file/function to change, not vague advice.

---

## 3. Documentation Lifecycle

### Greenfield project

```
Day 0:  Write goals.md fully. Draft context.md (domain sections only).
        Create empty architecture.md, progress-tracking.md, review.md shells.
Day 1+: Fill context.md as domain understanding develops.
        Write architecture.md as technical decisions are made.
        Populate progress-tracking.md as implementation begins.
Post-delivery: Write review.md.
```

### Reverse-engineering an existing service

```
Step 1: Read all source code thoroughly.
Step 2: Write goals.md (infer from code what the requirements must have been).
Step 3: Write context.md (explain domain, actors, workflows as they actually work).
Step 4: Write architecture.md (document as-built, not as-intended).
Step 5: Write progress-tracking.md (mark existing work complete; open items as ⬜).
Step 6: Write review.md (evaluate the existing implementation against the goals).
```

### Ongoing

- `goals.md` rarely changes after initial definition. A change here signals a requirement change — review its impact on architecture and progress tracking.
- `context.md` evolves as domain understanding improves.
- `architecture.md` evolves with technical decisions. When a decision is reversed, document why.
- `progress-tracking.md` is updated after every work session.
- `review.md` is updated at major milestones or post-incident.

---

## 4. Traceability Rules

| Rule | Description |
|---|---|
| **Requirement traceability** | Every task in progress-tracking.md references at least one `FR`, `NFR`, or `BO` ID. |
| **Architecture traceability** | Every architectural decision in architecture.md references the requirement(s) it satisfies. |
| **Review traceability** | Every finding in review.md references the original ID (FR, NFR, AC, etc.). |
| **Risk traceability** | Every risk in context.md and review.md references either a `C`, `A`, or `NFR` it threatens. |
| **Debt traceability** | Every TD item links to the task or review finding that identified it. |

---

## 5. ID Naming Conventions

| Prefix | Meaning | Example |
|---|---|---|
| `BO` | Business objective | `BO-01` |
| `FR` | Functional requirement | `FR-07` |
| `NFR` | Non-functional requirement | `NFR-03` |
| `C` | Constraint | `C-02` |
| `A` | Assumption | `A-04` |
| `AC` | Acceptance criterion | `AC-09` |
| `R` | Risk | `R-03` |
| `E{n}-T{m}` | Epic n, Task m | `E3-T06` |
| `TD` | Technical debt item | `TD-01` |

IDs are permanent. When an item is removed, do not reuse its ID. Mark it deleted with a note.

---

## 6. Delivery Workflow

```
1. Write / update goals.md
2. Review goals with team (or self-review for solo work)
3. Write / update context.md
4. Write / update architecture.md
5. Review architecture (identify risks and open questions)
6. Populate progress-tracking.md (epics + tasks)
7. Implement (update progress-tracking.md task status in real time)
8. Write / update review.md
9. Address high-severity review findings
10. Commit all five documents together with the implementation
```

---

## 7. Engineering Governance

- **Every PR** that changes a service's public API or storage schema must update architecture.md.
- **Every PR** that addresses a blocker or technical debt item must update progress-tracking.md.
- **Every production incident** that reveals a gap in the service's design must produce a finding in review.md and a new risk or TD item.
- `goals.md` is the single source of truth. If the code contradicts goals.md, either the code is wrong or goals.md needs updating — never silently accept the contradiction.

---

## 8. Applying This Framework to New Projects

When starting a new service:

1. Copy the five document shells from `docs/` of an existing service.
2. Clear the content (keep only the section headings and rules).
3. Fill `goals.md` before writing a single line of code.
4. Use the architecture.md of the most similar existing service as a reference — adapt, don't copy blindly.
5. The first commit of a new service must include at minimum a complete `goals.md` and a draft `context.md`.
