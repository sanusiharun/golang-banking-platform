# Feature Workflow — Requirement → Research → Build → Review

Orchestrates the full delivery pipeline for one feature/service change, end to end.
This command does not introduce new methodology — it sequences the skills and
commands that already exist in this repo and refuses to skip stages.

Invoke as: `/feature-workflow <service-name> <requirement description>`

---

## Stage 0 — Intake

- Identify the target service under `services/{service-name}/`. If it doesn't exist yet,
  confirm with the user this is a new service before scaffolding anything.
- Restate the requirement back to the user in 2-3 sentences before proceeding. If anything
  is ambiguous (scope, acceptance criteria, which service owns this), ask — do not guess.
- Create and check out a feature branch before touching any file:
  `git checkout -b feature/{service-name}-{short-slug}` off the current `main`. Every stage
  below (docs, code, commits) happens on this branch — never directly on `main`.
- Do not write code or docs in this stage.

## Stage 1 — Research → 5 Documents

Apply the backend delivery framework (see `/eng-delivery` for full rules). Produce or
update all five documents in `services/{service-name}/docs/`:

1. `goals.md` — business objectives, FR/NFR/C/A/AC IDs, service boundaries
2. `context.md` — domain overview, actors, workflows mapped to FR IDs, risks
3. `architecture.md` — Mermaid diagrams, package structure, API design, storage design,
   observability design (metrics/tracing/logging per `/eng-observability`)
4. `progress-tracking.md` — epics + `E{n}-T{m}` tasks derived from architecture.md,
   all marked not-started
5. `review.md` — shell only (section headings, no findings yet — nothing to review pre-build)

Rules:
- goals.md must exist and be internally consistent before context.md is drafted.
- Every downstream doc must trace back to an ID in goals.md (see traceability rules
  in `/eng-delivery`).
- Stop and show the user goals.md + architecture.md for confirmation before moving to
  Stage 2 — architecture decisions are expensive to unwind after code exists.

## Stage 2 — Build

Implement strictly against `architecture.md` and `/eng-standards` (folder layout,
layering, error wrapping, `pkg/httpx` responses, `slog` logging, repository pattern).

For each `E{n}-T{m}` task in progress-tracking.md, in order:
- Write the code (handler → service → repository → DAO, per the layering rule)
- Write unit tests alongside it (table-driven, per this repo's Go testing conventions)
- Wire observability per `/eng-observability` (metrics, spans, health checks) where the
  architecture doc calls for it
- Mark the task's status in `progress-tracking.md` immediately — do not batch updates
- Run `make lint` and `make test` for the affected service before moving to the next task

Do not implement anything not traceable to an FR/NFR/task ID. If you find yourself adding
something extra, stop and add the missing task to progress-tracking.md first.

## Stage 3 — Review

1. Run the full test suite and lint for the service (`make test`, `make lint`).
2. Update `HANDOFF.md` per this repo's git commit convention (required before every commit).
3. Commit the docs + implementation together on the feature branch (never split docs from
   the code they describe, never commit straight to `main`).
4. Ask the user for explicit confirmation, then push the feature branch and open a PR
   (`gh pr create` with `--base main --head feature/{service-name}-{short-slug}`) — this is
   a permission-required action, never do it silently.
5. **Before merge is even discussed**, run both `/code-review` (or `/code-review ultra` for
   the multi-agent cloud review) and `/security-review` against the PR — both are mandatory,
   not either/or, and both must run before asking the user to merge.
6. Fold verified findings back into `review.md`: requirement compliance table, architecture
   compliance table, code quality findings — each citing its FR/NFR/AC/architecture ID.
7. Fix findings on the same feature branch, push a follow-up commit, and note in the PR
   what changed. Do not silently drop a finding — either fix it or turn it into an explicit
   `TD-xx` entry in `progress-tracking.md`'s technical debt register with a stated severity.
8. Only after both reviews have run and high-severity findings are addressed does the PR go
   to the user for a merge decision. This workflow never merges on its own.

---

## Guardrails

- Never skip a stage or produce docs and code in the same pass "to save time" — the
  traceability chain is the point of this workflow.
- Never commit feature work directly to `main` — always on a `feature/{service-name}-{slug}`
  branch, so there's always something to open a PR from and `main` never needs to be rewound.
- Never open the PR or push without the user's explicit go-ahead (per this repo's own
  risk rules — pushing/PRs are shared-state actions).
- Never treat `/code-review` or `/security-review` as optional or interchangeable — a PR
  that only ran one of them has not completed Stage 3.
- If the user interrupts mid-workflow, leave `progress-tracking.md` accurately reflecting
  what's actually done — it's the resumption point for next time.
