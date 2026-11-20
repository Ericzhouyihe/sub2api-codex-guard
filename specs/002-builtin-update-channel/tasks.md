# Tasks: 内置更新通道（2.23.1）

**Input**: `specs/002-builtin-update-channel/spec.md`、`plan.md`

**Tests**: 完整性检查、定向 Go 测试、embed 构建、CI 镜像与 Release 验证、内置更新接口验证。

## Phase 1: 准备

- [ ] T001 创建备份分支 `backup/model-guard-2.22.0-<timestamp>`
- [ ] T002 确认上游 `v2.23.1` 提交为 `00e00796080c632188cf9df7912050b144a1a44f`
- [ ] T003 提交 `.gitignore` 的本地目录忽略规则（`.zcode/`）

## Phase 2: 合并与冲突解决（P1）

- [ ] T004 合并 `v2.23.1` 到 `feat/model-guard`（`--no-ff --no-commit`）
- [ ] T005 删除上游恢复的 14 个已废弃工作流，仅保留 `ci.yml`、`docker-image.yml`、`security-scan.yml`、`upstream-sync.yml`
- [ ] T006 合并 `frontend/src/i18n/locales/en/common.ts` 与 `zh/common.ts` 的双方键
- [ ] T007 合并 `frontend/src/router/index.ts`，保留 `AdminCodexEval` 与上游新增路由
- [ ] T008 确认 `backend/cmd/server/VERSION` 为 `2.23.1`
- [ ] T009 确认 Dockerfile 保留 `NODE_OPTIONS=--max-old-space-size=4096`

## Phase 3: 验证（P1）

- [ ] T010 运行 `tools/upstream/sync-model-guard.ps1 -VerifyOnly`
- [ ] T011 运行定向 Go 测试（codexeval/service/repository/handler admin）
- [ ] T012 运行 `go build -p 1 -tags embed -trimpath ./cmd/server`
- [ ] T013 检查冲突标记、空白错误和意外删除的 Model Guard 文件

## Phase 4: 文档（P2）

- [ ] T014 更新 `docs/UPGRADING.md`：新增内置更新通道、无自动上传、无本地回退数据说明
- [ ] T015 新建 `docs/upgrades/2.23.1.md` 记录门禁与发布证据
- [ ] T016 确认规格/文档不含服务器地址、域名、凭据或 JWT

## Phase 5: 发布（P1）

- [ ] T017 仅提交升级相关文件；排除 `.local-run/`、`data/`、构建产物、密钥和未跟踪原型
- [ ] T018 推送 `feat/model-guard`
- [ ] T019 等待 `CI` 与 `Docker Image` 工作流完成
- [ ] T020 验证 GitHub Release `v2.23.1` 与 GHCR `2.23.1`/`latest`
- [ ] T021 验证内置更新检查返回 `has_update=true`、`deployment=docker`、`update_supported=true`
- [ ] T022 在文档中记录「服务器侧点击更新」为唯一生产更新动作

## Phase 6: 收尾（P3）

- [ ] T023 确认未跟踪原型目录（`cmd/`、`internal/`、`ui/`、`codex-eval/`、`tools/package`、`tools/smoketest`）未被提交
- [ ] T024 确认 Docker 回退只依赖旧镜像 tag，无本地数据快照
- [ ] T025 更新 `spec.md` 状态为 Implemented 并记录最终提交/镜像摘要

## Dependencies & Execution Order

- T001-T003 先于合并。
- T004-T009 完成后才能运行验证。
- T010-T013 全部通过后才能提交/推送。
- T017 之前必须完成 T014-T016。
- T018 之后 CI 才会创建 Release；T020-T021 依赖 CI 成功。
- T023-T025 在发布验证后执行。

## Implementation Strategy

1. 备份、合并、解决冲突。
2. 完整性与本地构建门禁通过后再提交。
3. 推送源码，由 CI 自动发布；不在开发机执行任何服务器命令。
4. 用后台内置更新入口完成服务器更新，回退只使用已发布 tag。
