# Implementation Plan: Codex Eval v0.2.21 兼容版

**Branch**: `001-codex-eval-v021-compat` | **Date**: 2026-11-13 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/001-codex-eval-v021-compat/spec.md`

## Summary

在本仓库内维护 `codex-eval/` 全功能 fork：以 `codex-candy-eval` v0.4.6 的 7 个补丁为基础，修复补丁换行/上下文兼容问题，使其能对 **Sub2API v0.2.21** 源码应用并构建出 Linux amd64 安装器与服务端二进制。构建与安装全部本地完成，不依赖 GitHub Actions 或云端 CI；安装器负责备份、替换、健康检查与失败回滚。

本计划覆盖 **v0.2.21 的兼容性恢复**，不承诺 v0.2.22+ 自动适配；未来版本按同一流程人工验证并更新 `compatibility` 记录。根目录 `.s2plugin`（`cmd/codex-guard`、`internal/guard`、`ui/`、`manifest.source.json`）是另一套稳定轻量交付，不在本功能改动范围内，仅保持兼容共存。

## Technical Context

**Language/Version**: Go 1.27.0（Sub2API v0.2.21 服务端与安装器）、TypeScript/Vue 3（Sub2API 管理前端）、Python 3.13（构建编排）

**Primary Dependencies**: `github.com/Wei-Shaw/sub2api` v0.2.21 源码、`codex-candy-eval` v0.4.6 补丁集、`github.com/pterm/pterm`、`golang.org/x/sys`、`golang.org/x/term`（安装器）；前端使用 Node 24 与 bundled pnpm 11.28.5

**Storage**: 文件系统构建产物、安装备份与校验和；评估业务数据仍由 Sub2API 自身数据库保存，本功能不新增数据库表

**Testing**: Go 单元测试（`go test -tags=unit -p 1 ./internal/codexeval`）、补丁应用检查（`git apply --check --binary`）、构建产物 ELF/架构检查、真实 Sub2API v0.2.21 安装与回滚验收

**Target Platform**: Linux amd64（主目标）；Linux arm64 作为可选构建目标；构建主机可为 Windows、macOS 或 Linux

**Project Type**: 源码 fork + CLI 安装器（非 Web 服务、非官方插件 API 单体）

**Performance Goals**: 常规开发机 30 分钟内完成一次构建；安装器健康检查超时 120 秒；安装失败恢复在 120 秒内完成

**Constraints**: 禁止将 GitHub Actions/云端 CI 作为交付依赖；兼容性改动必须最小、可审查、可回退；安装器不得在未备份的情况下替换 `/app/sub2api`；发布包必须可公开分发且不含私密凭据

**Scale/Scope**: 1 个 fork（`codex-eval/`，约 47 个源文件 + 7 个补丁 + 前端资源）；2 个主要构建产物（安装器、服务端二进制）；当前仅验证 Sub2API v0.2.21

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

`.specify/memory/constitution.md` 目前仍是未填写的模板，没有已批准的项目原则。因此本计划按以下派生门禁执行：

- **G1 最小差异**: 兼容性改动集中在 `codex-eval/patches/`、`scripts/build.py`、`scripts/installer/` 和文档；不重写上游业务代码。通过。
- **G2 本地可构建**: 构建入口为本地 Python 脚本与 Go/Node 工具链，不要求 GitHub Actions 或远端流水线。通过。
- **G3 可复现与可验证**: 固定上游 `v0.2.21`、记录每个产物 SHA-256、验证 ELF 架构和安装后健康状态。通过。
- **G4 安装安全**: 安装器必须在替换前备份原二进制，失败时恢复并重启；没有跳过备份的部署路径。通过。
- **G5 开源合规**: 保留 `codex-eval/LICENSE`（LGPL-3.0）与 `THIRD_PARTY_NOTICES.md`，不引入不兼容许可证的依赖。通过。
- **G6 清晰边界**: 只承诺 v0.2.21；未知/更高版本必须显式失败或记录未验证，不伪装兼容。通过。

Phase 1 后复核：上述门禁仍通过；唯一需要记录的结构性取舍是「补丁式 fork + 二进制安装器」而非官方 `.s2plugin` 插件 API，理由见 `Complexity Tracking`。

## Project Structure

### Documentation (this feature)

```text
specs/001-codex-eval-v021-compat/
├── plan.md              # This file (/speckit-plan command output)
├── research.md          # Phase 0 output (/speckit-plan command)
├── data-model.md        # Phase 1 output (/speckit-plan command)
├── quickstart.md        # Phase 1 output (/speckit-plan command)
├── contracts/           # Phase 1 output (/speckit-plan command)
│   ├── build-cli.md
│   ├── installer-cli.md
│   └── artifact-layout.md
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
codex-eval/                        # 全功能 fork（本功能主要改动范围）
├── patches/
│   ├── 0001-evaluation-core.patch
│   ├── 0002-evaluation-data.patch
│   ├── 0003-vendor-assets.patch
│   ├── 0004-admin-channel.patch
│   ├── 0005-inspection.patch
│   ├── 0006-evaluation-ui.patch
│   └── 0007-sub2api-integration.patch
├── scripts/
│   ├── build.py                   # 本地两阶段构建编排
│   └── installer/
│       ├── main.go                # 交互式 Docker/原生安装、备份、回滚
│       ├── apple.go               # Apple container 分支
│       ├── executable.go          # ELF/Mach-O/PE 检查
│       └── messages.go            # 中英文提示
├── tests/                         # 上游回归测试（复制到构建工作区运行）
├── assets/                        # 前端/安装器资源
├── licenses/                      # 第三方许可证
├── go.mod                         # fork 的独立模块边界
├── README.md
└── LICENSE                        # LGPL-3.0

docs/
└── codex-eval-fork.md             # 兼容性矩阵、构建、安装、回滚说明

dist/
└── build/linux-amd64/             # 本地产物（不入库，仅作为交付物）
    ├── sub2api-plugins-installer-v0.2.21-linux-amd64
    ├── sub2api-plugins-v0.2.21-linux-amd64
    └── SHA256SUMS

# 根目录 `.s2plugin` 稳定版（不在本功能改动范围，仅共存）
cmd/codex-guard/
internal/guard/
ui/
manifest.source.json
tools/package/
```

**Structure Decision**: 采用「`codex-eval/` 子目录 fork + 根目录保留独立 `.s2plugin`」的双轨结构。`codex-eval/` 自带 `go.mod`，避免根模块把上游测试当成自己的测试；构建脚本只在该目录内工作，安装器通过嵌入 payload 分发 Linux 二进制。根目录 `.s2plugin` 继续使用 `internal/guard` 与 `tools/package`，两者互不引用。

## Phase Plan

- **Phase 0 — 研究与基线（已完成）**: 确认 `codex-candy-eval` v0.4.6 的 7 个补丁在 LF 规范化后可完整应用到 Sub2API v0.2.21；记录补丁清单、构建产物与已知 Windows 测试差异。
- **Phase 1 — 兼容层与构建**: 固定上游 tag，补丁统一 LF 并加入校验；完善 `scripts/build.py` 的两阶段构建、SKIP 开关、环境变量与产物哈希；确保不依赖 CI。
- **Phase 2 — 安装器与回滚**: 完善 `scripts/installer` 的容器选择、备份、stage 替换、health check 与失败恢复；补充原生安装路径。
- **Phase 3 — 回归与验收**: 运行 `codex-eval/tests` 的 Go 单元测试、补丁 `git apply --check`、产物 ELF 架构检查；在真实 Sub2API v0.2.21 测试实例执行安装、页面验证、糖果/指纹/鹈鹕/SIWC 入口检查与回滚演练。
- **Phase 4 — 文档与发布准备**: 更新 `docs/codex-eval-fork.md`、根 `README.md`、`codex-eval/README.md`；记录兼容矩阵、构建前置、已知限制和哈希；不生成任何 GitHub Actions 发布依赖。

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| 使用补丁式 fork + 二进制安装器，而不是官方 `.s2plugin` 插件 | codex-candy-eval 需要后端管理接口、账户评估侧栏、多供应商与账号控制；官方插件能力白名单仅允许 `openai.oauth.outbound_transport.v1`（OpenAI OAuth），无法覆盖完整功能 | 官方插件路线只能提供糖果/指纹等 OpenAI OAuth 简化探测，无法满足用户「完整功能 + 多供应商」的核心要求；根目录 `.s2plugin` 已作为轻量稳定版保留 |
| 仅支持 v0.2.21，不跟随最新 release 自动构建 | 用户明确不要 GitHub 自动部署，且要求先让最新版本可用；自动跟随会把未验证的上游变化带入生产 | 自动跟随 latest 需要持续维护补丁与 CI，且用户明确拒绝；手工验证 v0.2.21 更符合「简单修改、可控交付」的要求 |
