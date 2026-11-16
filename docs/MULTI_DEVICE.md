# 多设备协作与上游迭代流程

本文定义多个设备共同维护 `feat/model-guard` 的流程。核心原则是：**GitHub 上的
`feat/model-guard` 是唯一事实源；上游同步由 GitHub Actions 统一执行并开 PR；本地设备只负责
开发和审查，不各自维护一套上游合并逻辑。**

## 1. 哪些东西放哪里

| 内容 | 位置 | 是否提交 Git | 说明 |
|---|---|---|---|
| 服务器地址、账号密码、SSH 私钥口令 | 每台设备的根目录 `.env` | 否 | 从 `.env.example` 复制后手动填写；通过密码管理器传递，不要发聊天/邮件 |
| speckit 工具链状态 | `.specify/` | 否 | 每台设备本地安装/生成；里面的 `feature.json`、`scripts` 是机器相关状态 |
| 功能规格、计划、任务 | `specs/` | 是 | 脱敏后的 speckit 产物，多设备共享；不含真实地址、密码、JWT |
| 升级 Runbook 和同步说明 | `docs/UPSTREAM_SYNC.md`、`docs/UPGRADING.md` | 是 | 所有设备按同一份文档执行 |
| 同步脚本和工作流 | `tools/upstream/`、`.github/workflows/` | 是 | 可审计、可复现 |
| 远程运维脚本、canary JWT、日志、本地数据库副本 | `.local-run/`、`data/` | 否 | 本机运维数据，已加入 `.gitignore` |

> **结论：** `.specify/` 是本机工具状态，不需要提交；`specs/` 是团队共享的规格文档，应该提交。
> 其他设备即使不安装 speckit，也能直接阅读 `specs/*/spec.md`、`plan.md`、`tasks.md`。

## 2. 分支和职责

| 分支 | 用途 | 谁可以写 |
|---|---|---|
| `feat/model-guard` | 集成分支、默认分支、生产镜像来源 | 只通过 PR 合并 |
| `feature/<topic>` | 单个功能开发 | 各设备开发者 |
| `sync/upstream-<tag>` | GitHub Actions 自动生成的升级 PR 分支 | 仅同步工作流；冲突时由指定负责人接管 |
| `backup/model-guard-<版本>-<时间>` | 合并前本地备份 | 只在本机，不 push |

规则：

- `feat/model-guard` 开启分支保护，禁止直接 push 和 force push。
- 功能分支可以 rebase 自己的提交，但**不要 rebase 共享的 `feat/model-guard`**。
- 同一时间只允许一个人处理上游同步 PR；其他设备等 PR 合并后 `git pull`。
- 所有设备使用同一个 tag 命名规则：`vX.Y.Z`，由发布负责人在合并后的 `feat/model-guard` 上打。

## 3. 新设备首次配置

1. 安装 Git、PowerShell 7、Go 1.27.4、Node.js 24、pnpm 9.15.4。需要本地构建 Docker 镜像时再安装 Docker。
2. 克隆仓库并进入 `feat/model-guard`：

   ```powershell
   git clone https://github.com/Ericzhouyihe/sub2api-codex-guard.git
   Set-Location sub2api-codex-guard
   git switch feat/model-guard
   ```

3. 复制环境变量模板，从密码管理器填入真实值：

   ```powershell
   Copy-Item .env.example .env
   # 编辑 .env：SERVER、DOMAIN、SSH_HOST、SSH_USER、SSH_PASS 等
   ```

4. 启用 rerere，减少重复冲突处理：

   ```powershell
   git config rerere.enabled true
   git config rerere.autoupdate true
   ```

5. 验证同步脚本能识别当前环境（不会合并、不会改文件）：

   ```powershell
   pwsh ./tools/upstream/sync-model-guard.ps1 -DryRun
   ```

6. 如需使用 speckit：

   - 在 DSH 中安装对应的 `speckit-*` skills；
   - 在仓库根目录初始化 `.specify/`（或从主设备复制后重新执行初始化，确保脚本路径正确）；
   - `specs/` 直接从 Git 拉取，不要另建一份本地 spec 目录。

## 4. 日常功能开发

```powershell
git switch feat/model-guard
git pull --ff-only
git switch -c feature/<topic>

# 修改代码、spec、docs
# 按 docs/model-guard.md 的验证命令跑测试

git add <files>
git commit -m "feat: <说明>"
git push -u origin feature/<topic>
```

然后在 GitHub 开 PR 到 `feat/model-guard`。PR 合并后，其他设备：

```powershell
git switch feat/model-guard
git pull --ff-only
```

## 5. 上游有更新时

### 5.1 正常路径（推荐，不在每台设备上手动合并）

1. `.github/workflows/upstream-sync.yml` 每天 02:17 UTC 检查上游稳定 tag。
2. 发现新 tag 后，工作流执行：
   - 备份分支；
   - `git merge --no-ff`；
   - Model Guard 文件和 10 组接线检查；
   - focused Go tests；
   - `go build -tags embed ./cmd/server`；
   - 创建 `sync/upstream-<tag>` 分支和 PR。
3. 指定负责人在 GitHub 审查 PR：
   - 检查冲突解决是否保留上游新逻辑；
   - 检查 Model Guard 接线和测试结果；
   - 通过正常 CI 后合并。
4. 其他设备收到合并通知后 `git pull --ff-only`，不需要各自再跑一次上游合并。
5. 发布负责人按 `docs/UPGRADING.md` 打 tag、构建镜像、canary、生产切换。

### 5.2 手动路径（Action 失败或需要临时指定 tag）

在一台设备上：

```powershell
git switch feat/model-guard
git pull --ff-only
git status --short --untracked-files=no    # 必须为空
pwsh ./tools/upstream/sync-model-guard.ps1 -Tag v2.21.6
```

如果脚本报告冲突，它会 `git merge --abort`，不会留下半合并工作区。此时按
`docs/UPSTREAM_SYNC.md` 第 4 节处理：

```powershell
git merge --no-ff --no-commit v2.21.6
# 手动解决冲突，保留上游新逻辑后再恢复 Model Guard 接线
git add <resolved-files>
pwsh ./tools/upstream/sync-model-guard.ps1 -VerifyOnly
git commit -m "merge: upgrade to Sub2API 2.21.6"
```

验证通过后 push 到 `sync/upstream-v2.21.6` 分支并开 PR，不要直接 push 到 `feat/model-guard`。
冲突处理期间其他设备不要同时改同一组接线文件。

### 5.3 设备之间的边界

- `.env` 不跨设备同步；每台设备独立填写，服务器凭据只从密码管理器获取。
- `.local-run/`、`data/`、`.specify/` 不同步。
- `specs/`、`docs/`、`tools/upstream/` 通过 Git 同步。
- 不在聊天工具中发送 JWT、数据库密码、SSH 密码或包含这些值的截图。

## 6. 发布与生产

1. 合并后的 `feat/model-guard` 通过 CI。
2. 发布负责人创建 `vX.Y.Z` tag 并 push。
3. `docker-image.yml` 构建 GHCR 镜像。
4. 按 `docs/UPGRADING.md` 做备份、canary、生产切换和回滚。
5. 自动同步只负责代码，不会自动部署生产。

## 7. 合并前检查清单

- [ ] PR 目标是 `feat/model-guard`，不是 `main` 或其他分支。
- [ ] `git status --short --untracked-files=no` 为空（手动同步时）。
- [ ] Model Guard 的 8 个核心文件存在，10 组接线标记存在。
- [ ] focused Go tests 通过。
- [ ] `go build -tags embed ./cmd/server` 通过。
- [ ] 前端有改动时，按 `docs/model-guard.md` 跑 `pnpm`/`vue-tsc`/`vitest` 验证。
- [ ] 没有把 `.env`、`.local-run/`、JWT、数据库密码带入 diff。
- [ ] 其他设备已知道合并窗口，不会同时处理同一个上游 tag。
