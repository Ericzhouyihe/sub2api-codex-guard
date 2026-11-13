# Phase 1 Data Model: Codex Eval v0.2.21 兼容版

**Feature**: `001-codex-eval-v021-compat`
**Date**: 2026-11-13

本功能不新增数据库表；下列实体用于构建产物、兼容性记录与安装审计。评估业务结果仍由 Sub2API/codex-candy-eval 原有存储负责。

## Entity: CompatibilityRecord

记录某一组「上游 Sub2API 版本 + fork 版本 + 补丁集」的验证状态。

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| id | string | yes | 唯一标识，如 `sub2api-v0.2.21__codex-eval-v0.4.6` |
| upstream_version | string | yes | 如 `v0.2.21` |
| fork_version | string | yes | codex-candy-eval 基线版本，如 `v0.4.6` |
| patchset_revision | string | yes | 补丁集内容哈希（对 7 个 patch 文件按名称排序后计算 SHA-256） |
| status | enum | yes | `planned` / `verified` / `failed` / `deprecated` |
| verified_at | datetime | no | 实际验证完成时间 |
| verified_by | string | no | 验证人或自动化记录 |
| artifacts | list<ArtifactRef> | no | 关联构建产物 |
| notes | string | no | 已知限制、平台差异 |

**Validation rules**:
- `upstream_version` 必须符合 `^v?\d+\.\d+\.\d+`。
- `status=verified` 时必须有 `verified_at` 和至少一个成功产物。
- 同一 `upstream_version + fork_version + patchset_revision` 只能有一条 `verified` 记录。

**State transitions**:
`planned → verified`（构建 + 真实安装通过）或 `planned → failed`（补丁/构建/安装失败）；已 `verified` 可因上游变化标记 `deprecated`，不得静默覆盖。

## Entity: PatchEntry

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| id | string | yes | `0001`–`0007` |
| filename | string | yes | 如 `0001-evaluation-core.patch` |
| order | int | yes | 应用顺序，必须 1..7 连续 |
| sha256 | string | yes | 规范化（LF）后的文件哈希 |
| target_area | enum | yes | `backend-core` / `backend-admin` / `frontend-ui` / `assets` / `integration` |
| applies_to | list<string> | yes | 已验证的 Sub2API 版本，如 `["v0.2.21"]` |
| eol | enum | yes | 固定 `LF` |
| required | bool | yes | 本功能全部为 true |
| status | enum | yes | `pending` / `applies` / `conflict` |

**Validation rules**:
- `order` 必须唯一且连续；`filename` 必须与磁盘文件一致；`eol` 必须为 LF。
- `status=applies` 必须来自 `git apply --check --binary` 的成功结果。

## Entity: BuildRun

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| run_id | string | yes | `YYYYMMDD-HHMMSS` |
| stage | enum | yes | `source` / `resume` |
| upstream_tag | string | yes | 如 `v0.2.21` |
| platforms | list<string> | yes | 如 `["linux-amd64"]` |
| host_os | string | yes | 构建机 OS |
| host_arch | string | yes | 构建机架构 |
| go_version | string | yes | 如 `go1.27.4` |
| node_version | string | yes | 如 `v24.21.0` |
| pnpm_version | string | yes | 如 `11.28.5` |
| python_version | string | yes | 如 `3.13.x` |
| status | enum | yes | `running` / `succeeded` / `failed` |
| started_at | datetime | yes | |
| finished_at | datetime | no | |
| log_path | string | no | 构建日志路径 |

## Entity: BuildArtifact

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| artifact_id | string | yes | 唯一标识 |
| build_run_id | string | yes | 关联 BuildRun |
| kind | enum | yes | `installer` / `server` |
| platform | string | yes | `linux-amd64` / `linux-arm64` |
| filename | string | yes | 如 `sub2api-plugins-installer-v0.2.21-linux-amd64` |
| size_bytes | int | yes | 文件大小 |
| sha256 | string | yes | 小写 64 位十六进制 |
| upstream_version | string | yes | 构建使用的上游版本 |
| created_at | datetime | yes | |

**Validation rules**:
- 每个 BuildRun 至少要产出 installer + server 各一个，且 `sha256` 与 `SHA256SUMS` 一致。
- `kind=server` 必须通过 ELF 架构检查（`debug/elf` 或 `file` 等价检查）。

## Entity: InstallationRecord

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| installation_id | string | yes | 唯一标识 |
| host | string | yes | 目标服务器/容器运行时主机 |
| container_name | string | no | Docker/Apple container 名称；原生安装为空 |
| deployment_mode | enum | yes | `docker` / `native` / `apple-container` |
| binary_path | string | yes | 默认 `/app/sub2api` 或原生路径 |
| previous_version | string | no | 安装前版本探测结果 |
| installed_version | string | yes | 安装后版本 |
| backup_path | string | yes | 备份路径 |
| backup_sha256 | string | yes | 备份哈希 |
| status | enum | yes | `prepared` / `installed` / `verified` / `rolled_back` / `failed` |
| health_check | enum | yes | `passed` / `failed` / `not_run` |
| started_at | datetime | yes | |
| finished_at | datetime | no | |

**Validation rules**:
- 任何替换前必须存在 `backup_path` 与 `backup_sha256`。
- `status=verified` 必须 `health_check=passed`。
- `status=failed` 时必须记录是否已 `rolled_back`。

**State transitions**:
`prepared → installed → verified`；任一步失败 → `rolled_back`（备份恢复成功）或 `failed`（恢复失败，需人工介入）。

## Entity: EvaluationRun (existing business data, unchanged)

| Field | Type | Description |
|-------|------|-------------|
| id | string | 运行标识 |
| account_id | int64 | Sub2API 账号 ID |
| test_type | enum | `candy` / `fingerprint` / `pelican` / `siwc` |
| model | string | 本次使用的模型 |
| status | enum | `correct` / `wrong` / `failed` / `partial` / `skipped` |
| result_json | object | 原项目定义的详细结果（答案、概率、HTML 等） |
| created_at | datetime | 运行时间 |

本兼容功能不得改变该实体的业务语义；安装/升级时应尽量保留历史记录，具体迁移由 Sub2API 数据备份负责。

## Relationships

- `CompatibilityRecord` 1 — N `BuildArtifact`
- `BuildRun` 1 — N `BuildArtifact`
- `BuildArtifact` 0..1 — N `InstallationRecord`（同一产物可安装到多台服务器）
- `CompatibilityRecord` 1 — N `PatchEntry`
- `EvaluationRun` 与上述构建/安装实体无外键关系；由 Sub2API 自身数据库管理。
