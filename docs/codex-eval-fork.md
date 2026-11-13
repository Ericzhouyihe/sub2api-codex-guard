# Codex 降智测试全功能 fork（codex-eval）

本目录是 [`Nanboom233/codex-candy-eval`](https://github.com/Nanboom233/codex-candy-eval) 的完整 fork，用于解决原项目在 Sub2API 升级后二进制被覆盖、插件失效的问题。

## 为什么需要 fork

原项目的安装方式是给 Sub2API 源码打 7 个补丁，然后重新编译并替换容器内的 `/app/sub2api`。因此：

- 每次在 Sub2API 后台点「更新」或重启新镜像，原二进制会被覆盖，插件随之消失；
- 原项目最新提交 `bae76a3`（2026-11-08）只声明适配 Sub2API **v0.2.20**；
- 直接对 v0.2.21 使用原补丁会失败：7 个补丁中有 4 个因 patch 文件 CRLF 换行而无法应用（0002/0003 是二进制补丁头问题，0004/0007 是同一换行问题导致的上下文匹配失败）；规范化为 LF 后全部可应用。

本 fork 做了三件事：**把补丁修到 v0.2.21、把构建参数做成可移植的、用 GitHub Actions 跟随上游 release 自动构建**。

## 当前兼容性

| Sub2API 版本 | 状态 | 说明 |
| --- | --- | --- |
| v0.2.20 | 原项目声明支持 | 上游原始目标 |
| v0.2.21 | 已验证 | 7 个补丁全部 `git apply --binary` 成功；前端、后端、Linux amd64 安装器均构建成功 |
| 更新版本 | CI 自动尝试 | 上游改动导致补丁失效时 CI 失败，不产出坏包；需要更新补丁后再构建 |

已验证的本机构建产物：

- `codex-eval/dist/build/linux-amd64/sub2api-plugins-v0.2.21-linux-amd64`（132,694,176 B，SHA256 `A0062137F80D6C740D489BE603D21298EB6D09682FCD72803ED0EEEB48574714`）
- `codex-eval/dist/build/linux-amd64/sub2api-plugins-installer-v0.2.21-linux-amd64`（51,560,608 B，SHA256 `D49CAF6B601BCFD06D750371D475ADC80F4581DB65EC8CBE04522FF7B37D7591`）

## 安装与升级

1. 从本仓库 GitHub Actions 的 `Build and Release` 运行中下载对应版本的 `sub2api-plugins-installer-vX.Y.Z-linux-amd64`。
2. 上传到 Sub2API 服务器并执行：`chmod +x ... && ./...`。
3. 安装器会自动识别 Docker 容器（默认 `sub2api`）或 systemd 原生服务，备份原二进制，替换后重启并做 readiness 检查；失败时自动恢复。
4. Sub2API 升级后，重新运行与新版本匹配的安装器即可恢复插件。

> 安装器会修改容器内 `/app/sub2api`（或原生服务的可执行文件）。首次使用前建议先手动备份，并在维护窗口执行。

## 自动构建（每个上游版本）

`.github/workflows/codex-eval-build.yml`（根目录版，适用于本仓库整体开源）与 `codex-eval/.github/workflows/build.yml`（供把 `codex-eval/` 单独作为仓库时使用）：

- 每天 02:00 UTC（北京时间 10:00）读取 `Wei-Shaw/sub2api` 最新 release；
- 手动 `workflow_dispatch` 可指定 `upstream_tag` 和 `force`；
- 先执行 `source` 阶段：浅克隆上游 tag、按顺序应用全部补丁；
- 再执行 `resume` 阶段：安装 frontend 依赖、构建前端、跑全部 Go 测试、构建 linux-amd64/linux-arm64 服务端和安装器；
- 通过后上传 artifacts 并创建 GitHub Release，release tag 与上游版本号一致。

如果上游改动导致补丁无法应用，`source` 阶段会直接失败，不会发布半成品。这时需要拉取新上游、更新 `codex-eval/patches/`，再触发构建。这就是「每个版本自动尝试适配」的边界：补丁类插件无法在所有上游大改下零人工维护。

## 本地构建

```powershell
# 完整构建（Linux amd64）
$env:UPSTREAM_TAG = "v0.2.21"
$env:SUB2API_BUILD_PLATFORMS = "linux-amd64"
python scripts/build.py
```

Windows 无 corepack 或内存不足时，参见 `codex-eval/README.md` 的「本机 Windows 构建」两阶段命令；关键环境变量：

- `SUB2API_PNPM_BIN`：直接指向 `pnpm.mjs`，绕过 corepack；
- `SUB2API_BUILD_WORKSPACE`：固定构建工作区，避免 Windows 临时目录清理失败；
- `SUB2API_SKIP_TESTS=1`：本机跳过 Go 测试（CI 必须完整运行）；
- `SUB2API_SKIP_FRONTEND=1`：已构建过前端时可跳过；
- `SUB2API_GOFLAGS="-mod=readonly -p=2"`：限制 Go 并行编译，避免小内存机器 OOM。

## 与原项目的关系

- 保留原项目的 MIT 许可证与全部版权声明；
- 补丁逻辑、UI、测试来自原项目；
- 本 fork 的差异集中在 `patches/*.patch` 换行规范、`scripts/build.py` 的可移植参数、`.github/workflows/build.yml` 的自动构建，以及 `README.md` 的说明。
- 原项目仍然可作为上游来源；如果上游发布了新的 Sub2API 适配，可直接对比/合并补丁。
