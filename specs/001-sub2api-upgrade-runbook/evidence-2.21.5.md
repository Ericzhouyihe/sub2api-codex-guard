# Upgrade Evidence: 2.21.4 → 2.21.5

**Date**: 2026-11-16 (Asia/Shanghai)
**Target**: Sub2API `2.21.5` with Model Guard preserved
**Branch**: `feat/model-guard`
**Merge commit**: `fac0046b4` (`merge: upgrade to Sub2API 2.21.5`)
**Image source commit**: `686e754c27565787ea17f7ebdca847c9dd7d63f3` (merge + runbook docs)
**Image**: `ghcr.io/ranxi2001/sub2api:2.21.5-modelguard`
**Image ID**: `sha256:6ebbf6aa74431d44b4757b60b7e1ada33906fd50c681614d0d78215448870d8e`
**Image size**: 255,821,760 bytes
**Built**: 2026-11-15T20:39:20Z

## Baseline (before upgrade)

| Item | Value |
|---|---|
| Production image | `ghcr.io/ranxi2001/sub2api:2.21.4-modelguard` |
| Version | `Sub2API 2.21.4 (commit: 5e045118b..., built: 2026-11-15T15:03:09Z)` |
| Health | `/health` 200, container healthy |
| Users | 3 |
| Accounts | 53 |
| Settings rows | 316 |
| Model Guard candy accounts | 11 |
| Model Guard candy records | 196 |
| Model Guard fingerprint records | 0 |
| Inspection | enabled=true, group_id=2, model=gpt-6.1-sol, effort=low, type=candy, 15–35 min |

Backup: `/root/backups/model-guard-2.21.5-20261115-202743`
Compose backup: `/root/sub2api-deploy/docker-compose.yml.bak-2.21.5-20261115-204657`

## Gate Results

| Gate | Result | Evidence |
|---|---|---|
| Merge without conflicts | PASS | `git merge --no-ff --no-commit v2.21.5`; `git diff --name-only --diff-filter=U` empty |
| Model Guard registration preserved | PASS | routes/admin.go, AppSidebar.vue, router/index.ts unchanged |
| Targeted Go tests | PASS | `go test -p 1 -tags unit ...` all packages ok |
| Local embed build | PASS | `.local-run/model-guard-2.21.5.exe` reports `Sub2API 2.21.5` |
| Server image build | PASS | `Sub2API 2.21.5 (commit: 686e754c..., built: 2026-11-15T20:39:20Z)` |
| Canary API routes | PASS | state 401, settings 401, `/codex-eval` 200, `/admin/codex-eval` 200 |
| Canary UI | PASS | 4 tabs; 7 GPT accounts; 154 visible history rows; screenshots below |
| Production health | PASS | `/health` 200; container `Up (healthy)` |
| Production version | PASS | `/api/v1/admin/system/version` = 2.21.5 |
| Model Guard production API | PASS | state code 0, candy_keys=11, settings enabled=true/group 2/gpt-6.1-sol/low |
| Public page | PASS | `https://oxapi.dpdns.org/admin/codex-eval` 200, title `模型哨兵 - OXAPI` |
| Rollback path | PASS | old image and compose backup both exist |

## Data Counts After Upgrade

| Item | Before | After | Note |
|---|---:|---:|---|
| Users | 3 | 3 | unchanged |
| Accounts | 53 | 53 | unchanged |
| Settings | 316 | 316 | unchanged |
| Candy accounts | 11 | 11 | unchanged |
| Candy records | 196 | 197 | one scheduled production test at 04:35, before promotion |
| Fingerprint records | 0 | 0 | unchanged |

## Screenshots

- Canary: `.local-run/shots-canary-215/canary-01-candy.png`, `canary-02-fingerprint.png`, `canary-03-history.png`, `canary-04-settings.png`
- Production domain: `.local-run/shots-domain-215/domain-01-candy.png`, `domain-02-history.png`, `domain-03-settings.png`

## Rollback

1. Restore compose image tag to `ghcr.io/ranxi2001/sub2api:2.21.4-modelguard` (or restore the compose backup).
2. `docker compose up -d --pull never sub2api`.
3. Verify `/health` and `/api/v1/admin/system/version`.
4. Only if data is confirmed corrupt, restore `data.tar.gz` and `db-sub2api.dump` from the backup directory; this loses writes after the backup point.

## Notes

- Canary used `RUNTIME_ROLE=full` because `gateway` skips admin/Model Guard routes.
- Canary inspection was disabled to avoid duplicate upstream consumption; production inspection remained enabled and unchanged.
- Canary resources were removed after validation.
