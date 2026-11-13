# 第三方组件与参考实现

本项目以 MIT 发布，但包含或改编了以下第三方代码。分发时请保留本文件与相应许可证。

## 1. Sub2API 插件协议（internal/pluginapi）

- 来源：https://github.com/Wei-Shaw/sub2api （`backend/pkg/pluginapi/v1`）
- 许可证：LGPL-3.0（`internal/pluginapi/LICENSE.LGPL-3.0`、`COPYING.GPL-3.0`）
- 说明：`plugin.pb.go`、`plugin_grpc.pb.go`、`runtime.go`、`plugin.proto`、`manifest.schema.json` 为协议桩代码，未修改协议语义。

## 2. CPA Codex Candy Eval（糖果题与评分逻辑）

- 来源：https://github.com/haowang02/cpa-plugin-codex-candy-eval
- 许可证：MIT（Copyright 2026 Hao Wang）
- 说明：`internal/guard/candy.go` 中的 `candyPrompt` 与 `hasStandalone21` 评分逻辑复刻自该项目的 `internal/plugin/candy.go`。

## 3. Sub2API OAI Basis Points（插件骨架与 UI Bridge 客户端）

- 来源：https://github.com/Roins-hub/sub2api-oai-basispoints
- 许可证：MIT（Copyright Roins-hub）
- 说明：`internal/guard/forward.go`、`clients.go` 的透传帧处理、`tools/package/main.go` 的打包/签名流程，以及 `ui/assets/bridge-v1.js` 的 UI Bridge 客户端均改编自该项目。

## 4. LM Detector（指纹检测模型与题库）

- 来源：https://github.com/Ikaleio/lm-detector
- 许可证：MIT（`internal/guard/data/lm-detector/LICENSE`）
- 说明：`internal/guard/fingerprint_detector.go` 及 `internal/guard/data/lm-detector/`（`shared_detector.bin`、`models.json`、`enrollment-suite.json`）移植/复制自该项目；`internal/guard/fingerprint.go` 的挑战流程改编自 CPA Codex Candy Eval（MIT，Copyright 2026 Hao Wang）。

## 5. CPA Codex Candy Eval（鹈鹕与 SIWC 参考）

- 来源：https://github.com/haowang02/cpa-plugin-codex-candy-eval
- 许可证：MIT（Copyright 2026 Hao Wang）
- 说明：`internal/guard/pelican.go` 的鹈鹕提示词、代码块提取与变体设计改编自 `internal/plugin/pelican.go`；`internal/guard/siwc.go` 参考其 SIWC 状态语义，因 Sub2API 无 CPA 的 SIWC 路由/元数据，实现为最小可用性探测。

## 6. Sub2API Excel2API Plugin（宿主账号服务调用参考）

- 来源：https://github.com/1812095643/sub2api-excel2api-plugin
- 许可证：仓库未附独立 LICENSE 文件（使用前请向作者确认）
- 说明：仅参考其 `InitHostServices` / `ListAccounts` / `ResolveOutboundIdentity` 的调用方式，未复制代码。

## 7. codex-candy-eval 全功能 fork（codex-eval/）

- 来源：https://github.com/Nanboom233/codex-candy-eval
- 许可证：LGPL-3.0（`codex-eval/LICENSE`）；其 `licenses/` 目录另含 CPA MIT、LM-detector MIT、Sub2API LGPL-3.0 等第三方许可证。
- 说明：`codex-eval/` 保留原项目全部补丁、UI 与测试，仅修复补丁换行、增加可移植的本地构建参数。该子目录按其原许可证分发，不适用根项目的 MIT 许可。

