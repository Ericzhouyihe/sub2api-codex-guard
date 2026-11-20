# Feature Specification: 内置更新通道（无服务器上传 / 无本地回退数据）

**Feature Branch**: `feat/model-guard`

**Created**: 2026-11-20

**Status**: Implemented (2026-11-20; `v2.23.1` published, production click pending)

**Input**: 用户描述："上游有更新了，2.23.1 了，根据 spec 里写的大概的完成的一些流程，继续更新成最新的，并同步到服务器端。最好不要设计成自动上传到服务器，会不会在执行流程里也有敏感信息。让项目自带的能拉取到更新的信息，然后点更新按钮更新。还有回退的功能是会留存旧数据到本机吗？如果是就不要搞这个，感觉数据越堆越多。使用 spec 的 skill。优化提交的内容，不需要提交的都不要提交到 GitHub。"

## 背景与目标

本 fork 已经具备「后台检查本仓库 Release → 管理员点击更新 → Watchtower 拉取新镜像」的内置更新链路。当前需要：

1. 按既有升级流程把上游 `v2.23.1` 合并进 `feat/model-guard`，保留 Model Guard。
2. 发布新版本时只推送源码提交，由 GitHub Actions 自动构建镜像、创建 Release，服务器通过内置更新入口拉取。
3. 明确禁止从开发机自动上传文件、执行远程命令或把服务器凭据放进仓库/CI。
4. 明确 Docker 更新模式不保留本地旧数据或本地回退快照；回退只依赖仓库中已发布的旧镜像 tag。

## User Scenarios & Testing *(mandatory)*

### User Story 1 - 合并并发布 2.23.1（Priority: P1）

维护者把上游 `v2.23.1` 合并到 `feat/model-guard`，保留 Model Guard 路由、页面、嵌入资源和测试，推送源码后由 CI 自动产出带版本号的镜像与 GitHub Release。

**Why this priority**: 这是本次用户请求的核心结果；没有正确发布，内置更新入口无法发现新版本。

**Independent Test**: 合并后运行完整性检查、定向 Go 测试和 embed 构建；推送后 CI 的 Docker Image 工作流成功，GHCR 出现 `2.23.1` 与 `latest` 两个 tag，GitHub Release `v2.23.1` 存在。

**Acceptance Scenarios**:

1. **Given** 当前分支为 `feat/model-guard` 且基线为 2.22.0，**When** 合并 `v2.23.1` 并解决所有冲突，**Then** `backend/cmd/server/VERSION` 为 `2.23.1`，Model Guard 注册点全部存在。
2. **Given** 合并提交尚未推送，**When** 运行定向测试与 embed 构建，**Then** 两项均通过。
3. **Given** 源码提交已推送到 `origin/feat/model-guard`，**When** CI 完成，**Then** 镜像 `ghcr.io/ericzhouyihe/sub2api-codex-guard:2.23.1` 和 `:latest` 可拉取，Release `v2.23.1` 的 tag 指向该提交。

---

### User Story 2 - 使用项目内置入口更新服务器（Priority: P1）

管理员在后台看到「有新版本」提示，点击「立即更新镜像」，由服务器上的 Watchtower 拉取并重建容器；开发机不接触服务器，也不上传二进制或镜像。

**Why this priority**: 用户明确拒绝自动上传服务器，并要求复用项目自带能力，避免执行链路中出现敏感信息。

**Independent Test**: 在后台版本浮层确认版本号来自本 fork 的 Release；点击更新后 Watchtower 日志显示拉取 `:latest` 并重建 `sub2api`；开发机没有新增 SSH/SCP 调用或服务器凭据。

**Acceptance Scenarios**:

1. **Given** 内置更新检查已配置为本 fork 仓库，**When** 发布 `v2.23.1`，**Then** 后台返回 `has_update=true` 且 `deployment=docker`、`update_supported=true`。
2. **Given** Watchtower HTTP API 已启用，**When** 管理员点击更新，**Then** 仅 Watchtower 发起镜像拉取，浏览器不需要等待完成。
3. **Given** 更新配置缺失或 token 为空，**When** 管理员查看版本浮层，**Then** 页面只显示手动命令提示，不显示可点击的更新按钮。

---

### User Story 3 - 回退不产生本地数据堆积（Priority: P2）

更新失败时，维护者通过把 compose 镜像指回已发布的旧版本 tag 完成回退；系统不在服务器上保存旧镜像、旧数据快照或回退备份目录。

**Why this priority**: 用户担心回退功能把旧数据留在本机导致数据越堆越多，因此回退必须是无本地留存的设计。

**Independent Test**: 检查 Watchtower 配置与更新服务：Docker 模式不提供本地回退接口；`WATCHTOWER_CLEANUP=true` 会清理旧镜像；升级手册只记录「切换已发布 tag」的回退方式。

**Acceptance Scenarios**:

1. **Given** Docker 部署，**When** 调用回退相关接口，**Then** 返回 `DOCKER_UPDATE_UNSUPPORTED`，不会在本地写入备份文件。
2. **Given** 新镜像已成功更新，**When** Watchtower 完成清理，**Then** 旧镜像不会作为长期回退副本保留；需要回退时从 GHCR 拉取旧 tag。
3. **Given** 更新流程文档，**When** 维护者查找回退步骤，**Then** 只能找到「改 compose image tag + 重建服务」，没有自动数据快照步骤。

---

### User Story 4 - 提交内容最小化（Priority: P3）

发布提交只包含升级所需的源码、规格和文档；构建产物、本地运行目录、数据目录、密钥、日志和未纳入产品目录的原型不进入 GitHub。

**Why this priority**: 避免仓库膨胀和敏感信息泄露，同时让每次升级的 diff 可审计。

**Independent Test**: 推送前检查 `git status --short` 与 `git show --stat`，确认没有 `dist/`、`build/`、`.local-run/`、`data/`、`.env`、私钥、日志或未跟踪原型目录。

**Acceptance Scenarios**:

1. **Given** 工作区包含 `.local-run/`、`data/`、`.zcode/` 等本地目录，**When** 执行发布提交，**Then** 这些路径不出现在提交中。
2. **Given** 工作区存在未跟踪的插件原型目录，**When** 执行本次升级提交，**Then** 原型目录保持未跟踪，不随升级提交进入仓库。
3. **Given** 发布提交，**When** 审查 diff，**Then** 只包含上游合并、Model Guard 冲突解决、规格/证据和必要配置。

---

### Edge Cases

- 上游修改了 fork 已删除的工作流文件：保留 fork 的精简工作流集合，不恢复上游文件。
- 上游修改了 Model Guard 同时触及的 i18n 或路由文件：合并双方内容，保留 `AdminCodexEval` 路由和菜单键。
- 上游改动 `backend/cmd/server/VERSION` 或 Dockerfile：版本号必须为 `2.23.1`，前端构建内存参数必须保留。
- CI 构建失败：不创建 Release、不推送 tag，服务器不会看到新版本。
- 服务器无法访问 GitHub：内置检查返回警告或缓存；不影响现有服务。
- 旧镜像 tag 不存在：回退只能选择 GHCR 中实际存在的 tag，不能依赖本地缓存。
- 未跟踪原型目录包含密钥或大文件：保持未跟踪，且不得通过 `git add -A` 提交。

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: 发布源 MUST 为本 fork 的 GitHub Releases（`Ericzhouyihe/sub2api-codex-guard`），不得指向上游仓库。
- **FR-002**: 内置更新检查 MUST 读取本 fork 的 latest Release，并返回当前版本、最新版本、是否有更新、部署模式和是否支持页内更新。
- **FR-003**: Docker 部署的页内更新 MUST 通过服务器本地 Watchtower HTTP API 触发，不得由开发机直接连接服务器执行命令。
- **FR-004**: 仓库和 CI MUST NOT 包含服务器地址、SSH 密码、JWT、数据库密码、TOTP 密钥或任何可用于远程登录的凭据。
- **FR-005**: 推送 `feat/model-guard` 后，CI MUST 构建并推送 `:2.23.1` 与 `:latest` 镜像，并创建 `v2.23.1` Release。
- **FR-006**: Docker 更新模式 MUST NOT 在服务器上创建本地数据快照、二进制备份或回退目录；旧镜像由 Watchtower 清理。
- **FR-007**: 回退说明 MUST 只使用已发布的旧镜像 tag 和 compose 修改，不提供自动数据回滚流程。
- **FR-008**: 合并 `v2.23.1` MUST 保留 Model Guard 的后端路由、前端入口、嵌入资源和定向测试。
- **FR-009**: 发布前 MUST 通过 Model Guard 完整性检查、定向 Go 测试和 `-tags embed` 服务端构建。
- **FR-010**: 发布提交 MUST 排除构建产物、本地运行目录、数据目录、密钥、日志和未纳入产品的原型目录。
- **FR-011**: 更新手册 MUST 记录 2.23.1 的执行证据，并说明「无自动上传、无本地回退数据」的边界。

### Key Entities

- **Release**: GitHub tag `v2.23.1`、目标提交、镜像 tag `2.23.1`/`latest`、Release notes。
- **Update check response**: `current_version`、`latest_version`、`has_update`、`deployment`、`update_supported`。
- **Watchtower bridge**: 服务器内部 HTTP API，仅接受带 Bearer token 的更新请求。
- **Rollback reference**: GHCR 中保留的旧版本镜像 tag，不包含本地数据副本。
- **Upgrade evidence**: 脱敏后的门禁结果、提交号、镜像摘要和验收时间。

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 合并、测试、构建和发布的全部必过门禁在 90 分钟内完成（不含 CI 队列等待）。
- **SC-002**: 升级后用户、账户、Model Guard 设置和历史记录数量与升级前一致。
- **SC-003**: 后台内置检查在发布后能识别 `2.23.1`，并在 Docker 模式下显示可用的页内更新入口。
- **SC-004**: 整个发布与更新链路中，开发机对生产服务器的主动连接次数为 0。
- **SC-005**: 发布提交中构建产物、本地目录和密钥文件数量为 0。
- **SC-006**: 回退不依赖本地旧镜像或本地数据快照，只依赖 GHCR 中已发布的 tag。

## Assumptions

- 服务器已配置 `WATCHTOWER_HTTP_API_URL`、`WATCHTOWER_HTTP_API_TOKEN` 和 `WATCHTOWER_HTTP_API_UPDATE=true`，且 Watchtower 仅更新带标签的 `sub2api` 容器。
- 服务器可以访问 GHCR 与本 fork 的 GitHub Release API。
- Docker 部署的数据卷（PostgreSQL、Redis、`data/`）在镜像更新时保持不变；本功能不负责数据库迁移回滚。
- 旧版本镜像 tag 在 GHCR 中保留足够时间，供人工回退使用。
- `feat/model-guard` 是唯一发布分支，CI 和内置更新检查都以它为准。

## Implementation Record

- Merge commit: `30dd5e363297e8fb43354f08620a6bf8509e6ab9`
- Release commit/tag: `f68a2c088e18dd6e0c248b7aa31448c5c4678cf9` / `v2.23.1`
- Image: `ghcr.io/ericzhouyihe/sub2api-codex-guard:2.23.1` / `:latest`
- Digest: `sha256:391c28a16cc07c1e27a0475b9447fdc58b0c74fb2ccd1bcc2613cd9beacc6fdc`
- CI: `CI` #16 success, `Docker Image` #18 success
- Production update: pending administrator click on “立即更新镜像” (no server upload by design)
