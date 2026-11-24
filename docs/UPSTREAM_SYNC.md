# 上游同步（保留 Model Guard）

本仓库是 `ranxi2001/sub2api` 的功能分支，`feat/model-guard` 上叠加了「模型哨兵」。
上游更新时不要重新复制整个仓库，也不要直接覆盖 `backend/internal/codexeval/`；使用本文的
「合并上游 tag + 完整性检查 + focused tests + PR」流程。

## 1. 为什么采用合并而不是重做

Model Guard 的绝大部分代码是独立新增的文件：

- `backend/internal/codexeval/`：糖果、指纹、历史、巡检、模型控制和通知。
- `frontend/src/views/admin/CodexEvalView.vue`：独立后台页面。
- `tests/model-guard-*.cjs`、`docs/model-guard.md`、`LICENSES/model-guard/`。

与上游共享、需要重点核对的接线点只有以下 10 组（11 个文件）：

| 文件 | Model Guard 接线 |
|---|---|
| `backend/internal/server/routes/admin.go` | `/codex-eval` 路由组、账户巡检路由、设置路由 |
| `backend/internal/server/router.go` | `InitializeCodexEval`、`InitializeCodexInspection`、静态页面路由 |
| `backend/cmd/server/main.go` | `codexeval.StopInspection()` 停机钩子 |
| `backend/internal/web/embed_on.go` | `/codex-eval` 绕过嵌入前端回退 |
| `backend/internal/repository/account_repo.go` | 巡检受管 `extra` 字段的行锁保护 |
| `backend/internal/repository/scheduler_cache.go` | 调度缓存中的巡检字段失效 |
| `backend/internal/service/account.go` | 模型准入前的限制层判断 |
| `backend/internal/service/admin_account.go` | 账户编辑时保留/清理巡检字段 |
| `backend/internal/service/openai_model_retirement.go` | 模型下线路由前的限制层判断 |
| `frontend/src/components/layout/AppSidebar.vue`、`frontend/src/router/index.ts` | 菜单项与 `/admin/codex-eval` 路由 |

因此日常升级以 `git merge` 为主：上游新增的代码自动进入，冲突通常只集中在上述文件。
脚本会检查这些接线是否仍然存在，运行 focused Go tests 并编译 `cmd/server`；如果冲突或检查失败，
会中止合并，不会留下半合并的工作区。

> 不建议用 `git rebase` 或 `git push --force` 更新 `feat/model-guard`：这是对外发布分支，
> 重写历史会让其他人的 clone、备份分支和 GitHub PR 失效。合并提交是可审计的升级记录。

## 2. 一次性配置

```powershell
git remote add upstream https://github.com/ranxi2001/sub2api.git
git config rerere.enabled true
git config rerere.autoupdate true
```

`rerere` 会记住重复冲突的解决结果，后续升级更省事。远程地址只写在 `.git/config`，不进入仓库。

## 3. 本地手动同步

```powershell
git switch feat/model-guard
git status --short --untracked-files=no    # 必须为空
pwsh ./tools/upstream/sync-model-guard.ps1
```

指定版本：

```powershell
pwsh ./tools/upstream/sync-model-guard.ps1 -Tag v2.24.3
```

只检查、不合并：

```powershell
pwsh ./tools/upstream/sync-model-guard.ps1 -Tag v2.24.3 -DryRun
```

> Windows 本机首次运行如果 Go 依赖下载超时，可先设置
> `$env:GOPROXY = 'https://goproxy.cn,direct'`，再运行脚本。

手动解决冲突后，只做完整性检查、测试和编译检查（允许工作区处于已解决待提交状态）：

```powershell
pwsh ./tools/upstream/sync-model-guard.ps1 -VerifyOnly
```

脚本动作：

1. 校验当前分支必须是 `feat/model-guard`，且已跟踪文件没有未提交修改。
2. 从 `https://github.com/ranxi2001/sub2api.git` 选择最新的稳定 `vX.Y.Z` tag（或使用 `-Tag`）。
3. 建立 `backup/model-guard-<旧版本>-<时间戳>` 备份分支。
4. `git merge --no-ff --no-commit <tag>`。
5. 检查 Model Guard 文件和 10 组接线点。
6. 运行：

   ```powershell
   go test -p 1 -tags unit ./internal/codexeval ./internal/service ./internal/repository ./internal/handler/admin `
     -run 'Test(Codex|ModelGuard|Fingerprint|HasStandalone)' -count=1
   go build -tags embed -trimpath -o <临时路径> ./cmd/server
   ```

7. 通过后提交 `merge: upgrade to Sub2API <version>`。
8. 不自动 push；确认 diff 后执行 `git push origin feat/model-guard`。

如果第 4 步出现冲突，脚本会打印冲突文件并执行 `git merge --abort`。按照下一节的规则手动
解决并 `git add` 后，运行 `pwsh ./tools/upstream/sync-model-guard.ps1 -VerifyOnly` 完成
完整性、测试和编译检查，再提交合并。

## 4. 冲突处理规则

1. **先保留上游新逻辑**，不要整文件选择 `ours` 或 `theirs`。
2. 再按上表逐项恢复 Model Guard 接线。
3. 如果上游改了函数签名，以新签名为准调整调用点；不要复制旧的调用代码。
4. `git add` 已解决的冲突后，运行 `sync-model-guard.ps1 -VerifyOnly` 做完整性、Go 测试和 server 编译检查。
5. 必须通过 focused Go tests 和 server build；前端改动还要按 `docs/model-guard.md` 的本地验证命令构建和测试。

常见冲突：

- `admin.go`：上游新增路由组时，把 `eval := admin.Group("/codex-eval")` 及账户/设置路由放回
  对应路由注册函数内，不要放到错误的中间件分组。
- `router.go`：`InitializeCodexEval` 必须在 `gateway` 角色提前返回之后、`v1` 路由组之前。
- `AppSidebar.vue` / `router/index.ts`：菜单项和路由是纯增量，通常可直接保留两边。
- `account_repo.go` / `admin_account.go`：巡检 `extra` 字段是受管字段，不能退回普通快照覆盖。

## 5. GitHub Actions 自动同步

`.github/workflows/upstream-sync.yml` 每天 02:17 UTC 检查一次上游，也可以手动触发：

1. 检出 `feat/model-guard`。
2. 运行同一个 `tools/upstream/sync-model-guard.ps1`。
3. 只有合并、接线检查、focused tests 和 server build 全部通过时，创建 `sync/upstream-<tag>`
   分支并向 `feat/model-guard` 开 PR。
4. 如果上游没有新 tag，workflow 正常结束，不产生 PR。
5. 如果冲突或测试失败，workflow 失败并保留日志；不会改 `feat/model-guard` 的远端分支。

> **必须配置 `UPSTREAM_SYNC_TOKEN`**：workflow 在合并前会先检查该 secret；在仓库 Secrets 中添加一个 fine-grained PAT（仅此仓库，
> Contents: Read and write、Pull requests: Read and write）。用默认 `GITHUB_TOKEN` 创建的 PR
> 不会触发后续 CI workflow，必须改用 PAT。仓库设置还需要允许 GitHub Actions 创建 pull request
> （Settings → Actions → General → Workflow permissions → Allow GitHub Actions to create and
> approve pull requests）。

合并 PR 后，`backend/cmd/server/VERSION` 已变为上游版本；`docker-image.yml` 会发现对应的
`vX.Y.Z` tag 不存在，自动创建 tag/Release、构建并推送 GHCR 镜像（含 `:latest`）。生产
compose 使用 `:latest` 且运行 Watchtower 时，服务器不会自动轮询；管理员在后台看到新版本后
点击「立即更新镜像」，由 Watchtower 拉取并重建。首次大版本或需要固定版本时，仍按
`docs/UPGRADING.md` 做备份、canary、生产验证和回滚。

## 6. 相关文件

- 同步脚本：[`tools/upstream/sync-model-guard.ps1`](../tools/upstream/sync-model-guard.ps1)
- 工作流：[`.github/workflows/upstream-sync.yml`](../.github/workflows/upstream-sync.yml)
- 升级 Runbook：[`docs/UPGRADING.md`](UPGRADING.md)
- 模型哨兵说明：[`docs/model-guard.md`](model-guard.md)
- 本次升级证据：[`docs/upgrades/2.24.3.md`](upgrades/2.24.3.md)

## 7. 安全边界

- 服务器地址、域名、SSH 账号和密码只放本地根目录 `.env`；`.env` 已被 `.gitignore` 忽略。
- 发布链路只推送源码提交；不从开发机执行 SSH/SCP 上传，也不把服务器凭据放进仓库、Workflow 或规格文档。
- 服务器更新只通过内置更新入口触发 Watchtower；Docker 模式不保留本地旧镜像或回退数据。
- `.local-run/`、`data/`、`.dsh/`、`.specify/` 是本机目录，不进入 Git。
- `specs/` 是项目规格、计划和任务，会提交到 Git；只能写脱敏后的流程和变量名，不能写入真实凭据。
- 生产数据库密码、JWT、TOTP 密钥仍保存在服务器 `deploy/.env`，不要把服务器 `.env` 复制到仓库。
- 多设备协作和 `.specify` 本地工具的处理方式见 [`docs/MULTI_DEVICE.md`](MULTI_DEVICE.md)。
