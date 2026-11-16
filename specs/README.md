# Specs

本目录保存 speckit 生成的规格、计划和任务，随 Git 提交，供多设备共享。

- 可以写：功能目标、验收标准、任务拆分、脱敏后的架构说明。
- 不可以写：真实服务器地址、域名、SSH 凭据、数据库密码、JWT、OAuth Token、TOTP 密钥。
- `.specify/` 是每台设备本地的工具链状态，不进入 Git；需要时在设备上重新初始化。
- 升级流程本身的权威文档在 `docs/UPSTREAM_SYNC.md` 和 `docs/UPGRADING.md`，不依赖 `.specify/`。
