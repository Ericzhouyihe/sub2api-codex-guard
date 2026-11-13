# Contract: Local Build CLI

**Feature**: `001-codex-eval-v021-compat`
**Entry point**: `python codex-eval/scripts/build.py`
**Invariant**: 不调用 GitHub Actions、云端 CI 或任何需要远程凭据的服务。

## Commands

```text
python codex-eval/scripts/build.py --help
python codex-eval/scripts/build.py
```

无子命令；通过环境变量选择阶段、平台与跳过项。

## Environment Variables

| Name | Required | Default | Contract |
|------|----------|---------|----------|
| `SUB2API_UPSTREAM` | no | `https://github.com/Wei-Shaw/sub2api.git` | 上游源码仓库 URL；允许镜像 |
| `SUB2API_BUILD_STAGE` | no | `all` | `source` 仅克隆+应用补丁；`resume` 复用工作区继续测试/构建；`all` 一次性执行 |
| `SUB2API_BUILD_WORKSPACE` | no | `codex-eval/dist/work` | 构建工作区路径，必须可写 |
| `SUB2API_BUILD_PLATFORMS` | no | 由脚本定义 | 空格分隔平台，如 `linux-amd64 linux-arm64` |
| `SUB2API_SKIP_TESTS` | no | `0` | `1` 跳过单元测试与 `go vet`；仅用于本地排障，不得用于正式发布 |
| `SUB2API_SKIP_FRONTEND` | no | `0` | `1` 复用已有前端 dist；仅在已成功构建过前端时使用 |
| `SUB2API_GOFLAGS` | no | `-mod=readonly` | 传给 Go 的 flags；Windows 低内存建议 `-mod=readonly -p=2` |
| `SUB2API_PNPM_BIN` | no | 空 | 指定 pnpm 入口（Windows 本地构建使用 `build\bin\pnpm.cmd`） |
| `SUB2API_PNPM_INSTALL_ARGS` | no | `--frozen-lockfile` | 传给 pnpm install 的参数 |
| `UPSTREAM_TAG` | no | `v0.2.21` | 固定上游版本；不允许默认解析 latest |

## Required Outputs

成功时必须存在：

```text
codex-eval/dist/build/<platform>/sub2api-plugins-installer-<tag>-<platform>
codex-eval/dist/build/<platform>/sub2api-plugins-<tag>-<platform>
codex-eval/dist/build/<platform>/SHA256SUMS
```

每个产物必须：

- 是非空普通文件；
- `kind=server` 为 ELF 格式且架构与 `<platform>` 一致；
- SHA-256 与 `SHA256SUMS` 一致；
- 文件名与 `<tag>` 一致（如 `v0.2.21`）。

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | 成功；所有要求产物已生成 |
| 1 | 参数/环境错误、补丁应用失败、测试失败、构建失败或产物校验失败 |
| 2 | 仅用于 `--help` 等参数解析场景（由 Python argparse 产生） |

失败时不得留下被标记为“成功”的半成品；已有旧产物不得被误认为本次结果（脚本应清理或写入失败标记）。

## Side Effects

- 允许在 `SUB2API_BUILD_WORKSPACE` 内克隆、打补丁、安装依赖、构建；
- 允许写入 `codex-eval/dist/build/`；
- 不得修改用户仓库中除构建产物与构建工作区之外的文件；
- 不得读取或写入任何私密凭据。
