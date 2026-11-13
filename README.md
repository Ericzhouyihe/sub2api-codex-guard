# Codex 降智测试（糖果 / 指纹 / 鹈鹕）· Sub2API

本仓库包含两套交付：

1. **根目录：官方 `.s2plugin` 插件（稳定轻量版）**——可上传到 Sub2API「插件管理」，更新 Sub2API 后不会消失；受官方能力白名单限制，只能检测 **OpenAI OAuth** 账号。
2. **`codex-eval/`：`Nanboom233/codex-candy-eval` 的完整 fork（全功能版）**——保留多供应商（OpenAI / Anthropic / Gemini / Antigravity）、账户评估侧栏、自动巡检、模型控制、优先级和通知；通过安装器替换容器内 `/app/sub2api` 二进制，并带有按上游 release 自动构建的 GitHub Actions。它已适配到 **Sub2API v0.2.21**，后续版本由 CI 自动尝试构建。

已构建的 v0.2.21 Linux amd64 安装器：

- `codex-eval/dist/build/linux-amd64/sub2api-plugins-installer-v0.2.21-linux-amd64`（51,560,608 B，SHA256 `D49CAF6B601BCFD06D750371D475ADC80F4581DB65EC8CBE04522FF7B37D7591`）
- `codex-eval/dist/build/linux-amd64/sub2api-plugins-v0.2.21-linux-amd64`（服务端二进制，由安装器嵌入）

完整版构建、安装、自动适配说明见 [`codex-eval/README.md`](codex-eval/README.md) 与 [`docs/codex-eval-fork.md`](docs/codex-eval-fork.md)。下面是根目录 `.s2plugin` 的说明。

---

一个可上传到 Sub2API「插件管理」的官方 `.s2plugin` 插件：定期用经典糖果题、数字指纹和鹈鹕画图检测 OpenAI OAuth 账号的 Codex 是否降智，同时**原样透传所有真实 Codex 流量**，安装后不会改变转发行为。

## 功能

- **糖果检测（核心）**：向每个账号发送经典糖果题（正确答案 21），按账号、模型、次数统计正确 / 错误 / 失败 / 跳过。
- **指纹检测**：随机抽取 3 道数字挑战，用 lm-detector 参考库比对答案序列，给出最可能的模型与概率（移植自 MIT 许可的 Ikaleio/lm-detector）。
- **鹈鹕检测**：要求模型画「鹈鹕骑自行车」，支持原版 HTML 动画与多合一 SVG 两个版本，可直接在插件页预览生成结果。
- **SIWC 可用性探测**：Sub2API 没有 CPA 的 SIWC 路由/元数据概念，本插件用一条最小 Responses 请求探测账号通道是否可用（available / auth_error / limited / failed）。
- **定期执行**：可配置间隔（5 分钟 – 7 天），勾选要执行的检测项目，插件进程内自动调度。
- **手动检测**：配置页选择项目后点击「立即检测」，异步执行，结果自动刷新。
- **账号范围**：默认检测全部 OpenAI OAuth 账号，也可指定账号 ID；可只测宿主标记为可调度的账号。
- **结果面板**：按检测项目分标签页展示汇总、逐条答案、耗时、错误原因与运行日志；结果通过宿主 KV 持久化，插件重启后仍可查看。
- **零侵入转发**：所有 `openai.oauth.outbound_transport.v1` 流量原样透传，仅观察，不修改请求或响应。

> 说明：Sub2API 当前插件能力白名单只允许 `openai.oauth.outbound_transport.v1`，因此检测范围仅限 **OpenAI OAuth** 账号；Anthropic / Gemini 等账号不在该插件能力范围内。

## 目录结构

```
cmd/codex-guard/          插件进程入口（gRPC / go-plugin）
internal/guard/           传输实现、糖果/指纹/鹈鹕/SIWC 检测引擎、调度器、宿主服务
internal/config/          配置解析与校验
internal/pluginapi/       宿主插件协议（LGPL-3.0，复制自 Wei-Shaw/sub2api）
ui/                       插件管理页内嵌配置与状态面板
tools/package/            密钥生成、交叉编译、签名与打包
manifest.source.json      插件清单模板
```

## 构建

要求 Go 1.27+。UI 为原生 HTML/CSS/JS，不需要前端构建。

```powershell
$env:GOPROXY = "https://goproxy.cn,direct"   # 按网络环境调整
go build ./...
go test ./...
```

端到端自检（启动真实插件进程 + 假宿主 HostService + 本地转发上游）：

```powershell
go build -o .\build\codex-guard.exe .\cmd\codex-guard
go run .\tools\smoketest -binary .\build\codex-guard.exe
```

输出 `smoketest passed` 表示 go-plugin 握手、反向宿主服务、检测任务调度、KV 持久化和 Forward 透传均正常。

## 生成发布者密钥并打包

私钥必须放在源码树之外，且不要提交到仓库。

```powershell
go run ./tools/package keygen -out D:\keys\codex-guard-publisher
go run ./tools/package build `
  -signing-key D:\keys\codex-guard-publisher.private `
  -key-id codex-guard-v1 `
  -output dist
```

产物：

- `dist/codex-guard_sub2api_v0.2.0.s2plugin` — 可直接上传
- `dist/trusted-publisher.yaml` — 宿主要信任的公钥
- `dist/SHA256SUMS`

本地调试也可以用 `-unsigned` 生成未签名包，但宿主必须开启 `allow_unsigned: true`。

## 安装到 Sub2API

1. 把 `trusted-publisher.yaml` 中的 `trusted_publishers` 条目合并到 Sub2API 宿主的 `config.yaml` 的 `plugins` 节点下，重启应用服务。
2. 打开「插件管理」→ 选择本机的 `.s2plugin` 文件安装。
3. 按宿主提示创建/启用该插件的 OpenAI OAuth 绑定。**首次启用建议先把 rollout 比例设低（例如 1–10%）**，确认 Codex 流量正常后再逐步调高；插件只做透传，不改变模型、参数或响应。
4. 在插件配置页设置模型、每个账号答题次数、并发和定时间隔，点击「立即检测」验证。

> 检测任务在插件进程内异步执行，因此插件需要处于「已启用/运行中」状态；停用插件会同时停止定时检测。Sub2API 的 `config.test` 调用有 30 秒宿主超时，插件只在该调用中启动任务并立即返回，实际答题进度通过 `plugin.status` 自动刷新。

## 配置项

| 字段 | 默认 | 说明 |
| --- | --- | --- |
| `models` | `["gpt-6.1-sol"]` | 要检测的模型，1–10 个（建议与账号实际使用的模型一致） |
| `runs` | `3` | 每个账号每个模型的答题次数，1–10 |
| `concurrency` | `4` | 同时检测的账号数，1–16（同账号内串行） |
| `effort` | 空 | 推理强度：`low` / `medium` / `high` 或留空 |
| `account_ids` | `[]` | 指定账号 ID；空表示全部 OpenAI OAuth 账号 |
| `schedulable_only` | `false` | 只检测宿主标记为可调度的账号 |
| `schedule_enabled` | `false` | 是否开启定期检测 |
| `schedule_minutes` | `60` | 定期检测间隔，5–10080 分钟 |
| `schedule_tests` | `["candy"]` | 定期检测执行的测试项目，多选：`candy` / `fingerprint` / `pelican` / `siwc` |
| `active_test` | `candy` | 「立即检测」执行的项目 |
| `fingerprint_concurrency` | `3` | 每个账号的指纹题并发，1–3（全局并发上限 6） |
| `pelican_variant` | `original` | 鹈鹕版本：`original`（HTML 动画）或 `all-in-one`（SVG） |

## 安全与隐私

- 账号访问令牌只通过宿主 `ResolveOutboundIdentity` 在内存中获取，用于构造检测请求，不写入日志、KV 或磁盘。
- 检测请求与真实 Codex 请求走同一套出站代理配置；插件不保存、不上报任何凭据。
- 插件 UI 运行在 `sandbox="allow-scripts"` 的 iframe 中，只通过 Sub2API UI Bridge 与宿主通信，不访问外部网络。

## 许可

本项目以 **MIT** 许可证发布，见 [LICENSE](LICENSE)。第三方组件与参考实现见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
