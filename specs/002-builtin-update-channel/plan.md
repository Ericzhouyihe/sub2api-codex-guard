# Implementation Plan: 内置更新通道（2.23.1）

**Branch**: `feat/model-guard` | **Date**: 2026-11-20 | **Spec**: [spec.md](./spec.md)

## Summary

本次实施包含两部分：

1. 按既有升级运行手册，把上游 `ranxi2001/sub2api` 的 `v2.23.1` 合并进 `feat/model-guard`，解决工作流、i18n 和前端路由冲突，保留 Model Guard。
2. 确认并文档化「内置更新通道」：只推送源码提交，CI 自动构建镜像与 Release；服务器后台检查到新版本后由管理员点击更新，Watchtower 拉取镜像；不在服务器写本地回退数据，不接受开发机自动上传。

不修改更新链路的业务代码；本次只合并上游、验证、补文档和发布。

## Technical Context

**Language/Version**: Go 1.27.0（`backend/go.mod`）；Vue 3 + TypeScript + Vite

**Primary Dependencies**: Gin、Ent、PostgreSQL、Redis、Watchtower 1.7.1、GitHub Actions

**Storage**: PostgreSQL + `/app/data`（Model Guard 历史）；本功能不新增表、不写服务器本地备份

**Testing**:

- `tools/upstream/sync-model-guard.ps1 -VerifyOnly`
- `go test -p 1 -tags unit ./internal/codexeval ./internal/service ./internal/repository ./internal/handler/admin -run 'Test(Codex|ModelGuard|Fingerprint|HasStandalone)' -count=1`
- `go build -p 1 -tags embed -trimpath ./cmd/server`
- 推送后 CI：`.github/workflows/ci.yml`、`.github/workflows/docker-image.yml`

**Target Platform**: Linux amd64 生产容器；Windows 开发机仅做源码合并与本地构建检查

**Constraints**:

- 本地不安装/不调用服务器凭据；发布链路不含 SSH/SCP。
- 更新检查固定指向 `Ericzhouyihe/sub2api-codex-guard`。
- `WATCHTOWER_CLEANUP=true`，不保留本地旧镜像；Docker 模式不提供本地回退接口。
- 前端构建内存参数 `NODE_OPTIONS=--max-old-space-size=4096` 必须保留。
- 提交只允许源码、规格、文档和必要配置。

## Constitution Check

项目尚无正式 constitution。按本 spec 与既有升级手册执行以下门禁：

- **数据安全**: Docker 镜像更新不重建数据卷；本次不执行数据回滚。
- **Fork 保留**: Model Guard 路由、页面、嵌入资源和测试不得丢失。
- **无服务器直连**: 开发机不执行 SSH/SCP；更新只由服务器侧 Watchtower 完成。
- **无本地回退数据**: 不新增服务器备份目录、二进制备份或旧数据快照。
- **提交最小化**: 不提交构建产物、本地目录、密钥、日志和未跟踪原型。
- **证据**: 记录合并提交、镜像 tag/摘要、Release、门禁结果。

**Result**: PASS。

## Project Structure

### Documentation

```text
specs/002-builtin-update-channel/
├── spec.md
├── plan.md
└── tasks.md

docs/
├── UPGRADING.md          # 增加内置更新通道与无本地回退说明
└── upgrades/2.23.1.md    # 本次执行证据
```

### Source Code

```text
backend/cmd/server/VERSION
backend/internal/server/routes/admin.go
backend/internal/server/router.go
backend/internal/codexeval/
frontend/src/router/index.ts
frontend/src/components/layout/AppSidebar.vue
frontend/src/i18n/locales/{en,zh}/common.ts
.github/workflows/{ci,docker-image,security-scan,upstream-sync}.yml
```

**Structure Decision**: 仅合并上游与补充文档；更新能力沿用已有 `update_service.go` 与 `deploy/docker-compose.local.yml` 中的 Watchtower 配置。

## Phase Plan

### Phase 0 - 研究（完成）

- 确认本地 `HEAD` 与 `origin/feat/model-guard` 一致，均为 2.22.0。
- 确认上游 `v2.23.1` 提交为 `00e00796080c632188cf9df7912050b144a1a44f`。
- 确认冲突集中在 14 个已删除工作流、`frontend/src/i18n/locales/{en,zh}/common.ts` 和 `frontend/src/router/index.ts`。
- 确认内置更新检查的 `githubRepo` 已指向本 fork。
- 确认 Docker 模式更新走 Watchtower，`WATCHTOWER_CLEANUP=true`，本地回退接口不可用。

### Phase 1 - 设计（完成）

- 冲突解决规则：工作流保持 fork 精简集合；i18n 合并双方键；路由保留 `AdminCodexEval` 与上游新增路由。
- 发布方式：推送 `feat/model-guard`，由 CI 自动创建 `v2.23.1` Release 和 `:2.23.1`/`:latest` 镜像。
- 更新方式：管理员在后台点击更新，Watchtower 拉取 `:latest`；开发机不连接服务器。
- 回退方式：compose 指向 GHCR 中已发布的旧 tag；不保留本地旧镜像/数据。

### Phase 2 - 实施

- 创建备份分支并合并 `v2.23.1`。
- 解决冲突并运行完整性检查、测试、embed 构建。
- 更新 `docs/UPGRADING.md` 与 `docs/upgrades/2.23.1.md`。
- 仅提交必要文件，推送分支，等待 CI 发布。
- 验证 Release、GHCR tag 和内置更新入口；不执行服务器上传。

## Complexity Tracking

无新增抽象、依赖或服务。唯一新增的是规格/证据文档。
