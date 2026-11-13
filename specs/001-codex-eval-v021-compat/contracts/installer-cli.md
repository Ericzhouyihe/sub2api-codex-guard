# Contract: Installer CLI

**Feature**: `001-codex-eval-v021-compat`
**Entry point**: `sub2api-plugins-installer-<tag>-<platform>`（交互式）
**Invariant**: 替换 `/app/sub2api` 前必须完成备份；失败必须自动恢复。

## Interaction Flow

1. 打印标题与版本（`Sub2API Plugins v<buildVersion>`）。
2. 选择安装方式：
   - `[1] Docker / Compose`
   - `[2] Native executable`
   - `[3] Apple container`（仅 macOS arm64）
3. Docker 路径：自动选择或让用户选择运行中的 Sub2API 容器；检测容器架构与 `/app/sub2api` 路径。
4. 显示备份路径、目标路径与候选版本信息，请求确认。
5. 执行备份、stage 替换、权限修正、重启、健康检查。
6. 成功打印 `Installed. The application is ready.`；失败打印错误并尝试恢复。

## Guarantees

- **G-01 备份**: 在修改目标二进制前，必须把原文件复制到可读的备份路径；备份文件必须存在且非空。
- **G-02 原子替换**: 使用 `docker cp` 到临时路径 `/app/sub2api.plugins-new`，`chmod 755`，再 `mv -f` 到 `/app/sub2api`；不得直接覆盖读写中的目标文件。
- **G-03 权限**: 替换后文件属主与权限必须与原二进制一致（`chown $(stat -c '%u:%g' /app/sub2api)`、`chmod 755`）。
- **G-04 健康检查**: 重启后必须在 120 秒内通过 Sub2API 健康/就绪检查；失败视为安装失败。
- **G-05 自动恢复**: 任何替换后错误必须恢复备份并重启；恢复失败时保留备份路径并明确提示人工处理。
- **G-06 架构检查**: 候选二进制必须是 Linux 目标架构的 ELF；不匹配时拒绝安装。
- **G-07 版本提示**: 若构建版本与容器内已安装版本不一致，应提示用户确认，不得静默覆盖未知版本。
- **G-08 退出码**: `0`=成功；`1`=失败（含已回滚）；`2`=用户取消。

## Non-goals

- 不负责生成二进制（由 `contracts/build-cli.md` 的构建步骤生成）。
- 不修改 Sub2API 数据库，不迁移评估历史数据。
- 不实现 bind-mount；该方式需要用户自行修改 Compose/容器配置。
