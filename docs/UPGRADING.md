# Sub2API 升级运行手册（保留 Model Guard）

本手册用于把本仓库的 `feat/model-guard` 分支升级到 `ranxi2001/sub2api` 的任意新版本，并在不影响现有用户、账户、巡检设置和历史记录的前提下完成生产切换。

第一次执行对象：**2.21.4 → 2.21.5**。以后升级只需要替换第 1 节的输入值。

## 1. 输入值

| 变量 | 含义 | 2.21.5 示例 |
|---|---|---|
| `OLD_VERSION` | 当前生产版本 | `2.21.4` |
| `NEW_VERSION` | 目标官方版本 | `2.21.5` |
| `NEW_TAG` | 官方 Git tag | `v2.21.5` |
| `NEW_COMMIT` | tag 指向的 commit | `b9feadf7dd6228a4117e360821d9994b7cc54897` |
| `IMAGE_TAG` | 新镜像 tag | `ghcr.io/ranxi2001/sub2api:2.21.5-modelguard` |
| `SERVER` | 生产服务器 | `186.241.68.238` |
| `DOMAIN` | 公网域名 | `https://oxapi.dpdns.org` |
| `DEPLOY_DIR` | 生产 compose 目录 | `/root/sub2api-deploy` |
| `SOURCE_DIR` | 服务器源码目录 | `/root/sub2api-mg-src-2.21.5` |
| `BACKUP_DIR` | 本次备份目录 | `/root/backups/model-guard-2.21.5-<时间戳>` |
| `CANARY_DB` | canary 数据库 | `sub2api_canary_215` |
| `CANARY_PORT` | canary 宿主端口 | `18082` |

> 不要把 SSH 密码、JWT、数据库密码写入仓库。命令中的 `$JWT`、`$POSTGRES_PASSWORD` 等由当前 shell 环境提供。

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
    print('history_records=', len(d) if isinstance(d,list) else len(d.get('records',[])))
except FileNotFoundError:
    print('history_records=0 (no file)')
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
4. 重新执行 5.3 和 6.1，直到无冲突且测试通过。

### 5.3 合并后检查

```powershell
Get-Content backend\cmd\server\VERSION                    # 应为 NEW_VERSION
Select-String -Path Dockerfile -Pattern 'max-old-space-size' # 必须保留 4096 设置
git diff --cached --name-only | Select-String 'codexeval|model-guard|CodexEval' # 不应有删除
Select-String -Path backend\internal\server\routes\admin.go -Pattern 'codex-eval|codex_eval'
Select-String -Path frontend\src\components\layout\AppSidebar.vue -Pattern '模型哨兵|codex-eval'
Select-String -Path frontend\src\router\index.ts -Pattern 'codex-eval|CodexEval'
```

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

### 6.2 服务器镜像构建

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

rm -rf /root/sub2api-canary-$NEW_VERSION
mkdir -p /root/sub2api-canary-$NEW_VERSION
cp -a "$DEPLOY_DIR/data" /root/sub2api-canary-$NEW_VERSION/data

# 禁止 canary 自动巡检，避免重复消耗上游额度
docker exec -i sub2api-postgres psql -U sub2api -d "$CANARY_DB" -c \
  "UPDATE settings SET value='false' WHERE key='codex_eval_inspection_enabled';"
```

### 7.2 启动 full 角色 canary

> `RUNTIME_ROLE=gateway` 不会注册 admin/Model Guard 路由（`backend/internal/server/router.go:128-144` 提前 return）。Canary 必须用 `full`，并保持巡检关闭。

```bash
docker run -d --name sub2api-canary-$NEW_VERSION \
  --network sub2api-network \
  -p 0.0.0.0:$CANARY_PORT:8080 \
  -e DATA_DIR=/app/data \
  -e DATABASE_HOST=sub2api-postgres -e DATABASE_PORT=5432 \
  -e DATABASE_USER=sub2api -e DATABASE_PASSWORD="$POSTGRES_PASSWORD" \
  -e DATABASE_DBNAME="$CANARY_DB" -e DATABASE_SSLMODE=disable \
  -e REDIS_HOST=sub2api-canary-redis-$NEW_VERSION -e REDIS_PORT=6379 \
  -e RUNTIME_ROLE=full \
  -e JWT_SECRET="$JWT_SECRET" -e TOTP_ENCRYPTION_KEY="$TOTP_ENCRYPTION_KEY" \
  -v /root/sub2api-canary-$NEW_VERSION/data:/app/data \
  "$IMAGE_TAG"
```

Redis canary：

```bash
docker run -d --name sub2api-canary-redis-$NEW_VERSION \
  --network sub2api-network redis:8-alpine redis-server --appendonly yes
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
docker rm -f sub2api-canary-$NEW_VERSION sub2api-canary-redis-$NEW_VERSION
docker exec -i sub2api-postgres dropdb -U sub2api --if-exists "$CANARY_DB"
rm -rf /root/sub2api-canary-$NEW_VERSION
```

## 8. 生产切换

```bash
cd "$DEPLOY_DIR"
cp docker-compose.yml "docker-compose.yml.bak-$NEW_VERSION-$(date +%Y%m%d-%H%M%S)"
sed -i "s#image: ghcr.io/ranxi2001/sub2api:.*#image: $IMAGE_TAG#" docker-compose.yml
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
sed -i "s#image: ghcr.io/ranxi2001/sub2api:.*#image: ghcr.io/ranxi2001/sub2api:${OLD_VERSION}-modelguard#" docker-compose.yml
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

## 11. 证据记录

在 `specs/001-sub2api-upgrade-runbook/tasks.md` 或独立升级记录中填写：

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

## 12. 常见问题

| 现象 | 根因 | 处理 |
|---|---|---|
| 前端构建 `JavaScript heap out of memory` | 默认 Node heap 不足 | 确认 Dockerfile 第 44 行有 `NODE_OPTIONS=--max-old-space-size=4096`，重建 |
| Canary 的 Model Guard API 404 | 使用了 `RUNTIME_ROLE=gateway` | 改 `full` 重启；gateway 不注册 admin 路由 |
| 本机访问 canary 502 | 系统 HTTP 代理拦截 | 用 `curl --noproxy '*'`；不要用走代理的 `Invoke-RestMethod` |
| 本机连接 18082 失败 | 容器只绑定 `127.0.0.1` | 重建为 `-p 0.0.0.0:18082:8080` |
| 升级后仍提示新版本 | 构建时 `VERSION` 带了后缀 | `VERSION` 用纯版本号，`-modelguard` 只放镜像 tag |
| 巡检重复执行/额度异常 | Canary 未关闭巡检 | 启动前把 `codex_eval_inspection_enabled` 设为 `false`，验证后删除 canary |
| 合并后页面入口消失 | 冲突时覆盖了 fork 文件 | 按 5.2 恢复 routes/sidebar/router，重跑测试 |

## 13. 本次 2.21.5 执行值

```text
OLD_VERSION=2.21.4
NEW_VERSION=2.21.5
NEW_TAG=v2.21.5
NEW_COMMIT=b9feadf7dd6228a4117e360821d9994b7cc54897
IMAGE_TAG=ghcr.io/ranxi2001/sub2api:2.21.5-modelguard
SOURCE_DIR=/root/sub2api-mg-src-2.21.5
CANARY_DB=sub2api_canary_215
CANARY_PORT=18082
```
