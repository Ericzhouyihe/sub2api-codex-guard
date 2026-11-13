# Contract: Artifact Layout & Compatibility Record

**Feature**: `001-codex-eval-v021-compat`
**Invariant**: 产物路径、命名与哈希可被脚本和人工同时验证。

## Output Tree

```text
codex-eval/dist/build/
└── linux-amd64/
    ├── sub2api-plugins-installer-v0.2.21-linux-amd64
    ├── sub2api-plugins-v0.2.21-linux-amd64
    └── SHA256SUMS
```

可选 `linux-arm64/` 结构与 `linux-amd64/` 相同。

## Filename Contract

```text
sub2api-plugins-installer-<upstream_tag>-<platform>
sub2api-plugins-<upstream_tag>-<platform>
```

- `<upstream_tag>` 必须带 `v` 前缀，与 `UPSTREAM_TAG` 完全一致。
- `<platform>` 为 `linux-amd64` 或 `linux-arm64`。
- 文件名不得包含空格、时间戳或 `latest`。

## SHA256SUMS Contract

- UTF-8 文本，每行 `<64 位小写 hex><两个空格><文件名>`。
- 必须包含同目录下所有交付二进制；不得包含自身。
- 校验命令：`sha256sum -c SHA256SUMS`（Linux）或等价脚本。

## Compatibility Record Format

建议在 `docs/codex-eval-fork.md` 维护以下表格（人读）：

| Sub2API 版本 | codex-eval 基线 | 状态 | 验证日期 | 产物 SHA-256 前缀 |
|--------------|-----------------|------|----------|-------------------|
| v0.2.21 | v0.4.6 | verified | 2026-11-13 | `D49CAF6B` / `A0062137` |

状态只能为 `verified`、`failed`、`deprecated`；未验证版本不得标为 `verified`。

## 版本边界

- 本布局只承诺 `v0.2.21`。
- 其他上游版本若未重新验证，构建产物不得命名成已支持版本。
