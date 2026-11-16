# Phase 0 Research: Codex Eval v0.2.21 兼容版

**Feature**: `001-codex-eval-v021-compat`
**Date**: 2026-11-13
**Scope**: 在 Sub2API v0.2.21 上恢复 `codex-candy-eval` 完整功能，且不依赖 GitHub Actions/云端 CI。

## Research Questions

1. 为什么不用官方 `.s2plugin` 实现完整 codex-candy-eval？
2. 为什么当前补丁对 v0.2.21 失败？最小修复是什么？
3. 如何在不依赖云端 CI 的前提下构建可安装产物？
4. 安装器必须满足哪些安全与回滚条件？
5. 需要保留哪些测试与文档证据？

## Findings & Decisions

### 1. 集成方式：补丁式 fork，而非官方插件 API

- **Decision**: 保留 `codex-eval/` 的补丁式 fork，继续通过 Sub2API 源码集成（后端 `backend/internal/codexeval`、admin handler/service、前端 `CodexEvalView.vue`），并使用安装器替换容器内 `/app/sub2api`。
- **Rationale**: 官方 `.s2plugin` 能力白名单硬编码为 `openai.oauth.outbound_transport.v1`（`backend/internal/service/plugin_manifest.go`），只服务 OpenAI OAuth 账号，无法实现 codex-candy-eval 的多供应商、账户评估侧栏、账号启用/禁用、模型控制、优先级、通知等完整功能。根目录的 `.s2plugin` 已作为轻量版保留，但不承担本功能。
- **Alternatives considered**:
  - 纯官方插件：功能被能力白名单截断，不可行。
  - 重写为独立 Web 服务：需要复制 Sub2API 账号体系与鉴权，改动远大于原项目。
  - 仅保留根 `.s2plugin`：只能提供 OpenAI OAuth 的糖果/指纹类探测，不满足「完整功能」。

### 2. v0.2.21 补丁失败的根因：CRLF，而非上游代码不兼容

- **Decision**: 将 `codex-eval/patches/*.patch` 全部规范化为 LF，并在 `scripts/build.py` 的 `apply_patches` 中对含 CRLF 的补丁写临时 LF 副本后再 `git apply --binary`。
- **Rationale**: 对本地 v0.2.21 源码工作区执行 `git apply --check --binary` 时，0002/0003 报 `git diff header lacks filename information when removing 1 leading pathname component`，0004/0007 报多处 context 不匹配；统一换行后 7 个补丁全部成功。说明是补丁文件换行损坏，不是 v0.2.21 业务代码结构变化。
- **Alternatives considered**:
  - 手工重写全部补丁：不必要，且会破坏与上游 patch 集的可追溯性。
  - 逐个手工修改 context 行：维护成本高，换行修复已证明足够。
  - 只改构建脚本而不入库规范化补丁：仓库分发的补丁仍会伤害其他贡献者，故直接规范化入库。

### 3. 本地构建而非云端 CI

- **Decision**: 使用 `codex-eval/scripts/build.py` 作为唯一构建入口，支持两阶段 `SUB2API_BUILD_STAGE=source|resume`；固定上游版本 `v0.2.21`；产出安装器与服务端二进制；哈希写入 `SHA256SUMS`。不使用 GitHub Actions 作为交付依赖。
- **Rationale**: 用户明确「不要 GitHub 自动部署的」，并希望开源项目可自行构建；本地脚本可审计、可离线缓存、无需仓库密钥。固定 tag 保证可复现，避免 latest 漂移。
- **Alternatives considered**:
  - GitHub Actions 自动构建/发布：用户明确拒绝；且会增加补丁失效时的静默风险。
  - 直接提交预编译二进制：不可复现、不可审计，违反开源交付原则。
  - 每次构建拉 latest：不可复现，且用户只要求 v0.2.21。

### 4. 安装器安全模型

- **Decision**: 安装器必须在替换前 `docker cp` 备份原 `/app/sub2api`，再 stage 到 `/app/sub2api.plugins-new`，`chown $(stat -c '%u:%g' /app/sub2api)`、`chmod 755`、`mv -f`；替换后重启并等待健康检查；任何失败自动恢复备份并重启。原生安装路径同样先备份。
- **Rationale**: 用户最初的问题就是升级后插件消失；安装器必须可重复、可回滚，且不能因错误的架构/版本把服务弄坏。120 秒健康检查与备份恢复可覆盖容器重启、OOM、版本不匹配等场景。
- **Alternatives considered**:
  - 直接覆盖二进制：一旦新版本不可用，服务无法启动且无恢复路径。
  - bind-mount 宿主文件：依赖 Compose 配置，跨部署方式不稳定；安装器先提供通用备份/替换方案，bind-mount 可作为后续增强。

### 5. 测试与证据

- **Decision**: 使用 codex-candy-eval 自带 Go 单元测试（复制到构建工作区，`-tags=unit -p 1`）验证核心逻辑；构建前对每个补丁做 `git apply --check --binary`；产物做 ELF/架构检查与 SHA-256；真实环境做安装、页面访问、糖果/指纹/鹈鹕/SIWC 入口与回滚演练。
- **Rationale**: 单元测试是上游已有资产，能防止补丁应用后行为回退；真实环境测试才能覆盖 Docker 替换与宿主健康检查。Windows 上 `TestCodexInspectionFingerprintModeAndWildcards`、`TestCodexEvalRuntime` 已知失败，记录为平台差异，需在 Linux 目标验证。
- **Alternatives considered**:
  - 只跑构建不跑测试：无法发现补丁应用后逻辑回退。
  - 只跑单元测试不跑真实安装：无法验证安装器与容器行为。

### 6. 版本策略

- **Decision**: 当前只承诺 **Sub2API v0.2.21**；`codex-eval` 的兼容记录只登记已验证版本。未来版本由维护者按同一流程重新验证并更新记录；不提供「自动默认适配所有版本」的承诺。
- **Rationale**: 用户要求「最新版能用」「尽量简单修改」；上游大版本变化可能影响 admin 路由、前端组件、数据库 schema，自动适配不可靠。明确边界比虚假承诺更安全。
- **Alternatives considered**:
  - 声称自动适配未来所有版本：无法验证，容易产生不可用安装包。
  - 锁定 v0.2.20 不变：不满足用户最新版要求。

## Build Environment Evidence

- Go: `go1.27.4 windows/amd64`（PATH 中）
- Node: v24.21.0；pnpm: 11.28.5（bundled；`build\bin\pnpm.cmd`）
- Python: DSH 自带 Python 3.13
- Windows 构建注意：`NODE_OPTIONS=--max-old-space-size=8192`、`GOMAXPROCS=2`、`SUB2API_GOFLAGS="-mod=readonly -p=2"`；两阶段避免 `TemporaryDirectory` 清理失败。
- 已验证产物：
  - 安装器 51,560,608 B，SHA-256 `D49CAF6B601BCFD06D750371D475ADC80F4581DB65EC8CBE04522FF7B37D7591`
  - 服务端二进制 132,694,176 B，SHA-256 `A0062137F80D6C740D489BE603D21298EB6D09682FCD72803ED0EEEB48574714`

## Open Questions

- 真实 Sub2API v0.2.21 服务器上的安装验证仍需执行（用户环境为 1Panel/Docker，当前本机无 Docker）。
- Linux 上 codex-eval 全量单元测试是否全绿需在 CI 之外的实际 Linux 构建机确认；Windows 已知 2 项平台差异不计为功能缺陷。
