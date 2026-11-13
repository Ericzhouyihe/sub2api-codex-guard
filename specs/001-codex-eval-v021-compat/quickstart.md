# Quickstart: Codex Eval v0.2.21 兼容版

**Feature**: `001-codex-eval-v021-compat`
**Purpose**: 在本地构建、验证并安装兼容 Sub2API v0.2.21 的 codex-candy-eval。

## Prerequisites

- Windows/Linux/macOS 构建机，能访问 GitHub 与 Go/Python/Node 依赖源。
- Go 1.27.x、Python 3.13、Node 24；pnpm 11.28.5（本项目提供 `build\bin\pnpm.cmd`）。
- 目标服务器：Linux amd64 + Docker/Compose（或原生 systemd 部署），Sub2API 版本为 **v0.2.21**。
- 目标服务器有可用的备份空间；安装前建议先做 Sub2API 数据备份。

> 不要求 GitHub Actions、云端 CI 或任何远程部署服务。

## 1. 构建（Windows 两阶段示例）

```powershell
cd D:\project\sub2api-codex-guard
.\codex-eval\scripts\build.ps1 -Stage source
```

source 阶段成功后，复用同一工作区继续：

```powershell
.\codex-eval\scripts\build.ps1 -Stage resume -SkipTests
```

`build.ps1` 会自动设置 `UPSTREAM_TAG=v0.2.21`、`SUB2API_BUILD_WORKSPACE=build\buildpy-work`、`GOMAXPROCS=2`、`SUB2API_GOFLAGS=-mod=readonly -p=2` 和 `NODE_OPTIONS=--max-old-space-size=8192`。

如果前端已经成功构建过，可加 `$env:SUB2API_SKIP_FRONTEND = "1"` 缩短时间；正式发布不要设置 `SUB2API_SKIP_TESTS=1`。

## 2. 验证产物

```powershell
Get-ChildItem .\codex-eval\dist\build\linux-amd64
Get-Content .\codex-eval\dist\build\linux-amd64\SHA256SUMS
```

预期文件：

- `sub2api-plugins-installer-v0.2.21-linux-amd64`
- `sub2api-plugins-v0.2.21-linux-amd64`
- `SHA256SUMS`

每个文件都必须是非空普通文件；服务端二进制应被识别为 Linux amd64 ELF。

## 3. 安装到测试服务器

1. 将安装器复制到 Linux 服务器并赋予执行权限：
   ```bash
   chmod 755 sub2api-plugins-installer-v0.2.21-linux-amd64
   ./sub2api-plugins-installer-v0.2.21-linux-amd64
   ```
2. 选择 `[1] Docker / Compose`，确认目标容器为运行中的 `sub2api`。
3. 核对安装器显示的备份路径与目标路径，输入确认。
4. 等待健康检查通过，看到 `Installed. The application is ready.`。

## 4. 验收检查

- 打开 Sub2API 管理后台，确认左侧「账户评估」页面存在且可打开。
- 选择一个可用的 OpenAI OAuth 账号，执行一次糖果测试，确认返回结果（正确/错误/失败均算流程正常）。
- 检查 `指纹`、`鹈鹕`、`SIWC` 入口是否存在；若某项受宿主能力限制，按 `docs/codex-eval-fork.md` 的说明记录。
- 观察真实 Codex API 请求是否正常（模型、响应未被改写）。
- 检查安装器输出的备份路径存在且非空。

## 5. 回滚演练

1. 记录安装器打印的备份路径。
2. 在测试环境故意停止/破坏安装后的容器，确认安装器能恢复并重启。
3. 或按文档手动使用备份路径恢复 `/app/sub2api`，重启容器并确认健康检查通过。

## Expected Outcomes

- 构建成功：`SHA256SUMS` 中 installer 与 server 两个产物均存在且校验通过。
- 安装成功：页面可打开，糖果测试可执行，真实请求不受影响。
- 安装失败：自动回滚后服务恢复可用，备份路径与错误信息可查。
- 所有结果记录到 `docs/codex-eval-fork.md` 的兼容性表格。

## Troubleshooting

- **补丁应用失败**：确认 `codex-eval/patches/*.patch` 为 LF 换行；运行 `git apply --check --binary` 定位失败文件。
- **前端构建 OOM**：设置 `NODE_OPTIONS=--max-old-space-size=8192`。
- **Go 测试/构建 OOM**：设置 `GOMAXPROCS=2` 与 `SUB2API_GOFLAGS="-mod=readonly -p=2"`。
- **Windows 临时目录清理失败**：使用两阶段 `source` + `resume`，不要使用一次性 `all`。
- **安装器找不到容器**：确认容器正在运行且 Docker 命令对当前用户可用；原生部署选择 `[2] Native executable`。
