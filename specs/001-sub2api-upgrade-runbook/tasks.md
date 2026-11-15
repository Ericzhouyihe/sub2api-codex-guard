# Tasks: Reusable Sub2API Upgrade Runbook

**Input**: `specs/001-sub2api-upgrade-runbook/spec.md`, `plan.md`, `docs/UPGRADING.md`

**Prerequisites**: `spec.md`, `plan.md`, `docs/UPGRADING.md`, production SSH access, verified backup

**Tests**: This is an operational feature. Mandatory tests are the Go unit tests, embed build, canary API checks, canary UI checks, and production post-deploy checks defined in the runbook.

## Phase 1: Setup

- [x] T001 Create the feature specification in `specs/001-sub2api-upgrade-runbook/spec.md`
- [x] T002 Create the requirements-quality checklist in `specs/001-sub2api-upgrade-runbook/checklists/requirements.md`
- [x] T003 Create the implementation plan in `specs/001-sub2api-upgrade-runbook/plan.md`
- [x] T004 Create the durable command-level runbook in `docs/UPGRADING.md`

## Phase 2: Foundational

- [x] T005 Fetch the official `v2.21.5` tag from `https://github.com/ranxi2001/sub2api.git` and confirm commit `b9feadf7dd6228a4117e360821d9994b7cc54897`
- [x] T006 Record the 2.21.5 baseline: production version, image, health, user count, account count, Model Guard history count
- [x] T007 Create and verify `/root/backups/model-guard-2.21.5-<timestamp>/` with `data.tar.gz`, `db-sub2api.dump`, compose, env, and inspect files

## Phase 3: User Story 1 — Safe upstream upgrade (P1) 🎯 MVP

**Goal**: Merge 2.21.5, build and validate the fork image, then promote it without data loss.

**Independent Test**: Production reports 2.21.5, `/health` is 200, and baseline user/account/history counts are unchanged.

- [x] T008 [US1] Merge `v2.21.5` into `feat/model-guard` with `--no-ff --no-commit`; confirm no unresolved conflicts
- [x] T009 [US1] Commit the merge as `merge: upgrade to Sub2API 2.21.5` and push `origin/feat/model-guard`
- [x] T010 [US1] Run targeted Go unit tests in `backend/` and record the result
- [x] T011 [US1] Build `backend` with `-tags embed` and record the binary version
- [x] T012 [US1] Build `ghcr.io/ranxi2001/sub2api:2.21.5-modelguard` on the server with `VERSION=2.21.5` and the merge commit
- [x] T013 [US1] Start an isolated full-role canary on port 18082 with `sub2api_canary_215`, inspection disabled
- [x] T014 [US1] Promote production by changing only the compose image tag and recreating `sub2api`
- [x] T015 [US1] Verify production health, version, administrator identity, and baseline data counts

## Phase 4: User Story 2 — Model Guard parity (P2)

**Goal**: Prove the fork feature still works after the upstream merge.

**Independent Test**: All four Model Guard tabs render, both Model Guard APIs return authenticated responses, and saved settings/history match the baseline.

- [x] T016 [US2] Verify `backend/internal/server/routes/admin.go` still registers `/codex-eval`, account codex-eval, and inspection-settings routes
- [x] T017 [US2] Verify `frontend/src/components/layout/AppSidebar.vue` and `frontend/src/router/index.ts` still expose `/admin/codex-eval`
- [x] T018 [US2] On the canary, verify `/api/v1/admin/codex-eval/state` and `/api/v1/admin/settings/codex-eval-inspection` return 401 without a token (not 404)
- [x] T019 [US2] On the canary, verify all four tabs, real accounts, saved inspection settings, and history count; save screenshots
- [x] T020 [US2] Repeat the Model Guard API and page checks on the production domain; save screenshots

## Phase 5: User Story 3 — Rollback readiness (P3)

**Goal**: Make a failed promotion recoverable within 15 minutes.

**Independent Test**: The previous image tag and backup paths are recorded, and a compose-only image revert is verified as the first rollback action.

- [x] T021 [US3] Record the previous image tag, compose backup path, backup directory, and data-restore command in the upgrade evidence
- [x] T022 [US3] Confirm the previous image still exists locally on the server and the compose backup is readable

## Phase 6: User Story 4 — Reuse for future releases (P4)

**Goal**: Ensure the runbook is release-agnostic and complete for the next maintainer.

- [x] T023 [US4] Re-read `docs/UPGRADING.md` and replace any 2.21.5-specific value that is presented as a universal rule with a variable
- [x] T024 [US4] Add any newly discovered failure mode or gate to the troubleshooting table
- [x] T025 [US4] Record the completed 2.21.5 evidence and final gate decisions in the upgrade record

## Phase 7: Polish

- [x] T026 [P] Verify `git status` contains only intended source/spec/docs changes plus untracked `.local-run/`, `data/`, and local `.dsh/` skills
- [x] T027 [P] Confirm no secret (SSH password, JWT, database password) was written into the repository
- [x] T028 Update `docs/UPGRADING.md` with the actual 2.21.5 merge commit and image ID after the build

## Dependencies & Execution Order

- T005–T007 must complete before T008.
- T009 must complete before T012 (server build needs the pushed merge commit).
- T010–T011 must pass before T012.
- T012 must complete before T013.
- T013–T020 must pass before T014 is considered successful; T014 may be executed only after canary gates pass.
- T021–T022 can be prepared before T014 but must be verified before declaring completion.
- T023–T028 run after production verification.

## Implementation Strategy

1. Complete setup and baseline/backup.
2. Merge and test locally; stop on any conflict or test failure.
3. Build the server image; stop on any build failure.
4. Validate the canary; stop on any 404, page failure, or data mismatch.
5. Promote production; stop and roll back on any post-deploy gate failure.
6. Record evidence and update the runbook with lessons from this execution.
