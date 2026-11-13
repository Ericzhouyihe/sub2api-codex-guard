# 可选 CI 示例（不属于交付路径）

这里的 GitHub Actions 文件是历史示例，**不是本项目的构建依赖**。官方交付声明：

- 本功能只要求本地构建：`python codex-eval/scripts/build.py`；
- 不需要 GitHub Actions、远程 CI 或任何云端凭据；
- 是否把这两个文件重新放回 `.github/workflows/` 由仓库维护者自行决定，放回也不会改变本地构建路径。

保留它们仅用于参考或后续可选的自动构建实验；正式验收以 `contracts/build-cli.md` 的本地命令为准。
