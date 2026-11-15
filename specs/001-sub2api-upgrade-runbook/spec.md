# Feature Specification: Reusable Sub2API Upgrade Runbook

**Feature Branch**: `001-sub2api-upgrade-runbook`

**Created**: 2026-11-16

**Status**: Implemented (2.21.5 upgrade complete)

**Input**: User description: "现在又有2.21.5的版本了,能不能帮我更新一下,并且我在本项目安装了speckit的skill,能不能用对应的skill保留更新的相关流程,方便以后的所有更新"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Perform a safe upstream upgrade (Priority: P1)

A maintainer receives a new upstream Sub2API release and follows one documented runbook to move the forked deployment from its current release to the new release without losing data or Model Guard behavior.

**Why this priority**: An upgrade that loses accounts, history, or the Model Guard management surface is worse than staying on the old release. This is the primary outcome the user asked for.

**Independent Test**: Start from a recorded baseline, execute the runbook against a new release tag, and confirm the production version, data counts, and Model Guard pages after promotion.

**Acceptance Scenarios**:

1. **Given** a clean feature branch and a new official release tag, **When** the maintainer merges the tag and completes all gates, **Then** production reports the new version and all pre-upgrade users, accounts, settings, and history are still present.
2. **Given** a completed merge with no conflicts, **When** the maintainer builds and validates a canary, **Then** the canary exposes the Model Guard management API and UI before production is changed.
3. **Given** a failed gate at any stage, **When** the maintainer stops the rollout, **Then** production remains on the previous image and the backup remains available.

---

### User Story 2 - Prove Model Guard parity after the upgrade (Priority: P2)

A maintainer verifies that the fork-specific Model Guard feature still works after upstream changes: routes, admin UI, candy/fingerprint/history/settings tabs, saved settings, and historical records.

**Why this priority**: Model Guard is the reason this fork exists. An upgrade that compiles but silently drops the feature is not acceptable.

**Independent Test**: On the canary, call the Model Guard state and inspection-settings endpoints with an administrator token and open all four Model Guard tabs; repeat the page check on the production domain.

**Acceptance Scenarios**:

1. **Given** a canary running the new image, **When** an administrator opens `/admin/codex-eval`, **Then** the page loads inside the admin shell and all four Model Guard tabs render without errors.
2. **Given** existing inspection settings and history, **When** the new version starts, **Then** the saved group, model, schedule, and history records are visible and unchanged.
3. **Given** no administrator token, **When** the Model Guard API is called, **Then** it returns an authentication error rather than a missing-route error.

---

### User Story 3 - Roll back quickly if the upgrade fails (Priority: P3)

A maintainer restores the previous image and, only if necessary, the previous data snapshot when a post-deploy gate fails.

**Why this priority**: Rollback is the safety net that makes an in-place production upgrade acceptable.

**Independent Test**: In a non-production drill, switch the compose image back to the recorded previous tag, recreate the service, and verify health and version.

**Acceptance Scenarios**:

1. **Given** a failed post-deploy health check, **When** the maintainer restores the previous image tag, **Then** the service becomes healthy on the previous version within the rollback target.
2. **Given** data corruption is confirmed, **When** the maintainer restores the recorded database and data snapshot, **Then** the pre-upgrade user and account counts are restored.

---

### User Story 4 - Reuse the process for every future release (Priority: P4)

A maintainer opens the runbook, replaces the old and new release identifiers, and executes the same gates without rediscovering commands, ports, or rollback steps.

**Why this priority**: The user explicitly asked to preserve the process for all future updates, not only for 2.21.5.

**Independent Test**: Hand the runbook to a maintainer who did not perform the 2.21.5 upgrade and confirm they can identify every required input, gate, and rollback action without asking for missing steps.

**Acceptance Scenarios**:

1. **Given** a future release tag, **When** the maintainer follows the runbook, **Then** only release-specific values need to change.
2. **Given** a new maintainer, **When** they read the runbook, **Then** they can distinguish mandatory gates from optional diagnostics.

### Edge Cases

- Upstream changes a file that also contains Model Guard registration code; the merge must preserve both upstream behavior and the fork routes.
- Upstream removes or renames a dependency used by Model Guard; the build or tests must fail before production is touched.
- Frontend production build exceeds the default Node heap; the documented memory setting must be applied.
- The new release changes database schema or settings keys; the canary must prove migration and saved settings survive.
- The canary starts background jobs such as scheduled inspection; duplicate upstream consumption must be prevented.
- The canary binds only to loopback or the operator's HTTP proxy intercepts the request; validation must distinguish application failure from access-path failure.
- The release version string contains a fork suffix; the built-in update checker must still consider the installation current.
- A backup is missing, incomplete, or from the wrong release; the rollout must stop before changing production.
- A post-deploy check fails after the container is recreated; rollback must not require rebuilding the previous image.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The runbook MUST identify the official upstream repository and resolve the requested release to an immutable commit before any merge.
- **FR-002**: The runbook MUST record the current branch, commit, production image, running version, health status, user count, account count, and Model Guard history count as the upgrade baseline.
- **FR-003**: The runbook MUST require a verified backup of the production data directory, database, compose file, and environment file before the production image is changed.
- **FR-004**: The merge step MUST preserve all Model Guard backend routes, frontend entry points, embedded assets, tests, and the fork-specific frontend build memory setting.
- **FR-005**: The runbook MUST require a merge commit with no unresolved conflicts; any conflict MUST be resolved by preserving both upstream changes and Model Guard behavior, then re-tested.
- **FR-006**: The runbook MUST require the repository's targeted Go unit tests and an embed-tagged backend build to pass before a container image is built.
- **FR-007**: The runbook MUST build a new container image whose reported application version matches the upstream release, while the image tag carries a fork identifier.
- **FR-008**: The runbook MUST validate the new image in an isolated canary with its own database, data directory, port, and credentials before production is changed.
- **FR-009**: The canary MUST run with the full application role required for admin and Model Guard routes, while automatic inspection is disabled to avoid duplicate upstream consumption.
- **FR-010**: The runbook MUST verify canary health, administrator authentication, reported version, Model Guard API authentication, all four Model Guard tabs, saved settings, and historical records.
- **FR-011**: Production promotion MUST preserve the existing data volumes and MUST be reversible by changing only the image tag and recreating the service.
- **FR-012**: Post-deploy verification MUST cover the direct server endpoint and the public domain, the reported version, health, administrator identity, Model Guard state, inspection settings, and recent logs.
- **FR-013**: The runbook MUST define explicit stop conditions and a rollback procedure for merge, test, build, canary, promotion, and post-deploy failures.
- **FR-014**: The runbook MUST record the completed upgrade evidence and MUST be structured so a future release only requires replacing release-specific values.

### Key Entities

- **Release candidate**: An official upstream tag, its immutable commit, release notes, and the fork commit that merges it.
- **Upgrade runbook**: The ordered, reusable procedure containing inputs, gates, verification evidence, and rollback actions.
- **Backup set**: The timestamped production data archive, database dump, compose file, environment file, and container/image metadata needed to restore service.
- **Canary environment**: An isolated application instance using the candidate image, a copy of production data, a separate database, and a non-production port.
- **Verification record**: The observed version, health responses, endpoint statuses, page checks, data counts, screenshot paths, and decision for each gate.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A maintainer can complete the merge, test, build, canary, promotion, and post-deploy verification for a new release in under 90 minutes, excluding upstream image download time.
- **SC-002**: The upgrade preserves 100% of pre-upgrade users, accounts, Model Guard settings, and Model Guard history records.
- **SC-003**: 100% of mandatory Model Guard checks pass on the canary and on production before the upgrade is declared complete.
- **SC-004**: A failed promotion can be returned to the previous healthy version within 15 minutes using the recorded image tag and backup set.
- **SC-005**: A future maintainer can execute the runbook for a different release by changing only the old/new version, commit, image tag, and date values.
- **SC-006**: The production service remains available throughout the rollout except for the single planned container recreation window.

## Assumptions

- The maintainer has SSH access to the production host and permission to change the production compose file and recreate the application container.
- The production database and data directory are persistent and are not deleted during a normal image upgrade.
- The official upstream release is published in `ranxi2001/sub2api` and is reachable from the fork's Git remote.
- A canary can use a separate database and port on the same host, and automatic inspection can be disabled there.
- The existing administrator account and a valid administrator session can be used for verification.
- Model Guard remains a fork-only feature and is not expected to be present in the upstream image.
- The operator accepts that canary and production verification may consume a small amount of upstream quota only when explicitly triggering tests; scheduled inspection stays disabled on the canary.
