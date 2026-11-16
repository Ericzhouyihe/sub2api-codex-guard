# Implementation Plan: Reusable Sub2API Upgrade Runbook

**Branch**: `feat/model-guard` (feature spec `001-sub2api-upgrade-runbook`) | **Date**: 2026-11-16 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/001-sub2api-upgrade-runbook/spec.md`

## Summary

Create a reusable, gated upgrade procedure for this Sub2API fork and execute it for the 2.21.5 release. The procedure merges the official `ranxi2001/sub2api` release tag into `feat/model-guard`, preserves Model Guard routes/UI/assets/tests, runs targeted Go tests and an embed build, builds a fork-tagged container, validates it in an isolated full-role canary, promotes it by changing only the production compose image tag, verifies direct and public endpoints, and records rollback evidence.

The first execution is the 2.21.4 → 2.21.5 upgrade. The durable output is `docs/UPGRADING.md`, a command-level runbook whose release-specific values are the only inputs a future maintainer must replace.

## Technical Context

**Language/Version**: Go 1.27.4; Vue 3 + TypeScript + Vite 5; Node 24 build image

**Primary Dependencies**: Gin, Ent, PostgreSQL 18, Redis 8, pnpm 9.15.4, Docker 29.x, Docker Compose v5.x

**Storage**: PostgreSQL for application data; `/app/data` (host `./data`) for Model Guard history; Redis for runtime state

**Testing**: `go test -p 1 -tags unit ./internal/codexeval ./internal/service ./internal/repository ./internal/handler/admin -run 'Test(Codex|ModelGuard|Fingerprint|HasStandalone)' -count=1`; `go build -p 1 -tags embed -trimpath ./cmd/server`; frontend i18n and Vite build inside Docker

**Target Platform**: Linux server (Ubuntu 24.04, x86_64), production container port 8080, canary port 18082

**Project Type**: Web service with Go backend, Vue frontend, PostgreSQL, Redis, Docker Compose deployment

**Performance Goals**: Upgrade gates complete within 90 minutes excluding image downloads; production recreation window under 2 minutes; rollback within 15 minutes

**Constraints**: Production host has sufficient RAM; frontend build needs `NODE_OPTIONS=--max-old-space-size=4096`; canary must use full role for admin/Model Guard routes but inspection disabled; existing data volumes must not be recreated; production has real upstream accounts, so verification must avoid unsolicited test runs

**Scale/Scope**: One production deployment, a small user/account base (counts verified at upgrade time), four Model Guard tabs; one canary at a time

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

No ratified project constitution exists; `.specify/memory/constitution.md` is still the placeholder template. The following gates are therefore derived directly from the feature specification and are mandatory:

- **Data safety**: verified backup before production change; production volumes preserved; rollback path recorded.
- **Fork preservation**: no merge resolution may remove Model Guard routes, UI entry points, embedded assets, tests, or the frontend build memory setting.
- **Canary first**: production image may change only after the candidate passes health, auth, version, Model Guard API, and Model Guard UI checks in an isolated full-role canary.
- **No duplicate inspection**: canary inspection settings disabled before startup; canary removed after validation.
- **Evidence**: every gate records command output, counts, endpoint status, screenshot path, and decision.

**Result**: PASS. No violations requiring complexity tracking.

## Project Structure

### Documentation (this feature)

```text
specs/001-sub2api-upgrade-runbook/
├── spec.md
├── plan.md
├── tasks.md
└── checklists/
    └── requirements.md

docs/
└── UPGRADING.md          # durable command-level runbook
```

### Source Code (repository root)

```text
backend/
├── cmd/server/           # VERSION, wire, main
├── internal/
│   ├── codexeval/        # Model Guard runtime, inspection, embedded UI
│   ├── handler/admin/    # admin handlers
│   ├── repository/       # account/settings persistence
│   ├── server/routes/    # admin route registration
│   └── service/          # account, inspection, update services
└── ent/                  # schema and generated code

frontend/
├── src/
│   ├── components/layout/AppSidebar.vue
│   ├── router/index.ts
│   └── views/admin/CodexEvalView.vue
└── tests/

deploy/
├── docker-compose.yml
└── docker-compose.local.yml

Dockerfile                # multi-stage frontend/backend/runtime build
```

**Structure Decision**: The runbook changes no application source. It operates on the existing Go/Vue monorepo, the root `Dockerfile`, and the server-side `$DEPLOY_DIR/docker-compose.yml`. The only repository additions are the spec artifacts and `docs/UPGRADING.md`.

## Phase Plan

### Phase 0 — Research (complete)

- Confirmed official release source is `ranxi2001/sub2api`, not the upstream vendor repository.
- Confirmed the fork's Model Guard registration points: `backend/internal/server/routes/admin.go`, `frontend/src/components/layout/AppSidebar.vue`, `frontend/src/router/index.ts`.
- Confirmed `RUNTIME_ROLE=gateway` skips admin/Model Guard initialization; canary must use `full` with inspection disabled.
- Confirmed frontend production build OOM at default heap; memory setting is required.
- Confirmed production uses host bind mounts and can be rolled back by image tag only.

### Phase 1 — Design (complete)

- Define inputs, gates, evidence, stop conditions, and rollback in `docs/UPGRADING.md`.
- Keep the runbook release-agnostic; include a worked 2.21.5 example.
- Keep secrets as environment/SSH inputs, never in the repository.

### Phase 2 — Tasks

- Execute the runbook for 2.21.5.
- Record results in `tasks.md` and the upgrade evidence section.
- Update the runbook with any newly discovered gate or failure mode.

## Complexity Tracking

No constitution violations. No additional projects, abstractions, or dependencies are introduced.
