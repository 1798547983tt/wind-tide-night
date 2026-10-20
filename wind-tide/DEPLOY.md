# 风与潮之夜 · 部署手册

适用于一台装好 Docker 和 Docker Compose 的 Linux 服务器（站点用的是阿里云 Alibaba Cloud Linux 3）。除特别说明外，命令都在服务器上用 root 执行，工作目录是 `/opt/wind-tide`。`<服务器IP>`、`<域名>` 这类尖括号要换成实际值；`<域名>` 指网站用的完整域名，例如 `api.example.com`。

## 0. 整体结构

- **镜像**：代码推到 GitHub 的 `wind-tide` 分支后，GitHub Actions 先跑测试，通过后构建镜像 `ghcr.io/1798547983tt/wind-tide-night:latest`。服务器只拉取，不编译（2G 内存编译前端会爆）。
- **服务器上两个容器**：
  - `caddy`：对外开 80/443，自动申请和续期 HTTPS 证书，并直接提供插画文件。
  - `site`：网站本体，只在容器网络里开 3000，外面访问不到。
- **目录**：

  | 路径 | 放什么 |
  |---|---|
  | `/opt/wind-tide/data` | 数据库 `one-api.db`、日志、Discord 头像缓存 |
  | `/opt/wind-tide/art` | 首页和开幕用的插画（不进仓库，见 docs/adr/0003） |
  | `/opt/wind-tide/backups` | 每日数据库备份，保留 7 天 |
  | `/opt/wind-tide/.env` | 密钥和站点地址，只在服务器上，不进仓库 |

## 1. 服务器一次性准备

```bash
# 2G 交换空间：内存只有 2G，防止高峰时被系统杀进程
fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile
grep -q '^/swapfile ' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab

mkdir -p /opt/wind-tide/data /opt/wind-tide/art /opt/wind-tide/backups
# 数据库和备份里有 API 密钥和 Discord 密钥，只让 root 能读
chmod 700 /opt/wind-tide/data /opt/wind-tide/backups

# 备份用的 SQLite 小镜像（系统自带的 SQLite 太旧）
docker build -t wt-sqlite - <<'EOF'
FROM alpine:3.24
RUN apk add --no-cache sqlite
EOF
```

## 2. 首次部署

还没有域名、先用服务器 IP 上线的话，按本节做的同时看第 7 节。

0. **域名解析**：在域名服务商那里添加一条 A 记录，主机记录填子域名（例如 `api`），记录值填 `<服务器IP>`。解析生效后再往下做。本地电脑开着代理时查到的地址可能不准，所以在服务器上确认：

   ```bash
   curl -s "https://dns.google/resolve?name=<域名>&type=A"   # 结果里应有 <服务器IP>
   ```

1. **上传部署文件**（在本地仓库根目录执行）：

   ```bash
   scp wind-tide/deploy/compose.yaml wind-tide/deploy/Caddyfile wind-tide/deploy/backup.sh wind-tide/deploy/.env.example root@<服务器IP>:/opt/wind-tide/
   ```

2. **填写 `.env`**（服务器上）：

   ```bash
   cd /opt/wind-tide && cp .env.example .env && chmod 600 .env
   openssl rand -hex 32   # 执行两次，分别填进 SESSION_SECRET 和 CRYPTO_SECRET
   vi .env
   ```

   - `SITE_ADDRESS` 和 `SESSION_COOKIE_TRUSTED_URL` 都填 `https://<域名>`。
   - `SITE_HOST` 填 `<域名>`，不带 `https://`。浏览器打开 IP 网址时不会说明自己要访问哪个站点，Caddy 靠这一项决定出示哪张证书。
   - `WT_OWNER_DISCORD_ID` 填站长的 Discord 用户 ID。
   - `CRYPTO_SECRET` 定下后不要再改，否则加密存储的数据会解不开。
   - `WT_CPA_URL` 和 `WT_CPA_MANAGEMENT_KEY` 可选，接入号池（捐赠账号功能）时才填，见第 9 节；不填时功能隐藏。

3. **上传插画**（本地执行；插画由本地素材流水线生成在 `wind-tide/.serve/art/`）：

   ```bash
   scp -r wind-tide/.serve/art/. root@<服务器IP>:/opt/wind-tide/art/
   ```

4. **让服务器能拉镜像**：GitHub 上新发布的镜像默认是私有的。二选一：
   - 推荐：GitHub 个人主页 → Packages → `wind-tide-night` → Package settings → Change visibility → Public。源码本来就公开，镜像公开没有额外风险。
   - 或者在服务器上 `docker login ghcr.io`，密码处粘贴一个只带 `read:packages` 权限的 classic token。

5. **启动**：

   ```bash
   cd /opt/wind-tide
   # 先检查配置：.env 少填一项、Caddyfile 写错，都会在这里报出来
   docker compose config -q
   docker compose run --rm --no-deps caddy caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
   docker compose pull && docker compose up -d
   docker compose ps                     # 两个容器都应是 running，site 稍后变 healthy
   curl -s https://<域名>/api/status       # 应看到 "success":true
   ```

   证书申请失败时看 `docker compose logs caddy`，处理办法见第 8 节。

6. **写入站点地址**：Discord 登录的回调地址用它。（应急登录链接不用它，用的是 `.env` 里的 `SITE_ADDRESS`。）

   ```bash
   docker run --rm -i -v /opt/wind-tide/data:/data wt-sqlite sqlite3 /data/one-api.db <<'EOF'
   INSERT INTO options ("key", value) VALUES ('ServerAddress', 'https://<域名>')
     ON CONFLICT("key") DO UPDATE SET value = excluded.value;
   EOF
   docker compose restart site
   ```

7. **设置 Discord 登录**：见第 3 节。

8. **开启每日备份**：

   ```bash
   chmod +x /opt/wind-tide/backup.sh
   (crontab -l 2>/dev/null; echo '15 4 * * * /opt/wind-tide/backup.sh >> /opt/wind-tide/backups/backup.log 2>&1') | crontab -
   /opt/wind-tide/backup.sh && ls -l /opt/wind-tide/backups   # 先手动跑一次确认能用
   ```

## 3. Discord 登录设置

1. **登记回调地址**：Discord 开发者后台 → 你的应用 → OAuth2 → Redirects，添加 `https://<域名>/oauth/discord`，保存。同一页的「Public Client（公共客户端）」保持关闭：网站在服务器上保管 Client Secret，不需要它。
2. **填 Client ID 和 Client Secret**：
   - 平时的做法：站长登录后台 → 系统设置 → 认证 → Discord，填好保存。
   - Discord 还没配好、站长登录不进去时：先用第 4 节的应急登录进后台，再按上一条填写。
   - 不要把 Client Secret 写进任何文件、聊天记录或仓库。
3. **怀疑密钥泄露时**：在 Discord 开发者后台点 Reset Secret，把新密钥按第 2 条填进后台。旧密钥会立即失效。

## 4. 应急登录

Discord 登录坏了、或站长需要先进后台配置时使用：

```bash
cd /opt/wind-tide && docker compose exec site /new-api wt emergency-login
```

它会打印一条链接，形如 `https://<域名>/emergency-login#token=...`（前面可能多一行 `[SYS]` 开头的启动日志，忽略它，只复制带 `#token=` 的那一行）：

- 链接里的地址取自 `.env` 的 `SITE_ADDRESS`，后台的设置改不了它。这样即使有人短暂拿到后台权限、改了「服务器地址」，也没法把应急链接引到别的网站去。
- 10 分钟内有效，只能用一次；再生成一条，旧的就作废。
- 站长开了两步验证或 Passkey 的，用链接登录时仍需验证。
- 链接等同于站长密码，不要发给任何人。

## 5. 更新网站

代码推到 `wind-tide` 分支并且 Actions 全部通过后：

```bash
cd /opt/wind-tide && docker compose pull site && docker compose up -d site
docker image prune -f   # 清掉旧镜像，省磁盘
```

## 6. 备份与恢复

- 备份文件：`/opt/wind-tide/backups/one-api-YYYYMMDD.db.gz`，每天 4:15（北京时间）生成，保留 7 天。
- 想留在服务器以外：定期把整个 `backups` 目录下载到本地。

恢复到某一天：

```bash
cd /opt/wind-tide
docker compose stop site
cp data/one-api.db data/one-api.db.before-restore      # 先留一份现状
gunzip -c backups/one-api-20261015.db.gz > data/one-api.db
rm -f data/one-api.db-wal data/one-api.db-shm
docker compose start site
```

## 7. 没有域名时，或者换域名

**暂时只用服务器 IP**：第 2 节里所有 `<域名>` 都换成 `<服务器IP>`（`SITE_HOST` 填 IP 本身），再把 `Caddyfile` 里注释掉的 `tls { ... profile shortlived ... }` 那几行取消注释（删掉每行开头的 `#`）。Let's Encrypt 只给 IP 签 6 天有效的短期证书，Caddy 会自动续期。`Strict-Transport-Security` 那一行留着即可，浏览器对 IP 地址会忽略它。

**换域名**（包括从 IP 换到域名）：

1. 新域名按第 2 节第 0 步做好解析。
2. 改 `.env`：`SITE_ADDRESS` 和 `SESSION_COOKIE_TRUSTED_URL` 都改成 `https://<新域名>`，`SITE_HOST` 改成 `<新域名>`。
3. 从 IP 换过来的，把 `Caddyfile` 里取消注释的 `tls { ... }` 那几行重新注释掉。
4. 后台 → 系统设置里的「服务器地址」改成 `https://<新域名>`（也可以用第 2 节第 6 步的命令）。
5. Discord 开发者后台添加回调 `https://<新域名>/oauth/discord`。
6. 生效：`cd /opt/wind-tide && docker compose up -d`。所有人需要重新登录一次，旧地址不再能打开。

`Caddyfile` 里的 `Strict-Transport-Security` 会让浏览器一年内只用 HTTPS 访问这个域名。所以一旦某个浏览器用 HTTPS 打开过这个域名，第 8 节的 HTTP 临时方案在这个域名上就不再可用，只能用 IP 访问。

## 8. 常见问题

- **HTTPS 证书申请不下来**：先确认阿里云防火墙放行了 80 和 443，再看 `docker compose logs caddy`。实在不行先跑 HTTP，用 IP 访问：`.env` 里 `SITE_ADDRESS=http://<服务器IP>`、`SESSION_COOKIE_SECURE=false`、`SESSION_COOKIE_TRUSTED_URL=` 留空（这一行要保留，只是等号后面不写），`ServerAddress` 也改成 `http://...`，然后 `docker compose up -d`。HTTP 下登录信息是明文传输，只适合临时排查。**HTTPS 修好后，这三处要一起改回去**：`SITE_ADDRESS` 和 `SESSION_COOKIE_TRUSTED_URL` 填回 `https://...`，`SESSION_COOKIE_SECURE=true`。只改回 `SITE_ADDRESS` 的话，站点会在 HTTPS 下继续用不安全的 Cookie 设置，而且不会报错提醒。
- **证书申请成功了，浏览器却打不开（提示连接不安全或无法建立安全连接）**：检查 `.env` 里 `SITE_HOST` 是否就是 `SITE_ADDRESS` 去掉 `https://` 后的那部分。两者不一致时，用 IP 访问的浏览器拿不到证书。改好后执行 `docker compose up -d`。
- **site 一直不是 healthy**：`docker compose logs --tail 100 site` 看报错。常见原因是 `.env` 少填了某一项，compose 启动时会直接提示缺哪个变量。
- **插画不显示**：确认 `/opt/wind-tide/art/manifest.json` 存在，浏览器访问 `https://<域名>/art/manifest.json` 能打开。
- **磁盘快满**：`docker system df` 看占用，`docker image prune -f` 清旧镜像。

## 9. 接入号池（可选）

捐赠账号功能要连一个同机部署的 CLIProxyAPI（CPA）。CPA 本身的部署不在本文范围；它要和站点容器在同一个 Docker 网络里（本站是 `wind-tide_edge`），并开启 `management.allow-remote`。

1. `.env` 追加两行（密钥是 CPA 配置 `management.secret-key` 对应的明文，不要写进任何文档或聊天）：

   ```
   WT_CPA_URL=http://cliproxyapi:8317
   WT_CPA_MANAGEMENT_KEY=<CPA 管理密钥明文>
   ```

2. 先 `docker compose config` 看变量有没有带进 `site` 服务，再 `docker compose pull site && docker compose up -d site`。
3. 验证：`curl -s https://<域名>/api/status` 里 `wt_donation_enabled` 为 `true`；登录后侧栏「个人」里出现「捐赠账号」；打开页面不报"无法连接号池"。
4. 页面报号池不可用时，先看 `docker compose logs --tail 100 site` 里有没有 CPA 返回 401 的记录，**不要反复刷新**：CPA 对错误密钥有 5 次/30 分钟的封禁，封禁期间正确的密钥也会被拒。
5. 回退：删掉 `.env` 里这两行，`docker compose up -d site`。功能隐藏，表 `wt_donations` 留着无害。

## 10. 接入 AIS playground 号池（可选）

AIS playground 标签和「算力池」页面要连一个同机部署的 aistudio-api（下称 AIS），它用一个受控浏览器拿成员捐的 Google AI Studio 登录 Cookie 去调模型。接口约定见 `API.md` §15。

### 10.1 部署 AIS 本身

放在 `/opt/aistudio-api`，和站点同一个 Docker 网络（`wind-tide_edge`），不发布任何端口。`compose.yaml` 的要点（镜像请按 digest 固定，升级时改 digest 再 `docker compose up -d`）：

```yaml
services:
  aistudio-api:
    image: ghcr.io/deconstructedcube/aistudio-api:latest@sha256:<digest>
    restart: unless-stopped
    env_file: .env                      # 只放 AISTUDIO_WEB_PASSWORD=<控制台密码>
    environment:
      AISTUDIO_DATA_DIR: /app/data
      AISTUDIO_ACCOUNTS_DIR: /app/data/accounts
      AISTUDIO_CONFIG_FILE: /app/data/config.yaml
      AISTUDIO_BROWSER_IDLE_TIMEOUT: "600"   # 10 分钟没请求就关掉浏览器省内存
    volumes: ["./data:/app/data"]
    networks: [edge]
    mem_limit: 768m
    memswap_limit: 1536m
    cpus: 1.5
    shm_size: 256m
    pids_limit: 512
    security_opt: ["no-new-privileges:true"]
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS http://127.0.0.1:8080/health || exit 1"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 60s
networks:
  edge:
    external: true
    name: wind-tide_edge
```

1. `openssl rand -hex 24` 生成控制台密码，写进 `.env`（权限 600）。它同时是 AIS 管理接口的密码，站点用它调号池；不要写进任何文档或聊天。
2. `docker compose up -d`，等 `docker compose ps` 显示 healthy。
3. 在 AIS 控制台（下一步接入 Caddy 后访问）或用管理接口 `POST /api-keys` 新建一个中转密钥（名字随意，例如 `wind-tide relay`），存到 `/opt/aistudio-api/relay.key`（600）。站点的渠道用它调 AIS。
4. 服务器内存紧张时（本站 2 GB），上面的 768 MB 上限和 10 分钟空闲回收是必须的；空闲时 AIS 只占 40 MB 左右，开浏览器时会冲到几百 MB。

### 10.2 控制台域名

在站点的 `Caddyfile` 末尾加一个只在服务器上存在的块（仓库里的 `deploy/Caddyfile` 不含它，记在服务器的 `LOCAL-CHANGES.md`），先去 DNS 加 `ais` 的 A 记录指向服务器：

```caddyfile
ais.example.com {
    # 和 cpa 块一样的安全响应头
    @relay path /v1beta/*
    respond @relay 404          # 中转接口只允许站点容器内网访问
    handle {
        reverse_proxy aistudio-api:8080
    }
}
```

`docker compose exec -T caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile </dev/null`。证书拿到后 `https://ais.example.com/health` 应返回 `{"status":"ok"}`，`/v1beta/models` 应是 404。控制台里能看到成员捐的账号，**不要在控制台手动删账号**：站点以为它们还在；要删就在站点页面上移除。

### 10.3 接到站点

1. `.env` 追加三行（密码来自 `/opt/aistudio-api/.env`，密钥来自 `relay.key`，都不要打印）：

   ```
   WT_AIS_URL=http://aistudio-api:8080
   WT_AIS_PASSWORD=<AIS 控制台密码>
   WT_AIS_API_KEY=<relay.key 的内容>
   ```

2. `docker compose config | grep WT_AIS_` 确认三个变量进了 `site`，再 `docker compose pull site && docker compose up -d site`。
3. 验证：`curl -s https://<域名>/api/status` 里 `wt_ais_enabled` 为 `true`；后台「渠道」里出现标签 `wt-ais` 的渠道 `AIS playground`（站点每次启动都会把它和分组 `AIS PG`、模型倍率改回约定值，见 `API.md` §15.2）；登录后「捐赠账号」页多出 AIS playground 和算力池两个标签。
4. 第一次有人捐成功后，用站长账号（不计次数）各调一次 `AIS PG-gemini-4-argon` 和 `AIS PG-gemini-3.8-flash`，确认 AI Studio 那边确实有 `gemini-4-argon` 和 `gemini-3.8-flash`；没有的话改 `.env` 不解决问题，要改代码里的模型表（`WtAisModels`）。
5. 回退：删掉这三行，`docker compose up -d site`。标签和页面隐藏，渠道、分组和三张表留着无害；要彻底清理再手动删渠道。

## 11. 接入 BUILD 号池（可选）

BUILD 标签和算力池页的 BUILD 水箱要连一个同机部署的 AIStudioToAPI（下称 BUILD）：它用受控浏览器打开 AI Studio 的 Build 页面，拿成员捐的 Google 登录去调模型，对外是 OpenAI / Gemini 兼容接口。站点只用它的控制台接口（登录、`/api/status`、`/api/files`、`/api/accounts/*`）和中转接口，约定见 `API.md` §15。

### 11.1 部署 BUILD 本身

放在 `/opt/aistudio-to-api`，和站点同一个 Docker 网络（`wind-tide_edge`），不发布任何端口。`compose.yaml` 要点（镜像按 digest 固定，升级时改 digest 再 `docker compose up -d`）：

```yaml
services:
  aistudio-to-api:
    image: ghcr.io/ibuhub/aistudio-to-api:latest@sha256:<digest>
    restart: unless-stopped
    env_file: .env                 # WEB_CONSOLE_PASSWORD=<控制台密码>、API_KEYS=<中转密钥>
    environment:
      TZ: Asia/Shanghai
      HOST: 0.0.0.0
      PORT: "7860"
      MAX_CONTEXTS: "2"            # 同时开着的浏览器上下文数
      SWITCH_ON_USES: "10"         # 每 10 次请求换下一个账号，让捐来的号轮着用
      CHECK_UPDATE: "false"
      ENABLE_AUTH_UPDATE: "true"
      STREAMING_MODE: real
      RATE_LIMIT_MAX_ATTEMPTS: "5"
    volumes:
      - ./configs/auth:/app/configs/auth   # 成员捐的登录文件 auth-<n>.json，目录 700
      - ./data:/app/data
    networks: [edge]
    mem_limit: 1536m
    memswap_limit: 2560m
    cpus: 1.5
    shm_size: 512m
    pids_limit: 1024
    security_opt: ["no-new-privileges:true"]
    logging: {driver: json-file, options: {max-size: 10m, max-file: "3"}}
networks:
  edge:
    external: true
    name: wind-tide_edge
```

1. `openssl rand -hex 24` 生成控制台密码、`openssl rand -hex 32` 生成中转密钥，分别写进 `.env` 的 `WEB_CONSOLE_PASSWORD` 和 `API_KEYS`（权限 600），中转密钥另存一份到 `relay.key`（600）。两者都不要写进任何文档或聊天。
2. `mkdir -p configs/auth data && chmod 700 configs/auth data`，然后 `docker compose up -d`，等 `docker compose ps` 显示 healthy（镜像自带健康检查）。
3. 轮询规则是镜像自己的，站点改不了：每 `SWITCH_ON_USES` 次请求、连续 3 次失败、或上游 429/503 时换下一个账号；没有"每次请求随机选号"的选项。
4. 内存：开浏览器时会占到几百 MB，上面的 1.5 GB 上限按 4 GB 内存的机器设；2 GB 的机器上它和 AIS 只能二选一。

### 11.2 控制台域名

同 §10.2，在服务器的 `Caddyfile` 末尾加一个块（先去 DNS 加 `build` 的 A 记录指向服务器，再加块；记录没生效前 Caddy 申请不到证书）：

```caddyfile
build.example.com {
    # 和 ais 块一样的安全响应头
    @relay path /v1/* /v1beta/*
    respond @relay 404          # 中转接口只允许站点容器内网访问
    handle {
        reverse_proxy aistudio-to-api:7860
    }
}
```

`docker compose exec -T caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile </dev/null`。证书拿到后 `https://build.example.com/health` 应返回 200，`/v1/models` 应是 404。控制台里能看到成员捐的登录文件（账号名 `wt-<成员 id>-<随机>`），**不要在控制台手动删它们**：要删就在站点页面上移除。

### 11.3 接到站点

1. `.env` 追加三行（密码来自 `/opt/aistudio-to-api/.env`，密钥来自 `relay.key`，都不要打印）：

   ```
   WT_BUILD_URL=http://aistudio-to-api:7860
   WT_BUILD_PASSWORD=<BUILD 控制台密码>
   WT_BUILD_API_KEY=<relay.key 的内容>
   ```

2. 站点的 `compose.yaml` 要把这三个变量传给 `site`（仓库里 `wind-tide/deploy/compose.yaml` 已带；按老版本部署的服务器要照着补三行），`docker compose config | grep -c WT_BUILD_` 应为 3，再 `docker compose pull site && docker compose up -d site`。
3. 验证：`curl -s https://<域名>/api/status` 里 `wt_build_enabled` 为 `true`；后台「渠道」里出现标签 `wt-build` 的渠道 `BUILD`（分组 `BUILD`、模型 `Build-gemini-3.8-flash`，站点每次启动都会把它改回约定值，见 `API.md` §15.2）；登录后「捐赠账号」页多出 BUILD 标签，算力池页多出 BUILD 水箱。
4. 第一次有人捐成功后，用站长账号（不计次数）调一次 `Build-gemini-3.8-flash`，确认控制台那边能用 `gemini-3.8-flash`；要换模型只能改代码里的模型表（`WtAisModels`）。
5. 回退：删掉这三行，`docker compose up -d site`。标签和水箱隐藏，渠道、分组和表留着无害；要彻底清理再手动删渠道。
