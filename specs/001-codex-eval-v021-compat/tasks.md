# Tasks: Codex Eval v0.2.21 兼容版

**Input**: Design documents from `/specs/001-codex-eval-v021-compat/`

**Prerequisites**: [plan.md](plan.md) (required), [spec.md](spec.md) (required), [research.md](research.md), [data-model.md](data-model.md), [contracts/](contracts/), [quickstart.md](quickstart.md)

**Note**: 当前仓库已有一版可运行的 v0.2.21 构建；下列任务用于把它收敛为**可重复、可审查、可交付**的开源版本，补齐版本锁定、检查、文档与真实安装验证。若某项已满足，执行时先验证证据，再标记完成。

**Tests**: 本功能未要求 TDD；测试/验证任务作为交付前的验收步骤包含在各 User Story 中。

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3, US4)
- Include exact file paths in descriptions

## Path Conventions

- 本功能主体：`codex-eval/`
- 构建产物：`codex-eval/dist/build/`
- 文档：`docs/`、根 `README.md`、`codex-eval/README.md`
- 规格文档：`specs/001-codex-eval-v021-compat/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: 固定版本与构建基线，避免每次构建漂移

- [x] T001 Pin `UPSTREAM_TAG` 默认值为 `v0.2.21` in `codex-eval/scripts/build.py` (`resolve_upstream_tag`)，未显式提供版本时不得自动解析 GitHub latest
- [x] T002 [P] Generate `codex-eval/patches/SHA256SUMS` for all 7 LF-normalized patches and create `codex-eval/scripts/check_patches.py` to verify filename order, SHA-256, and LF line endings
- [x] T003 [P] Add stale-output cleanup and a failure marker in `codex-eval/scripts/build.py` so a failed build cannot leave old `codex-eval/dist/build/` artifacts looking successful
- [x] T004 [P] Create `codex-eval/compatibility.json` seeded with `{"upstream_version":"v0.2.21","fork_version":"v0.4.6","status":"planned"}` and fields from `data-model.md`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: 构建、产物校验与安装安全的核心基础设施；所有 User Story 依赖此阶段

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T005 Implement `codex-eval/scripts/record_compatibility.py` to validate and update `codex-eval/compatibility.json` with `patchset_revision`, `verified_at`, `artifacts`, `notes` per `data-model.md`
- [x] T006 [P] Implement `codex-eval/scripts/verify_artifacts.py` validating artifact names, non-empty files, SHA-256 against `SHA256SUMS`, and Linux ELF architecture per `contracts/artifact-layout.md`
- [x] T007 [P] Harden `codex-eval/scripts/installer/main.go`: assert backup file exists/non-empty before replacing `/app/sub2api`; on failure log backup path and restore original before exit
- [x] T008 [P] Add `codex-eval/scripts/installer/installer_test.go` covering architecture/version mismatch rejection and backup-before-replace invariant without requiring Docker
- [x] T009 Verify `codex-eval/go.mod` module isolation; run `go test ./...` at repository root and `go test -tags=unit -p 1 ./internal/codexeval` in the prepared `codex-eval/dist/work/source` workspace, recording both results in `docs/codex-eval-fork.md`

**Checkpoint**: Foundation ready — build outputs can be produced and checked deterministically

---

## Phase 3: User Story 1 - 在 Sub2API v0.2.21 上安装并使用完整评估功能 (Priority: P1) 🎯 MVP

**Goal**: 在真实 Sub2API v0.2.21 实例完成安装，账户评估页面与糖果测试可用，真实 API 不受影响，失败可回滚

**Independent Test**: 在一台 v0.2.21 测试实例运行安装器，打开「账户评估」并执行糖果测试；随后发一次真实 Codex 请求确认转发正常

### Implementation for User Story 1

- [x] T010 [US1] Build v0.2.21 Linux amd64 artifacts locally with `UPSTREAM_TAG=v0.2.21` and `SUB2API_BUILD_STAGE=source|resume`; record installer/server hashes in `codex-eval/compatibility.json`
- [x] T011 [US1] Add `codex-eval/scripts/verify-install.sh` to check installer file exists, is executable, reports version, and prints the expected backup/rollback checklist before any server-side action
- [ ] T012 [US1] Run the installer on a real Sub2API v0.2.21 test instance; verify 「账户评估」 page opens and a candy test completes; record result in `docs/codex-eval-fork.md`
- [ ] T013 [US1] Verify real Codex API traffic before/after installation (same account, model, prompt) and record that model/parameters/response are unchanged in `docs/codex-eval-fork.md`
- [ ] T014 [US1] Exercise the failure path: force an invalid candidate binary or health-check failure in the test instance, confirm original `/app/sub2api` is restored and the service is healthy; record evidence in `docs/codex-eval-fork.md`
- [ ] T015 [US1] Verify the 4 test entry points in the running UI (candy/fingerprint/pelican/SIWC), and document any host-dependent limitation in `docs/codex-eval-fork.md`

**Checkpoint**: User Story 1 is fully functional and independently testable on v0.2.21

---

## Phase 4: User Story 2 - 不依赖 GitHub Actions，也能本地构建安装包 (Priority: P1)

**Goal**: 从干净源码与本地命令构建 Linux amd64 安装器，全程不要求 GitHub Actions

**Independent Test**: 在没有 GitHub Actions、没有远程 CI 凭据的干净目录执行本地构建，产出 installer + server + SHA256SUMS

### Implementation for User Story 2

- [x] T016 [US2] Remove GitHub Actions as a delivery dependency: delete `.github/workflows/codex-eval-build.yml` and `codex-eval/.github/workflows/build.yml`, or move them to `docs/optional-ci/` with a clear "not required" note; update `README.md` and `codex-eval/README.md` accordingly
- [x] T017 [US2] Add `codex-eval/scripts/build.ps1` Windows wrapper that sets `UPSTREAM_TAG=v0.2.21`, `SUB2API_BUILD_STAGE=source|resume`, `GOMAXPROCS=2`, `SUB2API_GOFLAGS=-mod=readonly -p=2`, and `NODE_OPTIONS=--max-old-space-size=8192`
- [x] T018 [US2] Add preflight checks to `codex-eval/scripts/build.py` for Go/Python/Node/pnpm versions and clear missing-tool errors, with messages pointing to `docs/codex-eval-fork.md`
- [x] T019 [US2] Execute a clean-clone build in a fresh directory using only local commands; record the exact command sequence, elapsed time, and resulting artifact hashes in `docs/codex-eval-fork.md`

**Checkpoint**: User Story 2 independently produces a valid local build with no remote CI

---

## Phase 5: User Story 3 - 用尽量小的改动适配新版本 (Priority: P2)

**Goal**: 兼容改动集中、可审查、可回退，不变成对原项目的大面积重写

**Independent Test**: 对比 upstream v0.4.6 与当前 `codex-eval/`，统计改动文件/行数并生成审查报告；移除兼容补丁后可恢复 upstream 原始构建

### Implementation for User Story 3

- [x] T020 [US3] Generate a diff summary of `codex-eval/` against upstream `codex-candy-eval` v0.4.6 (changed files, added/deleted lines, purpose), and add it to `docs/codex-eval-fork.md`
- [x] T021 [US3] Review all 7 files in `codex-eval/patches/` and add `codex-eval/patches/README.md` with one-line purpose, target area, and upstream version range per patch
- [x] T022 [US3] Add `codex-eval/scripts/check_minimal_diff.py` that reports changed-file ratio and fails if compatibility changes exceed the 30% threshold from `spec.md` SC-004
- [x] T023 [US3] Verify a revert path: remove all 7 patches from a fresh v0.2.21 worktree, confirm upstream v0.2.21 source still builds without fork artifacts, and document steps in `docs/codex-eval-fork.md`

**Checkpoint**: Compatibility change set is reviewed and reversible

---

## Phase 6: User Story 4 - 在更新后的 Sub2API 中保留测试能力 (Priority: P3)

**Goal**: Sub2API 升级后可按文档重新安装兼容版并恢复评估页面

**Independent Test**: 从 v0.2.20 升级到 v0.2.21（或模拟镜像覆盖）后执行兼容版安装，确认页面恢复与回滚可用

### Implementation for User Story 4

- [x] T024 [US4] Write an upgrade/reinstall guide in `docs/codex-eval-fork.md` for v0.2.20/v0.2.19 → v0.2.21, including backup, reinstall, and data-preservation notes
- [ ] T025 [US4] Test the upgrade/reinstall scenario on a v0.2.21 test instance and record results/edge cases in `docs/codex-eval-fork.md`
- [x] T026 [US4] Add a documented rollback procedure (installer recovery or manual backup restore) to `docs/codex-eval-fork.md` and verify it end-to-end

**Checkpoint**: Upgrade and recovery path is reproducible

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: 文档、许可证、最终验收与发布准备

- [x] T027 [P] Update `README.md`, `codex-eval/README.md`, and `docs/codex-eval-fork.md` with final compatibility table, artifact hashes, known limitations, and explicit "no GitHub Actions required" statement
- [x] T028 [P] Verify license compliance in `LICENSE`, `codex-eval/LICENSE`, `THIRD_PARTY_NOTICES.md`, and `codex-eval/licenses/`; ensure no private keys or credentials are tracked
- [x] T029 Run [quickstart.md](quickstart.md) end-to-end on a clean machine and fix any discrepancy in `codex-eval/scripts/build.py` or documentation
- [x] T030 Create final git commit and annotated tag, run `git status --short` to confirm no build artifacts/secrets are tracked, and produce a release checklist in `docs/codex-eval-fork.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Setup — blocks all User Stories
- **User Stories (Phase 3+)**: Depend on Foundational
  - US1 and US2 both P1; can run in parallel with different owners
  - US3 P2 after US1/US2; US4 P3 after US1
- **Polish (Phase 7)**: Depends on all desired User Stories

### User Story Dependencies

- **US1 (P1)**: Starts after Foundational; no dependency on other stories
- **US2 (P1)**: Starts after Foundational; no dependency on US1 (build path is independent)
- **US3 (P2)**: Starts after Foundational; consumes build outputs from US2 for diff measurement
- **US4 (P3)**: Starts after US1; needs installer and rollback path

### Within Each User Story

- Prefer verifiable implementation tasks before documentation tasks.
- US1: build (T010) → install (T012) → traffic check (T013) → rollback check (T014) → UI check (T015).
- US2: remove CI (T016) → wrapper (T017) → preflight (T018) → clean build (T019).
- US3: diff summary (T020) → patch review (T021) → threshold check (T022) → revert test (T023).
- US4: guide (T024) → upgrade test (T025) → rollback (T026).

### Parallel Opportunities

- Phase 1: T002, T003, T004 can run in parallel after T001.
- Phase 2: T006, T007, T008 can run in parallel; T005 and T009 after related edits.
- US1: T011 and T015 can be prepared while T010 builds; T012–T014 must be sequential on the same test instance.
- US2: T017 and T018 can run in parallel after T016.
- US3: T020 and T021 can run in parallel; T022/T023 after.
- Polish: T027 and T028 can run in parallel.

---

## Parallel Example: User Story 1

```text
# Prepare checks while the build artifact is being produced:
Task: "Add codex-eval/scripts/verify-install.sh ..."
Task: "Verify the 4 UI entry points ..."

# Then sequentially on the same test server:
1. Build artifacts (T010)
2. Install (T012)
3. Verify traffic (T013)
4. Test rollback (T014)
```

## Parallel Example: User Story 2

```text
# After removing CI dependency (T016):
Task: "Add codex-eval/scripts/build.ps1 ..."
Task: "Add preflight checks to codex-eval/scripts/build.py ..."
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 Setup (version pin + patch checksums)
2. Complete Phase 2 Foundational (artifact verify + installer safety)
3. Complete Phase 3 US1 (build + real v0.2.21 install + candy test + rollback)
4. **STOP and VALIDATE**: this is the minimum usable delivery
5. Keep US2 local build path ready for reproducibility

### Incremental Delivery

1. Setup + Foundational → deterministic build base
2. US1 → v0.2.21 可安装、可用糖果测试（MVP）
3. US2 → 任何人本地可构建，不依赖 GitHub
4. US3 → 改动可审查、可回退
5. US4 → 升级/恢复路径完整
6. Polish → 文档、许可、发布检查

### Notes

- [P] tasks touch different files and have no dependency.
- [US1] is the MVP; [US2] is required for the "open-source, local build" promise.
- [US3]/[US4] improve maintainability and recovery; they do not block MVP.
- 不要提交 `dist/build/` 产物、私钥或 `.env` 文件。
