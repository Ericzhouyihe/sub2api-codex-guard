# Codex 降智测试 → Sub2API `.s2plugin`：开源调研、技术判断与实现方案

> 调研日期：2026-11-13 · 目标宿主：Sub2API 0.2.21 / 0.2.22 · 结论：**基于现有开源项目改造（"骨架复用 + 引擎移植"），不建议直接使用，也不建议纯自研。**

---

## 0. 一句话结论

- **不能"直接用"**：目前 GitHub 上没有任何一个现成项目同时满足「官方 `.s2plugin` 格式 + 糖果/指纹/鹈鹕/SIWC 全套检测 + 可上传不被更新覆盖」。最接近的两个项目各有致命缺口（一个是**补丁式替换二进制**，一个是**只有一种检测且无开源许可证**）。
- **不必"纯自研"**：`.s2plugin` 的宿主骨架、四种检测引擎、打包签名脚本在开源里**都已有成熟实现**，从零重写是浪费。
- **推荐"改造/合并"**：拿一个 **MIT/兼容许可的 `.s2plugin` 骨架**做壳，把 **CPA 插件（MIT）里那套检测引擎**移植进去，重写一层 host 适配（CPA C-ABI → Sub2API HostService），界面迁进插件 iframe。工作量集中在"适配 + 移植"，而不是"发明"。

---

## 1. 官方 Sub2API 插件机制（硬约束，来自 `Wei-Shaw/sub2api` 源码）

> 参考：`docs/PLUGIN_DEVELOPMENT.md`、`backend/pkg/pluginapi/docs/{package-format,ui-bridge,security,development}.md`、`backend/pkg/pluginapi/v1/plugin.proto`、`manifest.schema.json`、`backend/internal/service/plugin_manifest.go`、`plugin_manager.go`、`plugin_package.go`。

### 1.1 包格式
`.s2plugin` 本质是一个 ZIP：

```
manifest.json            # 声明 name/version/requires/capabilities/ui/files(sha256)
signature.json           # Ed25519 签名，覆盖 manifest.json 原始字节（生产必须）
runtimes/<goos>-<goarch>/<binary>   # 每平台一个可执行
ui/index.html + ui/assets/*         # 插件配置页（iframe 内加载）
```

- `files` 里是小写十六进制 SHA-256；禁止绝对路径、`..` 父跳转、符号链接、未声明或缺失文件。
- 校验逻辑在 `backend/internal/service/plugin_package.go`。

### 1.2 能力白名单（**最关键的限制**）
`backend/internal/service/plugin_manifest.go` 硬编码了唯一允许的能力：

```
PluginCapabilityOpenAIOAuthOutbound = "openai.oauth.outbound_transport.v1"
```

`Validate()` 只在 **能力 id = 该值 且 platform=openai 且 account_type=oauth** 时通过，否则报 `初期仅支持能力 ...`。

> **推论：`.s2plugin` 版本只能服务 OpenAI OAuth 账号。** 这是和"补丁版 codex-candy-eval"最大的差异——补丁版能测 Anthropic / Gemini / Antigravity 的 API-Key 账号，插件版拿不到这些账号的凭据（HostService 只暴露 OpenAI OAuth），**多供应商测试在 `.s2plugin` 版本里做不到**。

### 1.3 运行协议（gRPC）
- `TransportPlugin`：`GetInfo / Health / ValidateConfig / ApplyConfig / TestConfig / Forward(双向流) / InitHostServices`
  - `Forward` 帧序：请求 `start → body_chunk* → body_end`；响应 `start → body_chunk* → end/error`。**插件必须做正确的透传**。
- `HostService`：`KVGet / KVSet / KVDelete / KVList`、`ListAccounts`、`ResolveOutboundIdentity`
  - `AccountInfo` 含 id/platform/account_type/name/status/schedulable/is_shadow/metadata_json（**不含凭据**）。
  - 唯一凭据通道：`ResolveOutboundIdentity` 返回 `{token, proxy_url, headers}`。

### 1.4 生命周期（重要风险）
`backend/internal/service/plugin_manager.go` 的 `hasEnabledOpenAIBinding`（L1140-1145）要求 binding 已启用且能力=openai/oauth 才会拉起插件运行时；`Enable()` 支持 rollout 1-100%。**启用后，插件会按 RolloutPercent 分桶接管真实的 OpenAI OAuth 流量**（`openai_plugin_transport.go` → `RoundTripOpenAIOAuth`）。

> **没有"只装不用"模式。** 插件一变即可影响线上 Codex 流量 → `Forward` 一旦有 bug 会直接断流。必须：灰度 rollout + 完整透传契约测试。

### 1.5 UI Bridge v1（v0.2.8 起）
iframe 内可用方法仅 7 个：`sub2api.plugin.ready`、`config.load`、`config.save`、`config.test`（有副作用，需二次验证）、`plugin.status`（**只读，映射插件 `Health.status_json`，免二次验证，用于轮询**）、`ui.resize`、`ui.notify`。
iframe `sandbox=allow-scripts`、无 same-origin、CSP 禁外部网络（**只限制 iframe；插件二进制本身可以发网络请求**）。

### 1.6 签名与分发
- Ed25519 覆盖 `manifest.json` 原始字节；宿主 `config.yaml` 的 `plugins.trusted_publishers` 追加公钥；`allow_unsigned: true` 仅供本地调试。
- `requires.sub2api` 是**硬兼容范围**，例如 `>=0.2.7 <0.3.0`。

### 1.7 "界面更新后不消失"
`.s2plugin` 是宿主**插件存储（数据目录）**里的包，不是替换 `sub2api` 二进制；所以更新 `sub2api` 应用镜像**不会删除插件**。只要 `requires.sub2api` 范围仍匹配就继续可用；**大版本（如 0.3.0）改了 manifest/能力/桥接协议时需重新发版**。这正是用户诉求的答案。

---

## 2. 用户现有插件：`Nanboom233/codex-candy-eval`（补丁式，非插件）

- 结构：`patches/0001..0007` + `scripts/build.py` + installer。
- 7 个补丁把代码**注入 Sub2API 后端源码**后重新编译，**替换容器内 `/app/sub2api` 二进制**：
  - `0001-evaluation-core`：`backend/internal/codexeval/{candy,codex,fingerprint,fingerprint_detector,model,pelican,runtime,siwc,state}.go`（+2394 行）
  - `0002-evaluation-data` / `0003-vendor-assets`：数据与前端资源
  - `0004-admin-channel`：`handler/admin/account_codex_eval_handler.go` + `service/account_codex_eval_service.go`（+1177 行）
  - `0005-inspection`：巡检（+1413 行）
  - `0006-evaluation-ui`：`backend/internal/codexeval/web/*`（app/candy/fingerprint/pelican/siwc/inspection…，+2843 行）+ `frontend/.../CodexEvalView.vue`
  - `0007-sub2api-integration`：路由、侧栏（`AppSidebar.vue`）、i18n、router
- 功能：糖果 / 指纹 / 鹈鹕 / SIWC 四类检测，覆盖 OpenAI、Anthropic、Gemini、Antigravity；管理面板新增「账户评估」页；每账号每类留最近 20 条记录；批量测试；"启用"开关直接改账号可调度状态。
- 许可证：仓库整体 **LGPL-3.0**，并捆绑 `licenses/`（CPA-Codex-Candy-Eval-MIT、GPL-3.0、LM-detector-MIT、Sub2API-LGPL-3.0、Lucide、Renderers）。
- **致命问题**：改的是 `sub2api` 核心（新增 admin 通道、service、路由、前端页面），**无法用 `.s2plugin` 能力模型表达**（插件只能注册 `openai.oauth.outbound_transport.v1`，不能加侧栏页/管理员 HTTP 通道）；且二进制被镜像更新覆盖 → 失效、被锁版本（只出过 v0.2.19 预编译）、维护成本高。

---

## 3. 最佳业务逻辑来源：`haowang02/cpa-plugin-codex-candy-eval`（MIT）

> 这是 codex-candy-eval 的"上游/同源"实现，宿主是 **CLIProxyAPI（CPA）**，不是 Sub2API。

- 干净的 Go 模块：`internal/plugin/{candy,fingerprint,fingerprint_detector,pelican,siwc,codex,model,state,credentials,inspection,runs,ui}.go`
- 参考库：`internal/plugin/data/lm-detector/{enrollment-suite.json, models.json, shared_detector.bin}`（指纹检测的模型参考库，MIT）
- 前端：`internal/plugin/web/{app,candy,fingerprint,pelican,siwc,catalog,components,credentials,providers,storage,host}.js + style.css`
- 宿主适配：`cmd/cpa-codex-candy-eval/{cabi.go,main.go}` —— 通过 **C ABI**（`cliproxy_host_api`）与 CPA 通信，打包成 CPA 插件商店的 `.so`。
- 许可证：**MIT（Copyright 2026 Hao Wang）** —— 最干净、最适合二次开发的引擎来源。

**四种检测引擎细节**（移植目标）：
- **糖果（candy）**：`candyPrompt`（"不使用任何外部工具回答以下问题…"），正确答案 **21**；`candyMaxRuns=10`；`evaluateCandy` + `hasStandalone21` 评分；答错=结果（不重试），只有请求失败才重试（最多 3 次）。
- **指纹（fingerprint）**：基于 lm-detector 参考库，随机抽 **3 道数字挑战**，与候选模型比对、排名；三题都有效才给概率；"不像语言模型回答"时不给排名；每账号并发 1-3。
- **鹈鹕（pelican）**：两版——原版生成"鹈鹕骑自行车"HTML 动画；多合一版生成 SVG，加孙悟空/秦始皇/北极熊，并让鹈鹕喊出糖果答案；每账号保留 20 条带缩略图记录。
- **SIWC**：仅 OpenAI OAuth；每账号 1 次短 Responses 请求（不重试），按结果/错误码判定 `SIWC 可用 / 订阅共享受限 / 额度或频率受限 / 认证异常`。

---

## 4. 可复用的 `.s2plugin` 骨架（横向对比）

| 项目 | ★ | 最后提交 | 许可证 | 语言 | 是否完整 `.s2plugin` | 亮点 | 短板 |
|---|---|---|---|---|---|---|---|
| **sub2api-excel2api-plugin**（openai-account-health） | ~29 | 2026-09-25 | **无 LICENSE（仅 NOTICE）** | Go | ✅ 已发 0.5/0.6/0.7 | **最接近**：Transport 透传 + HostService + iframe UI + Cron/条件调度 + **并行降智检测（ModelTrace 数字挑战 292–332）** + 结果走 `status_json`；`requires >=0.2.7 <0.3.0` | **无开源许可证**，代码不能直接复用到开源项目；检测只有"指纹"一种 |
| **sub2api-oai-basispoints** | ~3 | 2026-10-01 | **MIT**（`internal/pluginapi` 为 LGPL） | Go | ✅ 多架构 | 干净 MIT；`manifest.source.json` + integration/ui 测试；linux amd64/arm64、darwin、windows 全平台 | 主功能是 BPS 隧道，UI 较简单；需自建检测模块 |
| **sub2api-state-kit** | ~37 | 2026-09-25 | LGPL-3.0 | Go | ✅ 面向 0.2.8 | **诊断面板 + 质量探针**（"说出最新苹果手机→17"）最接近糖果测试的 UX；`plugin-validation.md` 验证文档极佳 | LGPL 有传染性；质量探针绑定 state 票据逻辑 |
| **sub2api-turn-state-plugin** | — | 2026-09-21 | LGPL-3.0 | Go | ✅ | 内置 `pluginapi/` proto 副本 + `tests/*.test.cjs` | 功能偏 state |
| **sub2api-state-guard** | ~10 | 2026-09-20 | 无 LICENSE | Go | ✅ | `build.sh` + `manifest.source.json` + `tools/sync-pluginapi.sh` | 无许可证 |
| **sub2api-plugin-template**（Nanboom233） | — | 2026-11-08 | LGPL-3.0 | Go | 模板 | 脚手架/构建样板 | 只是模板 |
| 其他生态 | state-reuse ★33、oauth-sleeper ★20(Py)、magpie ★5(JS)、LightBridge ★156、sub2api-plus ★19 | — | — | — | — | 证明生态活跃 | 非本需求 |

社区里还有概念相近的外部工具可借鉴：`noonwake-ai/ai-drool-router ★4`（定时糖果+画图题作弊检测）、`Niutopia/sub2api-modeltrace-kit ★3`（模型真假检测/自动暂停）、`Tonwed/MGE_Sub2API ★5`、`ZYH1207/ManageAPI ★4`（降智探针）。**说明"降智检测做成插件"已有大量先例，方案可行。**

### 打包/签名可复用样板
- `sub2api-excel2api-plugin/tools/package.py`：用 `cryptography` 的 Ed25519，`PUBLIC_KEY/KEY_ID` 常量，生成+校签 + `files` 哈希 + 固定时间戳 zip（`runtimes/` 置 0755）。
- `sub2api-oai-basispoints/manifest.source.json` + `sub2api-state-guard/tools/sync-pluginapi.sh`：清单源文件与 proto 同步的样板。

---

## 5. 三条路线的取舍（回答"直接用 / 基于现有改 / 自己开发"）

| 路线 | 可行性 | 代价 | 结论 |
|---|---|---|---|
| **A. 直接用现成 `.s2plugin`** | ✗ | — | 没有一个项目同时具备"全套检测 + 可上传"。excel2api 只有指纹一种且无许可证；codex-candy-eval 是补丁不是插件。**不可行** |
| **B. 纯自研** | ✓ | 高 | 骨架、四种引擎、打包签名都已开源；从零重写属重复造轮子，还容易踩 Forward 透传/签名的坑。**不划算** |
| **C. 基于现有改造/合并** ✅ | ✓ | 中 | **骨架取 MIT 项目 + 引擎移植 MIT 的 CPA 插件 + 重写 host 适配层 + UI 迁进 iframe**。工作量集中在适配，风险可控。**推荐** |

### 具体基座建议（按许可证偏好二选一）
- **要 MIT/最宽松**：骨架用 `sub2api-oai-basispoints`（MIT，多架构、有集成/UI 测试）；引擎移植 `cpa-plugin-codex-candy-eval`（MIT）；`sub2api-excel2api-plugin` 仅作**架构参考**（因为无 LICENSE，不复制其代码，只借鉴"并行 worker + ResolveOutboundIdentity + status_json 出结果"的设计）。
- **接受 LGPL-3.0**（Sub2API 本体也是 LGPL-3.0，插件生态普遍 LGPL）：骨架可直接用 `sub2api-state-kit`（诊断面板/验证文档最好）或参考 `sub2api-excel2api-plugin` 的结构自行落地。

> ⚠️ **许可证注意**：`codex-candy-eval`（用户现有）是 LGPL-3.0；`cpa-plugin-codex-candy-eval` 是 MIT（更优）。`sub2api-excel2api-plugin` **没有 LICENSE 文件**，严格说不能直接复制其代码进开源项目——要么只取思路，要么联系作者授权。

---

## 6. 实现方案（分阶段）

### 全局约束
- 目标宿主：0.2.21 / 0.2.22 → `manifest.requires.sub2api = ">=0.2.7 <0.3.0"`，`recommended = 0.2.21`，并写 `tested_sub2api_versions`。
- 平台：Linux amd64（用户在 Docker 里跑），后续可加 arm64/darwin/windows。
- 能力：`openai.oauth.outbound_transport.v1`（platform=openai，account_type=oauth）。
- **MVP 只覆盖 OpenAI OAuth 的糖果 + 指纹 + SIWC；鹈鹕放二阶段；多供应商暂不支持**（受能力白名单限制）。

### Phase 0 — 决策与准备
1. 选定许可证（MIT vs LGPL-3.0）与基座。
2. 生成 Ed25519 密钥对（发布者 key id + 公钥）。
3. 用 speckit 写 `spec/plan/tasks`（本仓库已有 `.specify/` 脚手架）。

### Phase 1 — 骨架跑通（先"能装、能透传、不断流"）
- 落地 `manifest.json` + `cmd/<plugin>/main.go` + `TransportPlugin.Forward` **纯透传** + `InitHostServices` + 最简 `ui/index.html`（读 `plugin.status`）+ `tools/package.py`。
- 本地 `allow_unsigned: true` 装入 0.2.21，验证：**装上、启用、真实 Codex 流量不断**（这是最关键的安全门槛）。
- 复用 `manifest.source.json` 模板、`sync-pluginapi.sh` 同步 proto。

### Phase 2 — 检测引擎移植 + host 适配
- 把 `candy.go`、`fingerprint*.go`（+`data/lm-detector/*`）、`siwc.go` 拷入 `internal/eval/`，**去掉 CPA 的 credential/C-ABI 依赖**，改为注入 `ResolveOutboundIdentity` 得到的 token/headers/proxy。
- 参考 `sub2api-excel2api-plugin/internal/adapter/server.go` 的 `runDiagnostics/runDiagnosticJob`：`ListAccounts(platform=openai,account_type=oauth)` → 每账号 `ResolveOutboundIdentity` → **并发 worker 池（1–16）** → 逐个模型发检测请求 → 结果汇总。
- 结果结构写入 `Health.status_json`（供 UI `plugin.status` 轮询）。

### Phase 3 — UI（iframe 内）
- 移植 `web/{candy,fingerprint,siwc}.js` 到插件 `ui/assets/`，接 `bridge-v1.js`（`config.load/save` + `plugin.status` 轮询）。
- 做账号选择（`ListAccounts` 结果）、模型/并发/次数配置、结果表（状态/耗时/用量/详情）、历史记录（KV 持久化，每账号每类留最近 N 条）。

### Phase 4 — 鹈鹕 + 调度 + 打磨
- 移植 `pelican.go`（HTML/SVG 两版、缩略图、20 条记录）。
- 移植/参考 `schedule.go`：Cron 或条件任务（可调度账号出现/启动时测）。
- i18n、错误提示、额度提示、停止/取消。

### Phase 5 — 打包、签名、发布
- Ed25519 签名、`files` 哈希校验、跨架构构建、`trusted-publishers.yaml` 与公钥文档。
- README / CHANGELOG / GitHub Release（`.s2plugin` + SHA256SUMS）。

### Phase 6 — 验证清单
- 官方宿主契约测试 + `go test/race/vet` + `node --test` + 跨平台编译。
- 真实 0.2.21 冒烟：**装上不被镜像更新清除**、**Forward 不断流**、糖果/指纹/SIWC 三类跑通、结果落库、启用/停用/卸载正常。

---

## 7. 风险与对策

| 风险 | 影响 | 对策 |
|---|---|---|
| 能力白名单只放行 OpenAI OAuth | 丢失 Anthropic/Gemini/Antigravity 的 API-Key 检测 | 明确 MVP 范围；多供应商仍需补丁版或另做 |
| 启用即接管真实流量 | Forward bug 直接断 Codex | Phase 1 先做纯透传契约测试；上线用 rollout 灰度 |
| `excel2api` 无 LICENSE | 直接复制代码有法律风险 | 只借鉴设计；代码取 MIT 项目 |
| 许可证传染（LGPL） | 决定整个开源项目许可 | 先定许可再选基座（MIT 路线见 §5） |
| ModelTrace/lm-detector 数据有各自许可 | 合规 | 保留 `modeltrace_LICENSE`、`lm-detector/LICENSE` 归属 |
| 宿主 0.3.0 变更 manifest/能力/桥接 | 插件失效 | 锁 `requires` 范围，跟踪 `Wei-Shaw/sub2api` 上游 |

---

## 8. 给用户的三个决策点（确认后即开工）

1. **许可证**：走 **MIT**（骨架 oai-basispoints + 引擎 cpa，最干净）还是 **LGPL-3.0**（可直接用 state-kit / 参考 excel2api，UI 现成度最高）？
2. **功能范围**：MVP 是否接受 **只测 OpenAI OAuth**（糖果+指纹+SIWC），鹈鹕二期，多供应商不做？
3. **首个交付**：先要一个"能装上、不断流、带糖果+指纹检测"的**最小可用 `.s2plugin`** 让你在 0.2.21 上验证，还是直接冲全套？
