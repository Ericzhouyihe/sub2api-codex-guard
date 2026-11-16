# Requirements Quality Checklist: Reusable Sub2API Upgrade Runbook

**Purpose**: Review whether the upgrade-runbook specification is complete, unambiguous, testable, and safe enough to execute for the 2.21.5 release and every future release.
**Created**: 2026-11-16
**Feature**: [spec.md](../spec.md)

**Note**: This checklist is a reviewer-owned requirements-quality review artifact. Marking an item `[x]` means the criterion has been reviewed and satisfied; it does not mean the upgrade itself has been executed.

## Scope and user value

- [x] CHK001 The specification names the primary user (maintainer), the trigger (new upstream release), and the desired outcome (safe upgrade with Model Guard preserved). See User Story 1.
- [x] CHK002 The specification separates the upgrade journey from Model Guard parity, rollback, and future reuse, so each can be tested independently. See User Stories 1–4.
- [x] CHK003 The specification states that the process is reusable for releases after 2.21.5, not a one-off command transcript. See User Story 4 and FR-014.
- [x] CHK004 The specification defines the official upstream repository and requires an immutable release commit. See FR-001.

## Data safety and production protection

- [x] CHK005 The specification requires a pre-upgrade baseline of version, commit, image, health, and data counts. See FR-002.
- [x] CHK006 The specification requires a verified backup set before production changes. See FR-003.
- [x] CHK007 The specification requires an isolated canary with separate database, data directory, port, and credentials. See FR-008.
- [x] CHK008 The specification requires production promotion to preserve existing data volumes. See FR-011.
- [x] CHK009 The specification defines stop conditions and a rollback action for every stage. See FR-013.

## Model Guard parity

- [x] CHK010 The specification explicitly protects backend routes, frontend entry points, embedded assets, tests, and the fork-specific build memory setting. See FR-004.
- [x] CHK011 The specification requires all four Model Guard tabs, saved settings, and history records to survive the upgrade. See FR-010 and SC-002.
- [x] CHK012 The specification distinguishes an authentication failure (route present) from a missing route during API verification. See FR-010 and User Story 2, scenario 3.

## Verifiability and gates

- [x] CHK013 Every mandatory stage has a pass/fail condition: merge, tests, build, canary, promotion, and post-deploy checks. See FR-005 through FR-012.
- [x] CHK014 The specification requires both direct-host and public-domain verification after promotion. See FR-012.
- [x] CHK015 The success criteria are measurable, including a time target, data-preservation target, and rollback target. See SC-001 through SC-006.

## Edge cases and assumptions

- [x] CHK016 The specification covers merge conflicts, upstream dependency changes, frontend build memory exhaustion, schema/settings changes, duplicate canary jobs, access-path failures, version suffix handling, and backup mismatch. See Edge Cases.
- [x] CHK017 The specification states the required access, persistence, canary, administrator, and upstream assumptions. See Assumptions.
- [x] CHK018 No requirement depends on an unstated clarification; release-specific values are the only values a future maintainer must replace. See FR-014 and SC-005.

## Notes

- Review result: the specification is ready for planning and for the first execution against 2.21.5.
- The technical commands, host paths, ports, and image tags belong in the implementation plan and runbook, not in this requirements document.
- Items must remain unchecked if a future review finds a missing gate or an untestable requirement.
