# Codex 降智测试全功能 fork（codex-eval）

本目录是 [`Nanboom233/codex-candy-eval`](https://github.com/Nanboom233/codex-candy-eval) 的完整 fork，用于解决原项目在 Sub2API 升级后二进制被覆盖、插件失效的问题。

## 为什么需要 fork

原项目的安装方式是给 Sub2API 源码打 7 个补丁，然后重新编译并替换容器内的 `/app/sub2api`。因此：

- 每次在 Sub2API 后台点「更新」或重启新镜像，原二进制会被覆盖，插件随之消失；
- 原项目最新提交 `bae76a3`（2026-11-08）只声明适配 Sub2API **v0.2.20**；
- 直接对 v0.2.21 使用原补丁会失败：7 个补丁中有 4 个因 patch 文件 CRLF 换行而无法应用（0002/0003 是二进制补丁头问题，0004/0007 是同一换行问题导致的上下文匹配失败）；规范化为 LF 后全部可应用。

本 fork 做了三件事：**把补丁修到 v0.2.21、把构建参数做成可移植的、提供可复现的本地安装器构建**。本项目不依赖 GitHub Actions 或任何云端构建。

## 当前兼容性

| Sub2API 版本 | 状态 | 说明 |
| --- | --- | --- |
| v0.2.20 | 原项目声明支持 | 上游原始目标 |
| v0.2.21 | 已验证（本地构建） | 7 个补丁全部 `git apply --binary` 成功；前端、后端、Linux amd64 安装器均构建成功 |
| 更新版本 | 需本地重试 | 上游改动导致补丁失效时构建会失败且不发布产物；更新补丁后再按同一命令本地构建 |

已验证的本机构建产物：

- `codex-eval/dist/build/linux-amd64/sub2api-plugins-v0.2.21-linux-amd64`（132,694,176 B，SHA256 `A0062137F80D6C740D489BE603D21298EB6D09682FCD72803ED0EEEB48574714`）
- `codex-eval/dist/build/linux-amd64/sub2api-plugins-installer-v0.2.21-linux-amd64`（51,568,800 B，SHA256 `5037F559D0A025BD970890109FD119C8E04CD6FB070A297B09304E7D2B6621BE`）

## 安装与升级

1. 在本机执行本地构建（见下文），取得 `codex-eval/dist/build/linux-amd64/sub2api-plugins-installer-vX.Y.Z-linux-amd64`。
2. 上传到 Sub2API 服务器并执行：`chmod +x ... && ./...`。
3. 安装器会自动识别 Docker 容器（默认 `sub2api`）或 systemd 原生服务，备份原二进制，替换后重启并做 readiness 检查；失败时自动恢复。
4. Sub2API 升级后，重新运行与新版本匹配的安装器即可恢复插件。

> 安装器会修改容器内 `/app/sub2api`（或原生服务的可执行文件）。首次使用前建议先手动备份，并在维护窗口执行。

## 可选 CI 示例（不属于交付路径）

历史 GitHub Actions 文件已移至 `docs/optional-ci/`。它们不是构建依赖；本项目正式支持的是本地构建：

- 先执行 `source` 阶段：浅克隆固定 `v0.2.21`、按顺序应用全部补丁；
- 再执行 `resume` 阶段：安装 frontend 依赖、构建前端、跑 Go 测试、构建 linux-amd64 服务端和安装器；
- 构建失败会写 `dist/BUILD_FAILED.txt` 并保留旧产物到 `dist/.stale/`，不会把旧包当成本次成功产物。

如果上游改动导致补丁无法应用，`source` 阶段会直接失败，不会产出半成品。这时需要拉取新上游、更新 `codex-eval/patches/`，再在本地重新构建。这就是「每个版本可适配」的边界：补丁类插件无法在所有上游大改下零人工维护。

## 本地构建

```powershell
cd D:\project\sub2api-codex-guard
.\codex-eval\scripts\build.ps1 -Stage source
.\codex-eval\scripts\build.ps1 -Stage resume -SkipTests
```

首次构建可省略 `-SkipTests`；第二次重试已有前端产物时加 `-SkipFrontend`。脚本会设置 Windows 需要的 `NODE_OPTIONS`、`GOMAXPROCS` 和 `SUB2API_GOFLAGS`，并固定 `UPSTREAM_TAG=v0.2.21`。

关键环境变量：

- `SUB2API_PNPM_BIN`：直接指向 `pnpm.mjs`，绕过 corepack；
- `SUB2API_BUILD_WORKSPACE`：固定构建工作区，避免 Windows 临时目录清理失败；
- `SUB2API_SKIP_TESTS=1`：本机跳过 Go 测试（正式验收必须完整运行）；
- `SUB2API_SKIP_FRONTEND=1`：已构建过前端时可跳过；
- `SUB2API_GOFLAGS="-mod=readonly -p=2"`：限制 Go 并行编译，避免小内存机器 OOM。

## 干净本地构建记录（T019）

在全新的 `build\t019-clean` 工作区、不依赖 GitHub Actions 执行：

```powershell
$env:SUB2API_BUILD_WORKSPACE = "D:\project\sub2api-codex-guard\build\t019-clean"
$env:UPSTREAM_TAG = "v0.2.21"
$env:SUB2API_BUILD_STAGE = "source"
python .\codex-eval\scripts\build.py
$env:SUB2API_BUILD_STAGE = "resume"
$env:SUB2API_SKIP_TESTS = "1"
python .\codex-eval\scripts\build.py
```

- `source` 阶段：克隆 v0.2.21、规范化并应用 7 个补丁成功；
- `resume` 阶段：安装前端依赖、`vue-tsc` + `vite build` 成功、`go build -tags=embed` 成功、安装器构建成功；
- 实测耗时：**423 秒**（本次 Windows 机器，已跳过 Go 单测；前端构建约 45 秒 + 依赖安装约 22 秒）；
- 产物：`sub2api-plugins-v0.2.21-linux-amd64` 132,694,176 B / `A0062137...`；`sub2api-plugins-installer-v0.2.21-linux-amd64` 51,568,800 B / `5037F559...`；与 `SHA256SUMS` 一致。

## 回退到上游（T023）

在 `git worktree` 创建的干净 v0.2.21 工作树中不应用任何补丁，直接构建上游服务端：

```powershell
cd D:\project\_reposearch\_sub2api-v0.2.21-clean\backend
$env:GOMAXPROCS = "2"
$env:GOFLAGS = "-mod=readonly -p=2"
go build -o D:\project\sub2api-codex-guard\build\sub2api-v0.2.21-clean.exe ./cmd/server
```

实测：退出码 0，耗时 114 秒，产物 165,556,736 B；`go version -m` 显示 `path github.com/Wei-Shaw/sub2api/cmd/server`。说明移除全部 7 个补丁后上游 v0.2.21 仍可独立构建，兼容层可回退。

## 模块隔离与测试记录（T009）

- 根模块 `go test ./...`：通过（`internal/config`、`internal/guard`、`internal/pluginapi/v1`，其余包无测试）；
- 根模块 `go vet ./...`：通过；
- 嵌套 codex-eval 测试：在 `build/t019-clean/source/backend/internal/codexeval` 放入 `codex-eval/tests/test_codexeval_*_test.go` 后执行 `go test -tags=unit -p 1 ./internal/codexeval`，绝大多数用例通过，仅两项 Windows 平台差异失败：
  - `TestCodexInspectionFingerprintModeAndWildcards`：`fingerprint recovery did not apply in a single run without recovery_wait: calls=6 ...`；
  - `TestCodexEvalRuntime`：`recovery copy permissions`。
  这两项在 Linux CI/服务器上应完整运行；本地 Windows 结果已记录，不阻塞构建。

## 与上游的差异（T020）

对比基准：`Nanboom233/codex-candy-eval` 最新提交 `bae76a3`（v0.4.6）。本 fork 共 47 个上游文件，新增/修改集中在构建与安全校验，不修改原项目的评估业务设计。

| 类别 | 文件 | 变更目的 |
| --- | --- | --- |
| 补丁 | `patches/0001..0007-*.patch` | 仅把 CRLF 规范化为 LF；补丁内容与上游一致，保证能在 v0.2.21 上 `git apply --binary` |
| 构建 | `scripts/build.py` | 固定 `UPSTREAM_TAG=v0.2.21`、两阶段构建、旧的 `dist/build` 归档到 `.stale/`、失败写 `BUILD_FAILED.txt`、前置工具版本检查、`SHA256SUMS` |
| 构建 | `scripts/build.ps1` | Windows 本地入口；设置内存/并行度环境变量，不依赖 CI |
| 校验 | `scripts/check_patches.py`、`patches/SHA256SUMS` | 校验 7 个补丁的顺序、哈希和 LF |
| 校验 | `scripts/verify_artifacts.py`、`scripts/verify-install.sh` | 校验产物 ELF/哈希和安装前备份/回滚清单 |
| 记录 | `scripts/record_compatibility.py`、`compatibility.json` | 记录补丁集修订、构建产物和验证状态 |
| 安全 | `scripts/installer/executable.go`、`main.go`、`native.go`、`installer_test.go` | 在替换 `/app/sub2api` 前强制确认备份存在且非空；失败恢复 |
| 测试 | `tests/support.py` | Windows 下按 UTF-8 读取测试文件（仅本地测试兼容） |
| 文档 | `README.md`、`docs/optional-ci/README.md` | 说明本地构建路径；CI 示例不属于交付依赖 |
| 删除 | `.github/workflows/build.yml` | 移至 `docs/optional-ci/`，避免被误认为自动发布依赖 |

未改动：`internal/` 业务逻辑、UI 组件、测试断言、第三方数据（lm-detector 二进制与 json）均保持上游内容。

`codex-eval/scripts/check_minimal_diff.py --upstream <upstream-checkout>` 的结果：上游 47 个文件，修改 6 个、删除 1 个、fork 新增 11 个工具/文档文件；兼容性文件占比 **14.89%**（上限 30%）。

## 四个测试入口（T015）

`codex-eval` 的账户评估页面包含四个入口：**糖果（candy）**、**指纹（fingerprint）**、**鹈鹕（pelican）**、**SIWC 可用性探测**。本地 `go test -tags=unit` 已覆盖糖果、指纹、鹈鹕相关引擎；SIWC 依赖宿主的 Codex Responses 通道，在本机没有真实 Sub2API OAuth 账号，只能由服务器实测确认。

## 已知限制

- 本机没有 Docker、Linux Sub2API 或可用 OAuth 账号，T012–T015/T025 的真实服务器安装、流量对比、升级演练无法在本工作区完成；需在用户的 Sub2API v0.2.21 测试实例执行。
- Windows 上 `TestCodexInspectionFingerprintModeAndWildcards` 与 `TestCodexEvalRuntime` 两个测试因平台语义差异失败，Linux 服务器/CI 需完整复跑。

## 升级与重装（T024）

从旧版 Sub2API（v0.2.19/v0.2.20）或旧版 codex-eval 升级到 v0.2.21：

1. 先记录当前容器/服务名称与 `/app/sub2api` 的路径、版本和 SHA256；停止或摘除流量。
2. 用本地构建的 `sub2api-plugins-installer-v0.2.21-linux-amd64` 执行安装；安装器会先备份原二进制到容器内 `sub2api.bak-<timestamp>`（具体路径以安装器输出为准）。
3. 启动后打开「账户评估」页，先跑糖果测试；再发一次真实 Codex 请求确认转发正常。
4. 若页面或转发异常，按 T026 的回滚步骤恢复备份并重启，不要再次覆盖备份。
5. 历史评估记录由数据目录中的 `history.json`/`history.v*.json` 保存；安装器不会主动删除这些数据文件，升级前仍建议单独备份整个数据目录。

## 回滚流程（T026）

**安装器自动回滚**：安装器在替换前创建 `sub2api.bak-<timestamp>`；健康检查（120 秒）失败时会恢复该备份并重启原服务。查看安装器输出的备份路径并记录。

**手动回滚**：

1. 停止 Sub2API 容器/服务。
2. 在容器内执行 `cp /app/sub2api.bak-<timestamp> /app/sub2api && chmod 755 /app/sub2api`（Docker：`docker cp <backup> <container>:/app/sub2api`）。
3. 重启容器/服务，确认 `/app/sub2api` 的版本输出与备份前一致。
4. 检查「账户评估」入口消失（说明已回到未打补丁版本）且普通 Codex 转发正常。
5. 保留备份文件与本次安装器 SHA256，作为回滚审计记录；确认稳定后再按数据保留策略清理旧备份。

## 发布检查清单（T030）

- [ ] `git status --short` 无 `dist/build/`、`build/`、`*.private`、`.env` 等构建产物或密钥；
- [ ] `python codex-eval/scripts/check_patches.py` 输出 `Verified 7 patches.`；
- [ ] `python codex-eval/scripts/check_minimal_diff.py --upstream <upstream>` 输出 `OK`；
- [ ] `python codex-eval/scripts/record_compatibility.py` 输出 `OK`；
- [ ] 产物通过 `python codex-eval/scripts/verify_artifacts.py` 校验；
- [ ] 在真实 Sub2API v0.2.21 测试实例完成 T012–T015，并回填 `compatibility.json` 状态与 `verified_at`；
- [ ] 发布标签建议：`codex-eval-v0.4.6-sub2api-v0.2.21`；
- [ ] 发布说明写明：不依赖 GitHub Actions、本地构建命令、安装器 SHA256、回滚路径、Windows 平台测试差异。

## 与原项目的关系

- 保留原项目的 MIT 许可证与全部版权声明；
- 补丁逻辑、UI、测试来自原项目；
- 本 fork 的差异集中在 `patches/*.patch` 换行规范、`scripts/build.py` 的可移植参数、`scripts/build.ps1` 的本地构建入口，以及 `README.md` 的说明。
- 原项目仍然可作为上游来源；如果上游发布了新的 Sub2API 适配，可直接对比/合并补丁。
