# Sub2API 升级运行手册（保留 Model Guard）

本手册用于把本仓库的 `feat/model-guard` 分支升级到 `ranxi2001/sub2api` 的任意新版本，并在不影响现有用户、账户、巡检设置和历史记录的前提下完成生产切换。

第一次执行对象：**2.21.4 → 2.21.5**。从 2.21.6 起，日常发布优先使用内置更新通道（见 1.1）；本手册的备份、canary 和服务器手动切换步骤保留给首次大版本、数据库迁移或需要固定版本的场景。

## 0. 本地环境变量

本文档不记录真实服务器地址、域名、账号或密码。复制仓库根目录的 `.env.example` 为 `.env`（该文件已被 `.gitignore` 忽略），填写 `SERVER`、`DOMAIN`、`SSH_HOST`、`SSH_USER`、`SSH_PASS` 等变量。所有命令中的 `$SERVER`、`$DOMAIN` 等均从本地 `.env` 读取；生产数据库密码、JWT、TOTP 密钥只保存在服务器或本机 `.env`，不得写入仓库。

PowerShell 可临时载入：

```powershell
Get-Content .env | Where-Object { $_ -match '^[^#].*=' } | ForEach-Object {
  $k, $v = $_ -split '=', 2
  Set-Item -Path "Env:$($k.Trim())" -Value $v.Trim()
}
```

## 1. 输入值

| 变量 | 含义 | 2.21.5 示例 |
|---|---|---|
| `OLD_VERSION` | 当前生产版本 | `2.21.4` |
| `NEW_VERSION` | 目标官方版本 | `2.21.5` |
| `NEW_TAG` | 官方 Git tag | `v2.21.5` |
| `NEW_COMMIT` | tag 指向的 commit | `b9feadf7dd6228a4117e360821d9994b7cc54897` |
| `IMAGE_TAG` | 新镜像 tag | `ghcr.io/ericzhouyihe/sub2api-codex-guard:2.21.5` |
| `SERVER` | 生产服务器 | 从根目录 `.env` 读取 |
| `DOMAIN` | 公网域名 | 从根目录 `.env` 读取 |
| `DEPLOY_DIR` | 生产 compose 目录 | `/root/sub2api-deploy` |
| `NETWORK` | 生产 compose 的 Docker 网络 | `sub2api-deploy_sub2api-network` |
| `SOURCE_DIR` | 服务器源码目录 | `/root/sub2api-mg-src-2.21.5` |
| `BACKUP_DIR` | 本次备份目录 | `/root/backups/model-guard-2.21.5-<时间戳>` |
| `CANARY_DB` | canary 数据库 | `sub2api_canary_215` |
| `CANARY_SUFFIX` | canary 容器/目录后缀 | `215` |
| `CANARY_PORT` | canary 宿主端口 | `18082` |

> 不要把 SSH 密码、JWT、数据库密码写入仓库。命令中的 `$JWT`、`$POSTGRES_PASSWORD` 等由当前 shell 环境提供。

### 1.1 更新通道（2.21.6 起）

日常升级只发布源码，不向服务器上传任何文件：

1. 本地合并上游 tag，跑通第 5–6 节门禁。
2. 只推送 `feat/model-guard`。`docker-image.yml` 在 `VERSION` 对应 tag 不存在时自动创建 `vX.Y.Z` Release，构建并推送 `:X.Y.Z`、`:latest`。
3. 服务器后台通过内置检查读取本 fork 的 Release；管理员确认后点击「立即更新镜像」。
4. Watchtower 拉取 `:latest` 并重建 `sub2api`；`WATCHTOWER_CLEANUP=true` 删除旧镜像。

约束：

- 开发机不执行 `ssh`/`scp`/远程 `docker` 命令，不保存或传递服务器凭据。
- 服务器默认 `WATCHTOWER_HTTP_API_PERIODIC_POLLS=false`，不会自动轮询；只有点击按钮才更新。
- Docker 更新不创建本地数据快照或二进制备份；回退只使用 GHCR 中已发布的旧 tag（见 10.3）。
- 内置通道不做备份，也不会拦截不可逆迁移。上游发布说明要求升级前备份，或新增迁移含 `DROP`、`TRUNCATE`、`DELETE FROM`，或用 `UPDATE` 清除既有数据时（如 2.25.0 的 `398_drop_user_traffic.sql`、2.25.2 的 `399_remove_oauth_429_model_blocks.sql`、2.25.4 的 `400_drop_request_diagnostics.sql`），必须先确认这些数据是否需要保留；需要时先在服务器导出，再点击「立即更新镜像」（检查步骤见 5.3）。
- 6.2–9 节的服务器手动构建、canary、compose 切换仅用于例外场景。

## 2. 前置条件

- 本地仓库在 `feat/model-guard`，工作区干净（只允许 `.local-run/`、`data/` 未跟踪）。
- 能 SSH 登录生产服务器，且能执行 `docker`、`docker compose`、`pg_dump`。
- 生产 compose 使用持久化目录：`./data`、`./postgres_data`、`./redis_data`。
- 已确认新版本来自官方仓库 `https://github.com/ranxi2001/sub2api.git`，不是供应商仓库。
- 生产当前健康，且已记录当前版本、用户数、账户数、Model Guard 历史条数。

## 3. 基线记录（只读）

```bash
ssh root@$SERVER
cd $DEPLOY_DIR

curl -fsS http://127.0.0.1:8080/health
docker inspect -f '{{.Config.Image}} {{.State.Health.Status}}' sub2api
docker exec sub2api /app/sub2api -version

docker exec -i sub2api-postgres psql -U sub2api -d sub2api -Atc \
  "select 'users='||count(*) from users; select 'accounts='||count(*) from accounts;"
python3 - <<'PY'
import json
p='/root/sub2api-deploy/data/codex-eval/history.json'
try:
    d=json.load(open(p))
    results=d.get('results',{}) if isinstance(d,dict) else {}
    fingerprints=d.get('fingerprints',{}) if isinstance(d,dict) else {}
    candy_accounts=len(results) if isinstance(results,dict) else 0
    candy_records=sum(len(v) for v in results.values() if isinstance(v,list)) if isinstance(results,dict) else 0
    fingerprint_records=sum(len(v) for v in fingerprints.values() if isinstance(v,list)) if isinstance(fingerprints,dict) else 0
    print('candy_accounts=',candy_accounts)
    print('candy_records=',candy_records)
    print('fingerprint_records=',fingerprint_records)
except FileNotFoundError:
    print('candy_accounts=0')
    print('candy_records=0')
    print('fingerprint_records=0')
PY
```

保存输出到 `$BACKUP_DIR/baseline.txt`。

## 4. 备份（生产变更前必须完成）

```bash
STAMP=$(date +%Y%m%d-%H%M%S)
BACKUP_DIR=/root/backups/model-guard-$NEW_VERSION-$STAMP
mkdir -p "$BACKUP_DIR"

tar -C "$DEPLOY_DIR" -czf "$BACKUP_DIR/data.tar.gz" data
docker exec sub2api-postgres pg_dump -U sub2api -d sub2api -Fc > "$BACKUP_DIR/db-sub2api.dump"
cp "$DEPLOY_DIR/docker-compose.yml" "$BACKUP_DIR/"
cp "$DEPLOY_DIR/.env" "$BACKUP_DIR/"
docker inspect sub2api > "$BACKUP_DIR/container-inspect.json"
docker image inspect "$(docker inspect -f '{{.Config.Image}}' sub2api)" > "$BACKUP_DIR/image-inspect.json"
sha256sum "$BACKUP_DIR"/* > "$BACKUP_DIR/SHA256SUMS"
echo "$BACKUP_DIR" > /root/backups/model-guard-$NEW_VERSION-latest.txt
```

验收：`data.tar.gz`、`db-sub2api.dump` 非空，`SHA256SUMS` 可校验，`docker-compose.yml` 和 `.env` 均已复制。失败则停止，不得继续。

## 5. 合并官方 tag

### 5.1 本地合并

```powershell
git switch feat/model-guard
git status --short
git fetch --no-tags https://github.com/ranxi2001/sub2api.git tag $NEW_TAG
git rev-parse "$NEW_TAG^{commit}"          # 必须等于 NEW_COMMIT
git branch "backup/model-guard-$OLD_VERSION" HEAD
git merge --no-ff --no-commit $NEW_TAG
git diff --name-only --diff-filter=U       # 必须为空
```

### 5.2 冲突处理规则

如有冲突，按以下顺序处理，不得直接选择 `ours` 或 `theirs`：

1. 先保留上游新逻辑。
2. 再把 Model Guard 注册代码重新插入：
   - `backend/internal/server/routes/admin.go`：`/codex-eval` 路由组、账户 codex-eval 路由、巡检设置路由。
   - `frontend/src/components/layout/AppSidebar.vue`：模型哨兵菜单项。
   - `frontend/src/router/index.ts`：`/admin/codex-eval` 路由。
3. 若上游改了同一函数签名，以新签名为准，调整 Model Guard 调用点。
4. 工作流冲突保持 fork 的精简集合：只保留 `ci.yml`、`docker-image.yml`、`security-scan.yml`、`upstream-sync.yml`；上游恢复或新增的其他 workflow 一律删除。
5. i18n 冲突保留 `nav.accountEvaluation`；上游已删除的功能键（例如 `nav.harvestFlow`）不再恢复，上游已删除的页面/API 也不要从 fork 侧补回。
6. 上游删除的功能紧挨 Model Guard 接线时会出现内容冲突（2.25.2 移除 OAuth-429 模型屏蔽，冲突在 `account_repo.go` 的 `BulkUpdate` 和 `scheduler_cache.go` 的 `filterSchedulerExtra`）：跟上游删掉废弃键那一行，保留紧邻的 Model Guard 键，合并后 `git grep` 确认被删常量已无引用。
7. 重新执行 5.3 和 6.1，直到无冲突且测试通过。

### 5.3 合并后检查

```powershell
Get-Content backend\cmd\server\VERSION                    # 应为 NEW_VERSION
Select-String -Path Dockerfile -Pattern 'max-old-space-size' # 必须保留 4096 设置
git diff --cached --name-only | Select-String 'codexeval|model-guard|CodexEval' # 不应有删除
Select-String -Path backend\internal\server\routes\admin.go -Pattern 'codex-eval|codex_eval'
Select-String -Path frontend\src\components\layout\AppSidebar.vue -Pattern '模型哨兵|codex-eval'
Select-String -Path frontend\src\router\index.ts -Pattern 'codex-eval|CodexEval'
(Get-ChildItem .github\workflows).Count                  # 应为 4
Select-String -Path frontend\src\i18n\locales\en\common.ts -Pattern 'accountEvaluation'

# 迁移检查：OLD_UPSTREAM_COMMIT 是上一次升级 merge 提交的第二父提交（git rev-parse "<上一个 merge 提交>^2"）
git diff --name-only "$OLD_UPSTREAM_COMMIT" $NEW_TAG -- backend/migrations
git diff "$OLD_UPSTREAM_COMMIT" $NEW_TAG -- backend/migrations | Select-String -Pattern 'DROP |TRUNCATE|DELETE FROM'
git cat-file -p $NEW_TAG | Select-Object -First 14           # 发布说明里的升级前提醒
```

迁移检查只发现增量变更（加列、建表、建视图）时可以继续；出现 `DROP`、`TRUNCATE`、`DELETE FROM`，或发布说明要求升级前备份时，先按 1.1 的约束在服务器导出受影响的数据，再点击「立即更新镜像」，并把结论写进证据文档。2.25.0 的 `398_drop_user_traffic.sql` 是第一次遇到这种情形。

`DROP` 扫描抓不到改写数据的迁移。2.25.2 的 `399_remove_oauth_429_model_blocks.sql` 用 `UPDATE ... SET extra = extra - '键'` 清除账号里的旧屏蔽记录，是通读迁移正文和发布说明才发现的。每条新增迁移都要通读，并在证据文档里逐条写明「增量」还是「清除数据」。

提交合并：

```powershell
git commit -m "merge: upgrade to Sub2API $NEW_VERSION"
git push origin feat/model-guard
```

## 6. 测试与构建

### 6.1 后端测试与本地 embed 构建

```powershell
cd backend
go test -p 1 -tags unit ./internal/codexeval ./internal/service ./internal/repository ./internal/handler/admin -run 'Test(Codex|ModelGuard|Fingerprint|HasStandalone)' -count=1
go build -p 1 -tags embed -trimpath -o ..\.local-run\model-guard-$NEW_VERSION.exe ./cmd/server
```

两项均通过才可进入镜像构建。失败时停止，不得用旧二进制替代。

### 6.2 服务器镜像构建（仅例外场景）

> 日常发布由 `docker-image.yml` 构建并推送 GHCR，不需要登录服务器。只有 CI 不可用或需要固定本地镜像时才执行本节。

```bash
ssh root@$SERVER
git clone --depth 1 --branch feat/model-guard https://github.com/Ericzhouyihe/sub2api-codex-guard.git "$SOURCE_DIR"
cd "$SOURCE_DIR"
git rev-parse HEAD                         # 应为本地刚推送的 merge commit

docker build \
  --build-arg VERSION="$NEW_VERSION" \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  --build-arg NPM_CONFIG_REGISTRY=https://registry.npmmirror.com \
  -t "$IMAGE_TAG" .
```

前端阶段必须使用 `Dockerfile` 第 44 行的 `NODE_OPTIONS=--max-old-space-size=4096`。构建完成后验证：

```bash
docker run --rm --entrypoint /app/sub2api "$IMAGE_TAG" -version
```

输出必须包含 `Sub2API $NEW_VERSION` 和本次 merge commit。镜像版本号不带 `-modelguard`，后缀只放在镜像 tag 上，避免内置更新检查继续提示同一版本。

## 7. Canary 验证（生产切换前必须通过）

### 7.1 创建隔离数据库与数据目录

```bash
docker exec -i sub2api-postgres dropdb -U sub2api --if-exists "$CANARY_DB"
docker exec -i sub2api-postgres createdb -U sub2api "$CANARY_DB"
docker exec -i sub2api-postgres pg_restore -U sub2api -d "$CANARY_DB" --no-owner --no-privileges < "$BACKUP_DIR/db-sub2api.dump"

rm -rf /root/sub2api-canary-$CANARY_SUFFIX
mkdir -p /root/sub2api-canary-$CANARY_SUFFIX
cp -a "$DEPLOY_DIR/data" /root/sub2api-canary-$CANARY_SUFFIX/data

# 禁止 canary 自动巡检，避免重复消耗上游额度
docker exec -i sub2api-postgres psql -U sub2api -d "$CANARY_DB" -c \
  "UPDATE settings SET value='false' WHERE key='codex_eval_inspection_enabled';"
```

### 7.2 启动 full 角色 canary

> `RUNTIME_ROLE=gateway` 不会注册 admin/Model Guard 路由（`backend/internal/server/router.go:128-144` 提前 return）。Canary 必须用 `full`，并保持巡检关闭。

```bash
# 先启动 canary Redis，避免应用启动时连接失败
docker run -d --name sub2api-canary-redis-$CANARY_SUFFIX \
  --network "$NETWORK" redis:8-alpine redis-server --appendonly yes

docker run -d --name sub2api-canary-$CANARY_SUFFIX \
  --network "$NETWORK" \
  -p 0.0.0.0:$CANARY_PORT:8080 \
  -e DATA_DIR=/app/data \
  -e DATABASE_HOST=sub2api-postgres -e DATABASE_PORT=5432 \
  -e DATABASE_USER=sub2api -e DATABASE_PASSWORD="$POSTGRES_PASSWORD" \
  -e DATABASE_DBNAME="$CANARY_DB" -e DATABASE_SSLMODE=disable \
  -e REDIS_HOST=sub2api-canary-redis-$CANARY_SUFFIX -e REDIS_PORT=6379 \
  -e RUNTIME_ROLE=full \
  -e JWT_SECRET="$JWT_SECRET" -e TOTP_ENCRYPTION_KEY="$TOTP_ENCRYPTION_KEY" \
  -v /root/sub2api-canary-$CANARY_SUFFIX/data:/app/data \
  "$IMAGE_TAG"
```

### 7.3 自动检查

```bash
curl -fsS http://127.0.0.1:$CANARY_PORT/health
for p in \
  /api/v1/admin/codex-eval/state \
  /api/v1/admin/settings/codex-eval-inspection \
  /codex-eval /admin/codex-eval; do
  printf '%s ' "$p"
  curl -s -o /dev/null -w '%{http_code}\n' "http://127.0.0.1:$CANARY_PORT$p"
done
```

预期：`/health` 为 200；两个 API 为 **401**（路由存在）；两个页面为 200。若 API 为 404，说明 Model Guard 路由未注册，停止并检查角色/合并结果。

### 7.4 人工 UI 检查

使用管理员 token 打开 `http://$SERVER:$CANARY_PORT/admin/codex-eval`，逐项确认：

- 左上角版本为 `v$NEW_VERSION`。
- 糖果、指纹、历史记录、巡检设置四个页签都能切换。
- 真实账户、历史条数与基线一致。
- 巡检设置中的分组、模型、强度、间隔、通知配置与生产一致。
- 浏览器控制台无报错，页面无 423 合规拦截。

截图保存到 `.local-run/shots-canary-$NEW_VERSION/`。

### 7.5 Canary 清理

无论通过与否，验证结束后立即删除，避免 full 角色后台任务和巡检消耗额度：

```bash
docker rm -f sub2api-canary-$CANARY_SUFFIX sub2api-canary-redis-$CANARY_SUFFIX
docker exec -i sub2api-postgres dropdb -U sub2api --if-exists "$CANARY_DB"
rm -rf /root/sub2api-canary-$CANARY_SUFFIX
```

## 8. 生产切换

```bash
cd "$DEPLOY_DIR"
cp docker-compose.yml "docker-compose.yml.bak-$NEW_VERSION-$(date +%Y%m%d-%H%M%S)"
sed -i -E "s#image: ghcr.io/(ranxi2001/sub2api|ericzhouyihe/sub2api-codex-guard):.*#image: $IMAGE_TAG#" docker-compose.yml
grep 'image:' docker-compose.yml
docker compose up -d --pull never sub2api
```

只重建 `sub2api` 服务，不要执行 `docker compose down`，不要删除 `data`、`postgres_data`、`redis_data` 卷。

## 9. 生产验证

```bash
docker ps --filter name=^/sub2api$
curl -fsS http://127.0.0.1:8080/health
curl -fsS "$DOMAIN/health"

# 需要管理员 JWT
curl -fsS http://127.0.0.1:8080/api/v1/admin/system/version -H "Authorization: Bearer $JWT"
curl -fsS http://127.0.0.1:8080/api/v1/admin/codex-eval/state -H "Authorization: Bearer $JWT"
curl -fsS http://127.0.0.1:8080/api/v1/admin/settings/codex-eval-inspection -H "Authorization: Bearer $JWT"
curl -fsS http://127.0.0.1:8080/api/v1/auth/me -H "Authorization: Bearer $JWT"

for p in /codex-eval /admin/codex-eval; do
  printf '%s ' "$p"
  curl -s -o /dev/null -w '%{http_code}\n' "http://127.0.0.1:8080$p"
done

docker logs --tail 200 sub2api | grep -Ei 'panic|fatal|error' || true
```

同时打开 `$DOMAIN/admin/codex-eval` 确认页面版本、四个页签、账户和历史记录。截图保存到 `.local-run/shots-prod-$NEW_VERSION/` 和 `.local-run/shots-domain-$NEW_VERSION/`。

**通过条件**：版本正确、健康 200、管理员身份正确、Model Guard API 返回 code 0、巡检设置与升级前一致、页面四页签正常、日志无真实 error/panic、数据计数与基线一致。

## 10. 回滚

### 10.1 仅镜像回滚（首选，通常 1–2 分钟）

```bash
cd "$DEPLOY_DIR"
sed -i -E "s#image: ghcr.io/(ranxi2001/sub2api|ericzhouyihe/sub2api-codex-guard):.*#image: ghcr.io/ericzhouyihe/sub2api-codex-guard:${OLD_VERSION}#" docker-compose.yml
docker compose up -d --pull never sub2api
curl -fsS http://127.0.0.1:8080/health
```

### 10.2 数据回滚（仅确认数据损坏时）

```bash
cd "$DEPLOY_DIR"
docker compose stop sub2api
tar -C "$DEPLOY_DIR" -xzf "$BACKUP_DIR/data.tar.gz"
docker exec -i sub2api-postgres pg_restore -U sub2api -d sub2api --clean --if-exists < "$BACKUP_DIR/db-sub2api.dump"
docker compose up -d sub2api
```

数据回滚会丢失备份时间点之后的写入；执行前必须确认影响范围并再次备份当前状态。

### 10.3 内置更新通道的回退边界（无本地数据）

内置更新通道的 Docker 更新不创建本地回退数据：

- `update_service.go` 在 Docker 模式拒绝页内回退，返回 `DOCKER_UPDATE_UNSUPPORTED`，不会写二进制备份或数据目录。
- Watchtower 以 `WATCHTOWER_CLEANUP=true` 删除旧镜像，服务器不保留旧镜像副本。
- 需要回退时，只把 compose 的 `image:` 指回 GHCR 中已发布的旧版本 tag，再执行 `docker compose up -d sub2api`。
- 不使用 10.2 的数据回滚，除非确认数据损坏并另行评估数据丢失；日常更新不创建 `$BACKUP_DIR`，避免数据越堆越多。

## 11. 证据记录

本次 2.25.4 执行证据见 [docs/upgrades/2.25.4.md](upgrades/2.25.4.md)；历史证据见 [docs/upgrades/2.25.3.md](upgrades/2.25.3.md)、[docs/upgrades/2.25.0.md](upgrades/2.25.0.md)、[docs/upgrades/2.24.3.md](upgrades/2.24.3.md)、 [docs/upgrades/2.24.2.md](upgrades/2.24.2.md)、[docs/upgrades/2.24.1.md](upgrades/2.24.1.md)、[docs/upgrades/2.23.1.md](upgrades/2.23.1.md)、[docs/upgrades/2.21.5.md](upgrades/2.21.5.md)。升级记录不写入真实服务器地址或凭据，必要时只记录变量名。

| Gate | 结果 | 证据 |
|---|---|---|
| 合并无冲突 | PASS/FAIL | `git diff --name-only --diff-filter=U` 输出 |
| Model Guard 注册保留 | PASS/FAIL | routes/sidebar/router grep 输出 |
| Go 测试 | PASS/FAIL | 测试命令与结果 |
| embed 构建 | PASS/FAIL | 二进制路径与 `-version` |
| 镜像构建 | PASS/FAIL | image ID、大小、`-version` |
| Canary API | PASS/FAIL | 401/401/200/200 |
| Canary UI | PASS/FAIL | 截图路径、页签/账户/历史数量 |
| 生产健康 | PASS/FAIL | `/health`、容器状态 |
| 生产版本 | PASS/FAIL | `system/version` |
| Model Guard 生产 API | PASS/FAIL | state/settings 响应 |
| 公网页面 | PASS/FAIL | 域名截图 |
| 回滚路径 | PASS/FAIL | compose 备份路径、旧镜像 tag |
| 内置更新识别 | PASS/FAIL | `has_update=true`、`deployment=docker`、`update_supported=true` |
| 无本地回退数据 | PASS/FAIL | `WATCHTOWER_CLEANUP=true`、Docker 回退接口不可用 |
| 迁移影响评估 | PASS/不可逆 | 新增迁移清单、`DROP` 扫描、发布说明；不可逆时记录导出命令 |

## 12. 常见问题

| 现象 | 根因 | 处理 |
|---|---|---|
| 前端构建 `JavaScript heap out of memory` | 默认 Node heap 不足 | 确认 Dockerfile 第 44 行有 `NODE_OPTIONS=--max-old-space-size=4096`，重建 |
| Canary 的 Model Guard API 404 | 使用了 `RUNTIME_ROLE=gateway` | 改 `full` 重启；gateway 不注册 admin 路由 |
| 本机访问 canary 502 | 系统 HTTP 代理拦截 | 用 `curl --noproxy '*'`；不要用走代理的 `Invoke-RestMethod` |
| 本机连接 18082 失败 | 容器只绑定 `127.0.0.1` | 重建为 `-p 0.0.0.0:18082:8080` |
| Canary 应用启动失败/连不上 Redis | Redis 未先启动，或 `NETWORK` 写错 | 先启动 `sub2api-canary-redis-$CANARY_SUFFIX` 并等 `PONG`；`NETWORK` 用 `docker inspect sub2api` 的实际网络名（本机为 `sub2api-deploy_sub2api-network`） |
| 基线 `history_records=0` 但页面有历史 | `history.json` 不是 `records` 数组 | 用 `results` / `fingerprints` 统计 `candy_accounts`、`candy_records`、`fingerprint_records` |
| 升级后仍提示新版本 | 构建时 `VERSION` 带了后缀 | `VERSION` 用纯版本号，`-modelguard` 只放镜像 tag |
| Go 测试 setup 阶段下载依赖超时 | 上游新增了测试依赖（如 `jsonschema-go`），默认 `proxy.golang.org` 不可达 | 用 `GOPROXY=https://goproxy.cn,direct` 重跑测试与构建（2.24.1 首次发现） |
| 前端 `pnpm install` 长时间卡在最后一个包，或报 `ECONNRESET` | 个别依赖从 registry 以外的地址下载（`xlsx` 来自 `cdn.sheetjs.com`），直连极慢 | 用 `--reporter=append-only` 看进度；其余包会从本地 store 复用，给这一条命令设置 `HTTPS_PROXY` 为本机系统代理后重装（2.25.0 首次发现） |
| 日志出现 `INSTALL_OK`，但依赖并没有装好 | `pnpm install ... \| tail` 后面接 `&&`，管道掩盖了退出码 | 安装和门禁分开执行，逐条检查退出码；不要用带分类标记的 `ls` 判断 `.bin` 里有没有可执行文件（2.25.0 首次发现） |
| `pnpm install` 要求清空 `node_modules` | 本机 `pnpm` 与创建该目录的版本布局不同（Dockerfile 固定 `pnpm@9`） | 先看 `frontend/pnpm-lock.yaml` 有没有变化；没变就不要为了“刷新依赖”去清空，已有的依赖可以直接跑门禁 |
| Git Bash 里 `grep -c '/codex-eval' <文件>` 得到 0，但接线其实还在 | MSYS 把以 `/` 开头的参数当成 POSIX 路径，改写成 Windows 路径后再匹配 | 加 `MSYS_NO_PATHCONV=1`，或改用不带前导斜杠的模式、`git grep`（2.25.3 首次发现） |
| 拉上游 tag 报 `curl 56 schannel: server closed abruptly` / `early EOF` | 本机到 GitHub 的连接中途断开 | 直接重试（2.25.3 首次发现，第 3 次成功） |
| 巡检重复执行/额度异常 | Canary 未关闭巡检 | 启动前把 `codex_eval_inspection_enabled` 设为 `false`，验证后删除 canary |
| 合并后页面入口消失 | 冲突时覆盖了 fork 文件 | 按 5.2 恢复 routes/sidebar/router，重跑测试 |

## 13. 历史执行值：2.21.5

```text
OLD_VERSION=2.21.4
NEW_VERSION=2.21.5
NEW_TAG=v2.21.5
NEW_COMMIT=b9feadf7dd6228a4117e360821d9994b7cc54897
MERGE_COMMIT=fac0046b4
IMAGE_SOURCE_COMMIT=686e754c27565787ea17f7ebdca847c9dd7d63f3
IMAGE_TAG=ghcr.io/ranxi2001/sub2api:2.21.5-modelguard
IMAGE_ID=sha256:6ebbf6aa74431d44b4757b60b7e1ada33906fd50c681614d0d78215448870d8e
SOURCE_DIR=/root/sub2api-mg-src-2.21.5
BACKUP_DIR=/root/backups/model-guard-2.21.5-20261115-202743
COMPOSE_BACKUP=/root/sub2api-deploy/docker-compose.yml.bak-2.21.5-20261115-204657
CANARY_DB=sub2api_canary_215
CANARY_SUFFIX=215
CANARY_PORT=18082
EVIDENCE=docs/upgrades/2.21.5.md
```

> `MERGE_COMMIT` 是上游版本与 fork 的功能合并点；`IMAGE_SOURCE_COMMIT` 是实际构建的 `HEAD`，可能还包含合并后的文档提交。两者都记录，便于审计与回滚。

## 14. 2.23.1 执行值（内置更新通道）

```text
OLD_VERSION=2.22.0
NEW_VERSION=2.23.1
NEW_TAG=v2.23.1
NEW_COMMIT=00e00796080c632188cf9df7912050b144a1a44f
MERGE_COMMIT=30dd5e363
RELEASE_COMMIT=f68a2c088
RELEASE_TAG=v2.23.1
BACKUP_BRANCH=backup/model-guard-2.22.0-20261120-112441
IMAGE_TAG=ghcr.io/ericzhouyihe/sub2api-codex-guard:2.23.1
IMAGE_DIGEST=sha256:391c28a16cc07c1e27a0475b9447fdc58b0c74fb2ccd1bcc2613cd9beacc6fdc
EVIDENCE=docs/upgrades/2.23.1.md
UPDATE_PATH=后台内置检查 -> 立即更新镜像 -> Watchtower 拉取 :latest
SERVER_UPLOAD=none
```

> 本次不执行服务器 SSH/SCP、手动镜像构建或 canary；推送源码后由 GitHub Actions 创建 `v2.23.1` Release 和镜像，生产更新由管理员在后台点击完成。`WATCHTOWER_CLEANUP=true` 会删除旧镜像，服务器不保留本地回退数据。

## 15. 2.24.1 执行值（内置更新通道）

```text
OLD_VERSION=2.23.1
NEW_VERSION=2.24.1
NEW_TAG=v2.24.1
NEW_COMMIT=0755d5ef037f64d203828d0edcc513764b69e1d2
MERGE_COMMIT=7afdd54a4
BACKUP_BRANCH=backup/model-guard-2.23.1-20261122-124147
IMAGE_TAG=ghcr.io/ericzhouyihe/sub2api-codex-guard:2.24.1
IMAGE_DIGEST=sha256:f9a1b0a000e0497fa35c50c60ab1c8883730f5de3310dff542beda2b3de1d77d
EVIDENCE=docs/upgrades/2.24.1.md
UPDATE_PATH=后台内置检查 -> 立即更新镜像 -> Watchtower 拉取 :latest
SERVER_UPLOAD=none
GO_PROXY=GOPROXY=https://goproxy.cn,direct（上游新增测试依赖 jsonschema-go）
```

> 本次同样不执行服务器 SSH/SCP、手动镜像构建或 canary。冲突集中在 OAuth-429 模型屏蔽与巡检 extra 键的三个后端文件，按“保留双方”解决；工作流新增/恢复的 15 个文件全部删除。

## 16. 2.24.2 执行值（内置更新通道）

```text
OLD_VERSION=2.24.1
NEW_VERSION=2.24.2
NEW_TAG=v2.24.2
NEW_COMMIT=991cd30e2910d5eeb6ab55f6b7fc93e9a494e1a6
MERGE_COMMIT=7ac6872ce
BACKUP_BRANCH=backup/model-guard-2.24.1-20261123-102409
IMAGE_TAG=ghcr.io/ericzhouyihe/sub2api-codex-guard:2.24.2
IMAGE_DIGEST=sha256:410608c16d56c11dec46b13511d785b4aadbd1c34552acd4334fc69e25e7dfc4
EVIDENCE=docs/upgrades/2.24.2.md
UPDATE_PATH=后台内置检查 -> 立即更新镜像 -> Watchtower 拉取 :latest
SERVER_UPLOAD=none
```

> 本次冲突全部是「上游修改、fork 已删除」的 workflow 文件（4 个 modify/delete 加 1 个自动新增的 `billing-recovery-fairness.yml`），按惯例全部删除，只保留 fork 的 4 个工作流；后端、i18n、路由均自动合并，Model Guard 接线无需手工干预。数据库迁移 `393_account_usage_financial_rollups.sql` 随新镜像启动自动执行，不保留本地回退数据。

## 17. 2.24.3 执行值（内置更新通道）

```text
OLD_VERSION=2.24.2
NEW_VERSION=2.24.3
NEW_TAG=v2.24.3
NEW_COMMIT=691a36fc39aea4419bc2b114ef76303647dfccd1
MERGE_COMMIT=21c45cf4f
BACKUP_BRANCH=backup/model-guard-2.24.2-20261124-091757
IMAGE_TAG=ghcr.io/ericzhouyihe/sub2api-codex-guard:2.24.3
IMAGE_DIGEST=sha256:7812c3ed8e5a17cfb8dcc4662756ea0f00e8b2325ac1addb2809404b6909e9ac
EVIDENCE=docs/upgrades/2.24.3.md
UPDATE_PATH=后台内置检查 -> 立即更新镜像 -> Watchtower 拉取 :latest
SERVER_UPLOAD=none
```

> 本次冲突同样全部是「上游修改、fork 已删除」的 workflow 文件（3 个 workflow 加 `workflow-archive/README.md`），并删除上游自动新增的 2 个 workflow 和 1 个 `workflow-archive` 文件；只保留 fork 的 4 个工作流。后端、i18n、路由全部自动合并，Model Guard 接线无需手工干预。本版**无新增数据库迁移**。

## 18. 2.25.0 执行值（内置更新通道）

```text
OLD_VERSION=2.24.3
NEW_VERSION=2.25.0
NEW_TAG=v2.25.0
NEW_COMMIT=0fa2cb6ac91effee2a45b933bb44b460f05a8c49
MERGE_COMMIT=edcc1c22f
BACKUP_BRANCH=backup/model-guard-2.24.3-20261124-224339
IMAGE_TAG=ghcr.io/ericzhouyihe/sub2api-codex-guard:2.25.0
IMAGE_DIGEST=sha256:d8ba025685d35d98d4a0b05e25bf7385564f12db260cff7c2199febdb84dbf0a
EVIDENCE=docs/upgrades/2.25.0.md
UPDATE_PATH=后台内置检查 -> 立即更新镜像 -> Watchtower 拉取 :latest
SERVER_UPLOAD=none
MIGRATIONS=394–398（增量）；398_drop_user_traffic.sql（不可逆）
PRE_UPDATE_EXPORT=user_traffic_events, user_traffic_hourly
```

> 本次冲突仍然只是「上游修改、fork 已删除」的 workflow 文件（4 个 modify/delete 加 3 个上游新增的 workflow），全部删除，只保留 fork 的 4 个工作流；后端、i18n、路由全部自动合并，Model Guard 接线无需手工干预。
>
> **本版含不可逆迁移。** 上游移除「用户流量」功能，`398_drop_user_traffic.sql` 会删除 `user_traffic_events` 和 `user_traffic_hourly`，回退镜像无法恢复；内置通道不建快照。点击「立即更新镜像」之前，必须先在服务器导出这两张表（命令见证据文档）。

## 19. 2.25.3 执行值（内置更新通道）

```text
OLD_VERSION=2.25.0
NEW_VERSION=2.25.3
NEW_TAG=v2.25.3
NEW_COMMIT=c95ce812c7b13f6fc9b3873a82fd9859ffb0e513
MERGE_COMMIT=a86309ef6
BACKUP_BRANCH=backup/model-guard-2.25.0-20261126-210637
IMAGE_TAG=ghcr.io/ericzhouyihe/sub2api-codex-guard:2.25.3
IMAGE_DIGEST=sha256:449fc9b0d9c97a8cb033382abfb6dccb9625db731a709c1a8e020176873c27dd
EVIDENCE=docs/upgrades/2.25.3.md
UPDATE_PATH=后台内置检查 -> 立即更新镜像 -> Watchtower 拉取 :latest
SERVER_UPLOAD=none
MIGRATIONS=399_channel_monitor_v3_component_models、399_remove_oauth_429_model_blocks、400_quality_429_rounds（一次跨 v2.25.1–v2.25.3）
PRE_UPDATE_EXPORT=accounts.extra 中的 oauth_429_model_blocks（上游已废弃，仅在需要留档时导出）
```

> 本次一次跨过上游 v2.25.1、v2.25.2、v2.25.3。除惯例删除的 CI 文件（1 个 modify/delete 加 1 个上游新增 workflow）外，后端有两处内容冲突：上游移除 OAuth-429 模型屏蔽，正好挨着 Model Guard 的巡检键（`account_repo.go`、`scheduler_cache.go`）。按「跟上游删废弃键、保留 Model Guard 键」解决，`admin_account.go` 和 `openai_model_retirement.go` 自动合并。
>
> **本版有一条清除数据的迁移。** `399_remove_oauth_429_model_blocks.sql` 会从 `accounts.extra` 移除已废弃的 `oauth_429_model_blocks`，回退镜像无法恢复；它不是 `DROP`，关键字扫描发现不了。Model Guard 的 `codex_eval_inspection_blocked_models` 是另一个键，不受影响。生产若仍在 2.24.3 或更早，还会连带执行 2.25.0 的删表迁移，先看第 18 节。

## 20. 2.25.4 执行值（内置更新通道）

```text
OLD_VERSION=2.25.3
NEW_VERSION=2.25.4
NEW_TAG=v2.25.4
NEW_COMMIT=73b3dc4b9ce915be6dcf18a78e7ea31e6f62339d
MERGE_COMMIT=2b623678c
RELEASE_COMMIT=1007c3164
BACKUP_BRANCH=backup/model-guard-2.25.3-20261127-193946
IMAGE_TAG=ghcr.io/ericzhouyihe/sub2api-codex-guard:2.25.4
IMAGE_DIGEST=sha256:64ca724ddd6952b8ecb9128513f500c487ec47eae38bb1793fe4a07bbc7781bf
EVIDENCE=docs/upgrades/2.25.4.md
UPDATE_PATH=后台内置检查 -> 立即更新镜像 -> Watchtower 拉取 :latest
SERVER_UPLOAD=none
MIGRATIONS=400_drop_request_diagnostics.sql（不可逆，删除两张诊断表）
PRE_UPDATE_EXPORT=request_connection_traces、request_timing_details（仅在需要留档时导出）
```

> 本次只有 11 个上游提交，冲突仍是「上游修改、fork 已删除」的 5 个 workflow，全部删除后保留 fork 的 4 个工作流；后端自动合并，Model Guard 接线无需手工干预。前端全量 `pnpm vitest run` 633 个文件通过。
>
> **本版含不可逆删表迁移。** `400_drop_request_diagnostics.sql` 会删除 `request_timing_details` 和 `request_connection_traces`，回退镜像无法恢复；内置通道不建快照。Model Guard 巡检数据（`data/` 卷、`codex_eval_inspection_*`、`accounts.extra` 的 `codex_eval_inspection_blocked_models`）不受影响。上游另附 `deploy/retire-usage-billing-events.sql`，不在迁移目录内，不会被自动执行。
