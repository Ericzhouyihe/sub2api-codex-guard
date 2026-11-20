# Codex 降智测试 · 插件配置页

Sub2API `.s2plugin`「Codex 降智测试（糖果 / 指纹 / 鹈鹕）」的 iframe 配置界面。纯静态 HTML/CSS/JS，无框架、无外部资源。

## 文件

| 文件 | 说明 |
| --- | --- |
| `index.html` | 页面结构（仅引用 `assets/bridge-v1.js`、`assets/app.js`、`assets/styles.css`） |
| `assets/bridge-v1.js` | Sub2API UI Bridge v1 客户端（原样复制自 MIT 参考实现 `sub2api-oai-basispoints`，校验 `event.source`、`bridge_token`、`request_id`） |
| `assets/app.js` | 配置读取/校验/保存、检测触发、状态轮询、分标签结果与鹈鹕预览 |
| `assets/styles.css` | 浅色主题、卡片式布局、响应式样式 |

页面通过 `.../index.html#bridge_token=<token>` 在 `sandbox="allow-scripts"` 的 iframe 中加载。缺少 token 时会提示从 Sub2API 插件管理页打开。

## 使用的 Bridge 消息

| 类型 | 方向 | 参数 / 响应 | 用途 |
| --- | --- | --- | --- |
| `sub2api.plugin.ready` | UI → 宿主 | — | 页面加载完成后通知宿主 |
| `config.load` | UI → 宿主 | 无参数；响应 `{ok:true, config}` | 读取当前配置 |
| `config.save` | UI → 宿主 | `{config}`；响应 `{ok:true, config}`（返回规范化配置） | 保存配置，保存后立即生效 |
| `config.test` | UI → 宿主 | 无参数（测试**已保存**的配置）；响应 `{ok:true, result}` 或 `{ok:false, error}` | 立即检测；插件异步启动，响应可能较慢 |
| `plugin.status` | UI → 宿主 | 无参数；响应 `{ok:true, result:{healthy, message, status_json}}`，`status_json` 为 JSON 字符串 | 轮询运行状态（页面可见时每 3000ms 一次） |
| `ui.resize` | UI → 宿主 | `{height}` | 渲染后与窗口尺寸变化时同步 iframe 高度 |
| `ui.notify` | UI → 宿主 | `{level, message}` | 保存成功、检测开始及操作失败时少量提示 |

页面同时兼容宿主返回的 `reply.config`（顶层，v0.2.21 实际形态）与 `reply.result.config`（旧文档形态）。

## 配置 Schema

`config.save` 提交的 `config` 对象：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `models` | `string[]` | 模型名列表；界面按每行一个编辑，至少 1 个、最多 10 个 |
| `runs` | `integer` | 每账号每模型糖果检测次数，1–10 |
| `concurrency` | `integer` | 账号并发数，1–16 |
| `effort` | `string` | 推理强度：`""` / `"low"` / `"medium"` / `"high"` |
| `account_ids` | `integer[]` | 账号 ID；界面用逗号/空格分隔输入，留空表示全部 OpenAI OAuth 账号 |
| `schedulable_only` | `boolean` | 仅检测宿主标记为可调度的账号 |
| `schedule_enabled` | `boolean` | 是否启用定时检测 |
| `schedule_minutes` | `integer` | 定时检测间隔（分钟），5–10080 |
| `schedule_tests` | `string[]` | 定时执行的项目：`candy` / `fingerprint` / `pelican` / `siwc`，至少 1 个 |
| `active_test` | `string` | 「立即检测」执行的项目 |
| `fingerprint_concurrency` | `integer` | 每账号指纹题并发，1–3（全局上限 6） |
| `pelican_variant` | `string` | `original` 或 `all-in-one` |

页面会保留 `config.load` 返回的未知顶层字段，并在 `config.save` 时原样带回，避免丢配置。

## 状态 Schema

`plugin.status` 的 `result.status_json` 解析后：

```json
{
  "version": "0.2.0",
  "tests": {
    "candy": {"running": false, "last_run_at": "", "summary": {"total": 0, "correct": 0, "wrong": 0, "failed": 0, "skipped": 0}, "results": []},
    "fingerprint": {"running": false, "summary": {"completed": 0, "partial": 0, "failed": 0}, "results": []},
    "pelican": {"running": false, "summary": {"completed": 0, "failed": 0}, "results": []},
    "siwc": {"running": false, "summary": {"available": 0, "failed": 0}, "results": []}
  },
  "accounts": [{"id": 1, "name": "account name", "status": "active", "schedulable": true}],
  "next_run_at": "RFC3339 或空",
  "logs": [{"time": "RFC3339", "level": "info", "message": "..."}]
}
```

- 每个测试对象同时也会以测试名作为顶层键输出，兼容 v0.1.0 的 `candy` 读取方式。
- `candy.results` 最多展示最近 100 条（最新在前）；鹈鹕结果最多保留 20 条，点击「查看图」在沙箱 iframe 中预览生成的 HTML/SVG。
- 结果状态颜色：糖果 `correct` 绿、`wrong` 红；指纹/鹈鹕 `completed` 绿、`partial` 黄；SIWC `available` 绿；`failed` 橙、`skipped` 灰。
- 页面只把动态内容写入 `textContent`，不拼接 HTML；鹈鹕生成的 HTML 也在 `sandbox="allow-scripts"` 的独立 iframe 中渲染。

## 交互说明

- **保存配置**：校验表单后调用 `config.save`；成功后以宿主返回的规范化配置回填。
- **立即检测**：先保存当前表单（校验不通过则中止），再调用 `config.test`；立即提示「检测已开始，结果会自动刷新」并继续轮询。有任意项目在运行时按钮禁用。
- **结果标签页**：糖果 / 指纹 / 鹈鹕 / SIWC；切换标签同时设置「立即检测」项目并标记未保存。
- **刷新状态**：手动触发一次 `plugin.status`。
- 轮询在页面隐藏时暂停，重新可见时立即刷新一次；`pagehide` / `unload` 时停止轮询并释放 Bridge。

## 本地检查

```powershell
node --check ui\assets\app.js
node --check ui\assets\bridge-v1.js
```
