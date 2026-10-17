# 风与潮之夜 · 前后端接口约定

前端和后端都以本文件为准。新增后端代码放在带 `wt_` 前缀的新文件里（`model/wt_*.go`、`controller/wt_*.go`、`service/wt_*.go`、`router/wt_*.go`）。新路由挂在 `/api/wt/` 下。响应沿用上游格式 `{success, message, data}`。金额字段一律是原始 quota，由前端按 `quota_per_unit` 换算成美元。`message` 是英文原文，前端按它翻译（第 13 节列出 wind-tide 自己的消息）。

## 1. 部署配置（环境变量）与启动

| 变量 | 含义 |
|---|---|
| `WT_OWNER_DISCORD_ID` | 必填。站长的 Discord 用户 ID（17 到 20 位数字）。缺失或格式不对时，服务和 `wt` 命令都拒绝启动，分别提示 `WT_OWNER_DISCORD_ID is not set; set it to the owner's Discord user ID` 和 `WT_OWNER_DISCORD_ID must be a Discord user ID of 17 to 20 digits` |
| `WT_SITE_URL` | 站点对外的地址，例如 `https://your.site`。只用于 `wt emergency-login` 拼出完整的应急链接（第 4 节）；不填时命令只打印路径 |
| `WT_AVATAR_DIR` | 头像缓存目录，默认 `./wt-avatars`（容器内即 `/data/wt-avatars`） |
| `SESSION_SECRET` | 上游变量。头像地址的签名密钥由它派生（第 8 节），必须固定；用随机值时，重启前发出的头像地址全部失效 |
| `WT_CPA_URL` | 可选。号池（CLIProxyAPI，下称 CPA）的地址，只到端口、不带路径，例如 `http://cliproxyapi:8317`。与 `WT_CPA_MANAGEMENT_KEY` 一起非空（去掉首尾空白后）才开启捐赠账号功能（第 14 节） |
| `WT_CPA_MANAGEMENT_KEY` | 可选。CPA 管理密钥的明文。只从环境变量读，不进 options 表，控制台改不了：密钥会被发往 `WT_CPA_URL`，不能让后台权限改地址 |

**站长账号**：每次启动都检查 root 账号。

- 没有 root：创建一个绑定 `WT_OWNER_DISCORD_ID` 的 root（用户名 `root`，被占用时用 `wt_owner`）。它没有密码，只能用 Discord 登录。初始化标记为完成，不再出现上游的初始化向导。
- root 已绑定该 ID：不做任何改动。
- root 没有绑定，或绑定的是别的 ID：视为账号换了主人。在同一个事务里重新绑定，并清掉前任留下的全部凭据：
  - auth version 加一，所有浏览器会话失效；
  - 删除个人访问令牌和旧版 access token；
  - 删除 Passkey、两步验证（TOTP）及其备用码；
  - 清空密码；
  - 关闭它所有启用中的 API Key（保留，不删除），站长需要时自己重新打开；
  - 审计 `wt.owner_rebound`，只记数量（第 12 节）。
- 该 ID 已被别的账号占用：拒绝启动，提示 `WT_OWNER_DISCORD_ID is bound to another account; unbind or delete that account first`。

**清空密码**：每次启动都清空所有账号的密码，已注销的账号也算在内。

- 原因：账号留着密码时，上游会把密码当作它的二次验证方式，不再提供 Discord 验证。本站关闭了密码登录，密码这种方式不可用，结果这个账号的敏感操作（第 5 节）都没法验证。站点要是重新开启密码登录，二次验证又会退回到密码，例如从上游接管来的 root 还留着默认密码。
- 第 3 节在服务端拒绝重新设置密码，这一步清掉的是 wind-tide 接手之前，或拒绝生效之前留下的密码。
- 清掉了密码时审计 `wt.passwords_cleared`，只记账号数 `accounts`。没有可清的密码时不写这条审计。

**接管已有数据库**：不是由 wind-tide 建的库，第一次启动时接管一次。

- 禁用每一个非 root、并且没有 active 白名单条目的账号，原本就已禁用的也算在内。被禁用的账号 auth version 加一，会话失效，API Key 缓存清空。
- 其中绑定了 Discord 的账号，补一条 `removed` 状态的白名单条目，备注为 `Removed by the wind-tide upgrade`。站长之后可以在白名单里恢复它。
- 和第 5 节的“移除”一样，在禁用账号的同一个事务里撤销它的凭据：删除个人访问令牌和旧版 access token，关闭所有启用中的 API Key（保留，不删除）。之后恢复条目时，这些 Key 保持关闭，删掉的令牌也不会回来。
- 审计一次 `wt.takeover`，记录禁用的账号数 `accounts`、补出的条目数 `removed_entries`、删除的令牌数 `access_tokens`（含旧版 access token）和关闭的 API Key 数 `relay_keys`。没有可禁用的账号时不写这条审计。
- 最后写入选项 `WtAdoptedAt`（值为接管时间）。之后的启动看到它就不再接管，也不再补写默认选项（第 11 节）。接管中途失败时不写这个标记，下次启动会重来。

## 2. `/api/status` 新增字段

- `wt_discord_only: true`
- `wt_donation_enabled: true|false`：第 1 节的两个号池变量都非空时为 `true`（第 14 节）

## 3. Discord 登录闸门

**登录流程与登录 Cookie**：Discord 登录只能在发起它的浏览器里完成，用来防止登录 CSRF，也就是别人把自己的回调链接发给受害者，让受害者登进攻击者的账号。

- 前端照上游流程先请求 `POST /api/oauth/state`，body 为 `{ "provider": "discord", "intent": "login" }`。服务端返回 state 的同时种下登录 Cookie：
  - 值是随机数，登录流程里只存它的 SHA-256；
  - 属性为 `HttpOnly`、`SameSite=Lax`、有效期 10 分钟；
  - HTTPS 部署（`SESSION_COOKIE_SECURE=true`）时名为 `__Host-wt_oauth_login`，`Path=/`、`Secure`、不带 `Domain`。浏览器只接受本站 HTTPS 响应种下的这个名字，子域名和明文 HTTP 的响应都种不了、也盖不掉它；
  - 否则名为 `wt_oauth_login`，`Path=/api/oauth`。
- 回调 `GET /api/oauth/discord?code=&state=` 必须由同一个浏览器带着这个 Cookie 发起。缺少 Cookie 或对不上时返回 HTTP 403，`message` 为 `State parameter is empty or mismatched`，不登录也不进入下面的闸门。HTTPS 部署时只认带前缀的名字。
- 回调无论成功与否都会清掉这个 Cookie。同一浏览器再次发起登录会换一个新值，所以只有最近一次发起的登录能完成。
- 前端要求：这两个请求都是同源请求并带 Cookie（同源 `fetch` 的默认行为即可）。在别的浏览器或设备上打开回调链接会失败，这是预期行为。
- 只约束 Discord 登录（`intent = "login"`）。Discord 二次验证（`intent = "verify"`，第 5 节）不用这个 Cookie。

拿到 Discord 用户（id、username、global_name、avatar）后，按以下顺序判断：

1. id 等于 `WT_OWNER_DISCORD_ID`：以站长（root）身份登录。站长账号还没准备好时返回 `The owner account is not ready`。
2. id 在白名单中且状态为 active：
   - 已有账号就登录；账号被禁用时返回 `User has been banned`。
   - 没有账号就自动创建成员账号：role 1，初始余额 0，display_name 取 global_name，没有就取 username。
   - 条目关联的账号已被删除：条目自动改为 removed，按第 4 步拒绝。站长恢复这个条目后，该 Discord 账号下次登录会得到一个新账号。
   - root 账号永远不会经由白名单条目登录，只有站长本人的 Discord ID 能进 root。
3. 白名单里有旧版的用户名预填条目（现在已不能新建，见第 5 节）：状态为 active、用户名相同（忽略大小写和开头的 `@`），并且还没有绑定 ID。处理方式：
   - 把这个 Discord ID 锁定到该条目，同时记下当前用户名；已经持有该 ID 的账号也一并关联。
   - 该 ID 的 pending 入站申请改为 approved。
   - 审计 `wt.allowlist_pin`，然后按第 2 步处理。
   - 锁定之后，这条预填只放行这个 Discord ID，别人改成同样的用户名也进不来。有多条同名预填时取最早的一条。
4. 白名单条目状态为 removed：拒绝，`wt_gate = "removed"`。
5. 其余情况：生成或刷新入站申请。每个 Discord ID 只有一条申请记录。
   - 记录是 rejected 且距离拒绝不到 24 小时：`wt_gate = "rejected"`，并返回 `retry_after`（unix 秒）。
   - 其他情况：状态置为 pending，`wt_gate = "pending"`。

不放行时一律返回 HTTP 200，且不建立会话：

```json
{
  "success": false,
  "message": "Your entry request is waiting for approval",
  "data": {
    "wt_gate": "pending",
    "applicant": { "discord_id": "123", "username": "abc", "display_name": "ABC", "avatar_url": "/api/wt/avatar/123?v=hash&exp=1760198400&sig=..." },
    "requested_at": 1760000000,
    "retry_after": 0
  }
}
```

三种 `wt_gate` 对应的 `message`：`pending` 为 `Your entry request is waiting for approval`，`rejected` 为 `Your entry request was rejected, please try again later`，`removed` 为 `Your access to this site has been removed`。

每次经过闸门都会异步刷新头像缓存；登录成功时把 display_name 同步为最新的 global_name。名字含有数据库存不下的字符（例如 utf8mb3 的 MySQL 遇到 emoji）时，只是名字没存上，不影响登录。

**强制只走 Discord**：以下入口一律在服务端拒绝，不依赖后台开关。拒绝时返回 HTTP 403，`code` 为 `WT_DISCORD_ONLY`，`message` 为 `This site only supports signing in with Discord`：
- 初始化向导 `POST /api/setup`；
- 密码登录与注册、邮箱验证码、找回密码；
- Passkey 登录；
- GitHub、OIDC、LinuxDO、WeChat、Telegram、自定义 OAuth 的登录与注册；
- 任何 OAuth 账号绑定流程（`intent = "bind"`），包括 Discord；
- 清除 Discord 绑定：成员自己解绑，以及管理员 `DELETE /api/user/:id/bindings/discord`；
- 管理员新建用户 `POST /api/user/`。账号只能由 Discord 登录自动创建；
- 设置或修改密码：`PUT /api/user/self`（成员改自己的资料）和 `PUT /api/user/`（管理员编辑用户）的 body 里有 `password` 键，并且值不是 `null` 或空字符串。键名不分大小写，按服务端解析字段的规则匹配，所以 `Password` 这类写法同样拒绝。body 解析不了时也拒绝。不带密码的其他修改照常处理。

`POST /api/oauth/state`、`PUT /api/user/self` 和 `PUT /api/user/` 的 body 要先读出来判断：超过 64 KiB 时返回 HTTP 413，读取失败时返回 HTTP 400。

## 4. 应急登录链接

用法（容器里是 `/new-api`）：

```
new-api wt emergency-login [--base-url https://your.site]
```

- 写法仿照 `main.go` 里的 `plugin` 子命令。它连接数据库、生成令牌、打印链接后就退出，不启动服务，可以在服务运行时执行。数据库必须已由服务迁移过一次，否则报 `table wt_emergency_tokens is missing; start the server once so it migrates the database`。
- 链接的站点地址按顺序取：`--base-url`，其次环境变量 `WT_SITE_URL`。两者都没有时只打印路径 `/emergency-login#token=<token>`，并在 stderr 提示到站点自己的地址上打开。命令**从不**使用数据库里的 `ServerAddress`，因为管理员账号能改它，把链接指到别的网站。
- 站点地址必须是带主机名的 http 或 https 地址，不能带用户名、查询参数或 `#`。不合格时退出码为 2，提示 `the site address must be an http or https URL such as https://your.site`。参数写错时退出码也是 2，并打印上面的用法。
- 输出：链接在 stdout，只出现这一次；同一行之前可能有上游的启动日志（例如 `[SYS] ... using MySQL as database`）。有效期和“再生成会作废旧链接”的提示写在 stderr。
- 链接格式为 `{站点地址}/emergency-login#token=<token>`。令牌放在 `#` 后面，不会出现在服务器或代理的访问日志里。
- 令牌是 32 字节随机数（base64url，43 个字符），数据库只存 SHA-256；10 分钟过期，只能用一次，生成新令牌时作废所有旧令牌。
- `POST /api/wt/emergency-login`，body 为 `{ "token": "..." }`：成功则以站长身份建立会话，返回值与普通登录一致。失败一律返回同一条错误 `Emergency login link is invalid or has expired`。挂 `CriticalRateLimit`。
- 审计：生成链接记 `wt.emergency_link_issued`（只有过期时间），成功记 `wt.emergency_login`，失败记 `wt.emergency_login_failed`，都带 IP（命令行生成的那条没有 IP）。任何审计都不记录令牌或它的摘要。

## 5. 站长管理接口（仅 root）

**白名单条目**：

```json
{
  "id": 1,
  "discord_id": "123",
  "discord_username": "abc",
  "note": "",
  "status": "active",
  "user_id": 5,
  "account_status": "enabled",
  "display_name": "ABC",
  "avatar_url": "/api/wt/avatar/123?v=hash&exp=1760198400&sig=...",
  "created_at": 0,
  "updated_at": 0
}
```

- `discord_id` 只有旧版的用户名预填条目才可能为空：对方还没登录过。
- `user_id` 是这个条目放行的账号：已关联的账号；还没关联时，是已经持有该 Discord ID 的账号。没有账号或账号已删除时为 0。
- `account_status` 是该账号当前的状态：
  - `enabled`：正常；
  - `disabled`：已禁用；
  - `deleted`：关联的账号已被删除；
  - 空字符串：还没有账号。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/wt/allowlist?p=1&page_size=20&keyword=&status=` | `{items, total}`；keyword 匹配 Discord ID、用户名和备注 |
| POST | `/api/wt/allowlist` | body `{discord_id, note?}`，必须填 Discord 用户 ID。规则见下 |
| PUT | `/api/wt/allowlist/:id` | body `{note}` |
| POST | `/api/wt/allowlist/:id/remove` | 状态改为 removed，并禁用账号、撤销凭据；需要二次验证。见下 |
| POST | `/api/wt/allowlist/:id/restore` | 只能恢复 removed 条目：状态改为 active，并重新启用账号；需要二次验证。见下 |
| GET | `/api/wt/entry-requests?status=pending&p=1&page_size=20` | `{items, total}`，字段见下 |
| GET | `/api/wt/entry-requests/pending-count` | `{count}`，用于后台红点 |
| POST | `/api/wt/entry-requests/:id/approve` | 写入 active 白名单条目，申请状态改为 approved。规则见下 |
| POST | `/api/wt/entry-requests/:id/reject` | 只处理 pending 申请：状态改为 rejected，记录拒绝时间 |

入站申请条目：`{id, discord_id, discord_username, display_name, avatar_url, status, requested_at, decided_at}`。

**添加条目**：

- 必须填 Discord 用户 ID：没填时提示 `Enter a Discord user ID`，格式不对时提示 `Invalid Discord user ID`。
- 不能只填用户名。没人占用的 Discord 用户名谁都能拿去用，按用户名放行，等于放行第一个用这个名字登录的人。只知道对方用户名时，请对方先登录一次提交入站申请，站长再批准。body 里的 `discord_username` 会被忽略。
- 站长自己的 Discord ID 不需要条目，拒绝并提示 `The owner does not need an allowlist entry`。
- 同一个 Discord ID 已有条目时，提示 `This Discord account is already on the allowlist`。
- 该 ID 的 pending 申请一并改为 approved。
- 以前建的用户名预填条目保留，规则见第 3 节第 3 步。

**批准申请**：

- 站长自己的 ID 拒绝：`The owner does not need an allowlist entry`。
- 该 ID 的条目是 removed 时拒绝：`This Discord account has been removed, restore it from the allowlist instead`。
- 申请已经批准过时拒绝：`This entry request has already been handled`。被拒绝的申请仍然可以批准。
- 有一条还没锁定、用户名与申请人相同的旧版预填时，直接把预填锁定给申请人，不再新建条目。这样这条预填不会再放行下一个使用这个名字登录的人。

**移除**：

- 站长本人不能移除：`The owner cannot be removed from the allowlist`。
- 条目改为 removed，关联账号被禁用：auth version 加一，所有浏览器会话失效。账号原本已禁用时同样撤销凭据。
- 同一个事务里：
  - 删除该账号的个人访问令牌和旧版 access token；
  - 关闭它所有启用中的 API Key（保留，不删除）。
- 审计 `wt.allowlist_remove`，带撤销数量。

**恢复**：

- 只能恢复 removed 条目，否则提示 `Only a removed allowlist entry can be restored`。
- 条目改为 active，账号重新启用，余额不变。
- 移除时关掉的 API Key 保持关闭，需要成员自己重新打开；已删除的令牌也不会回来。
- 关联账号已被删除时，条目解除关联（`user_id` 变为 0），该 Discord 账号下次登录会新建账号。

**移除与恢复的二次验证**：条目的 `user_id` 不为 0 时，移除会禁用这个账号，恢复会启用它，因此和上游管理员禁用、启用用户一样，需要站长先做二次验证。验证参数如下：

- scope：`admin.user.manage`
- context：`{"user_id": <条目的 user_id>, "action": "disable"}`（移除）或 `{"user_id": <条目的 user_id>, "action": "enable"}`（恢复）

流程：

1. `user_id` 大于 0 时，前端先用上游的通用二次验证拿证明。这和用户管理里禁用、启用用户是同一套组件：
   - 可用方式由 `GET /api/verify/methods?scope=admin.user.manage` 列出；
   - Discord 重新验证走 `POST /api/oauth/state`，body 为 `{provider: "discord", intent: "verify", scope, context}`，回调成功后返回证明；
   - 两步验证、Passkey 等其他方式沿用上游的验证接口。
   - 成功后得到 `{proof_token, expires_at, method, scope}`。
2. 带请求头 `X-Security-Proof: <proof_token>` 请求移除或恢复接口。
3. 没带证明时返回 HTTP 403：`{"success": false, "code": "SECURITY_PROOF_REQUIRED", "message": "Additional verification required"}`。

注意事项：

- 证明只能用一次，并且只对这个账号、这个动作有效。
- 其他验证错误同样是 HTTP 403，按 `code` 区分：`SECURITY_PROOF_EXPIRED`、`SECURITY_PROOF_CONSUMED`、`SECURITY_PROOF_CONTEXT_MISMATCH`、`SECURITY_PROOF_SCOPE_MISMATCH`、`SECURITY_PROOF_METHOD_MISMATCH`、`SECURITY_PROOF_INVALID`。处理方法都是重新验证。
- `user_id` 为 0（还没人登录过，或账号已删除）时不需要验证，直接请求即可。
- 验证之后、真正修改之前，条目关联的账号如果变了，返回 `This allowlist entry has changed, refresh and try again`。前端刷新列表后重来。

**上游用户管理的限制**：启用一个非 root 账号时，必须有 active 白名单条目（按账号 ID 或 Discord ID 匹配）。这包括 `POST /api/user/manage` 的启用和编辑用户时把状态改为启用。

- 没有条目：`Only accounts on the allowlist can be enabled`。
- 条目是 removed：`This Discord account has been removed, restore it from the allowlist instead`，应改为在白名单里恢复。
- 上游的二次验证证明在这一步之前已经用掉，被拒后需要重新验证。

## 6. 签到（扩展上游）

- **签到日**：一律按北京时间（UTC+8）计算，不依赖服务器时区。
- **奖励**：固定额度。新库默认开启签到，奖励为 9,999 × `QuotaPerUnit`；不要求 Turnstile。
- `GET /api/user/checkin?month=YYYY-MM`：在上游原有字段之外，`data` 再加：
  - `today`（北京时间的 `YYYY-MM-DD`）
  - `checked_in_today`
  - `streak`（连签天数）
  - `reward_quota`
- `POST /api/user/checkin`：在原有返回之外，再加 `streak`。

## 7. 排行（登录成员可见）

`GET /api/wt/leaderboard?board=calls&period=today|week|all`，或 `GET /api/wt/leaderboard?board=balance`：

```json
{
  "board": "calls",
  "period": "today",
  "items": [ { "rank": 1, "user_id": 5, "display_name": "ABC", "avatar_url": "/api/wt/avatar/123?v=hash&exp=1760198400&sig=...", "value": 42 } ],
  "me": { "rank": 7, "value": 3 },
  "updated_at": 1760000000
}
```

- **调用**：成功的消费日志条数（log type = 2）。
  - `today` 按北京时间的签到日计算。
  - `week` 指含今天在内的 7 个签到日。
  - `all` 用 `users.request_count`。
- **余额**：`users.quota`。
- **规则**：
  - 排除 role ≥ 10 的账号和已禁用账号。
  - 并列名次相同（1、2、2、4）。
  - 取前 50 名；`me` 是请求者本人的名次和数值。请求者是站长时，`me` 为 null。
  - 每个榜单和榜期组合缓存 60 秒。

## 8. 头像

- 头像地址由服务端签名后放进各接口的 `avatar_url` 字段，格式为 `/api/wt/avatar/{discord_id}?v={头像 hash}&exp={过期时间}&sig={签名}`。
  - 前端原样使用这个地址，不要自己拼。
  - 账号没有自定义头像时 `avatar_url` 为空字符串，前端显示默认头像。
- 地址在同一个北京时间日内不变，到下一个北京时间日结束时过期，所以有效期在 24 到 48 小时之间。签名密钥由 `SESSION_SECRET` 派生。只有看到过这个地址的人才能取头像，陌生人无法用它探测哪些 Discord ID 在本站。
- `GET /api/wt/avatar/:discord_id?v=&exp=&sig=`：返回本地缓存的头像文件。
  - 只为已有账号或已有入站申请的 Discord ID 提供。
  - 以下情况一律返回同样的 404：没签名、签名不对、已过期、ID 不认识、没有自定义头像、还没缓存好。还没缓存时会在后台下载，下次请求就有。
  - 带 `Cache-Control: public, max-age=N`，N 取地址剩余有效秒数和 86400 中较小的一个；另带 `X-Content-Type-Options: nosniff`。
- 头像从 `https://cdn.discordapp.com/avatars/{id}/{hash}.png?size=128` 下载。
- `GET /api/user/self` 增加 `avatar_url` 字段。

## 9. 首页公开数据

`GET /api/wt/public-stats`，无需登录，缓存 60 秒：

```json
{ "members": 12, "calls_today": 345, "rewarded_quota_total": 123456789 }
```

## 10. 频率限制默认值

- **新库默认值**：`ModelRequestRateLimitEnabled = true`，`DurationMinutes = 1`，`Count = 5`，`SuccessCount` 保持上游默认。
- **计入限制的入口**：所有会产生生成任务的请求，包括：
  - 普通中转接口、Playground（`/pg/chat/completions`）；
  - Midjourney 提交类接口（`/mj/submit/*`、`/mj/insight-face/swap`，以及带模式前缀的同名路径）；
  - 任务提交 `/v1/tasks/:key`；
  - 视频生成 `/v1/video/generations`、`/v1/videos`；
  - 视频重混 `/v1/videos/:video_id/remix`。
- 查询任务进度的接口不计数。
- **站长豁免**：root 不受该限制，无论用浏览器会话还是 API Key 调用。

## 11. 新库默认选项

新库，以及接管前的旧库（第 1 节）里还没有设置的选项，写入以下值；已经有值的选项保持不变。写入 `WtAdoptedAt` 之后不再补写。

- `SystemName = 风与潮之夜`
- `QuotaForNewUser = 0`
- `RegisterEnabled = true`：Discord 自动建号需要它；闸门在第 3 节。
- 关闭密码登录和注册。
- 开启 `discord.enabled`。Client ID 和 Secret 部署时写入数据库，不写进代码或文件。
- 额度显示单位为 USD。
- 签到和频率限制的默认值见第 6 节和第 10 节。

## 12. 审计事件

站长在审计日志里能看到以下事件。参数里不含备注、令牌、密码或验证码。“目标账号”指参数里的 `target_user_id`：涉及的账号与操作人不同时才带。

| action | 何时记录 | 记在谁名下 | 参数 |
|---|---|---|---|
| `wt.allowlist_add` | 站长添加条目 | 站长 | `id`、`discord_id`、`discord_username`、`status`，以及目标账号 |
| `wt.allowlist_update` | 站长修改备注 | 站长 | 同上 |
| `wt.allowlist_remove` | 站长移除条目 | 站长 | 同上，再加：`access_tokens`（删除的令牌数，含旧版 access token）、`relay_keys`（关闭的 API Key 数）、`verification_method`（做了二次验证时才有） |
| `wt.allowlist_restore` | 站长恢复条目 | 站长 | `id`、`discord_id`、`discord_username`、`status`、目标账号、`verification_method`（做了二次验证时才有） |
| `wt.allowlist_pin` | 旧版用户名预填在登录时锁定到某个 Discord ID | 登录的成员 | `id`、`discord_id`、`discord_username` |
| `wt.entry_approve` | 批准入站申请 | 站长 | `id`、`discord_id`、`discord_username` |
| `wt.entry_reject` | 拒绝入站申请 | 站长 | 同上 |
| `wt.emergency_link_issued` | `wt emergency-login` 生成链接 | 站长 | `expires_at` |
| `wt.emergency_login` | 应急链接登录成功 | 站长 | 无 |
| `wt.emergency_login_failed` | 应急链接登录失败 | 站长（操作人角色为 0，因为请求是匿名的） | 无 |
| `wt.takeover` | 接管已有数据库时禁用了账号（第 1 节） | 站长 | `accounts`、`removed_entries`、`access_tokens`、`relay_keys` |
| `wt.passwords_cleared` | 启动时清掉了账号的密码（第 1 节） | 站长 | `accounts` |
| `wt.owner_rebound` | root 被重新绑定到站长 ID（第 1 节） | 站长 | `access_tokens`、`relay_keys`、`passkeys`、`two_factor`（启用中的两步验证数） |
| `wt.donation_add` | 成员提交回调地址（受理成功或失败都记，第 14 节） | 成员 | `id`（没建记录时为 0）、`provider`、`success` |
| `wt.donation_remove` | 成员移除捐赠账号（成功或失败都记） | 成员 | `id`、`provider`、`success` |
| `wt.donation_test` | 成员测试捐赠账号 | 成员 | `id`、`provider`、`success`、`latency_ms` |

## 13. 错误消息

以下 `message` 是 wind-tide 自己的英文原文，前端七种语言都要有翻译：

| message | 出现场景 |
|---|---|
| `Your entry request is waiting for approval` | 闸门：申请待审 |
| `Your entry request was rejected, please try again later` | 闸门：申请被拒、未满 24 小时 |
| `Your access to this site has been removed` | 闸门：条目已移除 |
| `The owner account is not ready` | 站长账号还没准备好 |
| `This site only supports signing in with Discord` | 第 3 节列出的被禁入口（`code = WT_DISCORD_ONLY`） |
| `Emergency login link is invalid or has expired` | 应急登录的所有失败 |
| `Invalid Discord user ID` | Discord ID 格式不对 |
| `Enter a Discord user ID` | 添加条目时没填 Discord 用户 ID |
| `This Discord account is already on the allowlist` | 重复添加 |
| `Allowlist entry does not exist` | 条目不存在 |
| `The owner does not need an allowlist entry` | 给站长添加条目或批准站长的申请 |
| `The owner cannot be removed from the allowlist` | 移除站长 |
| `Only a removed allowlist entry can be restored` | 恢复一个不是 removed 的条目 |
| `This allowlist entry has changed, refresh and try again` | 验证之后条目关联的账号变了 |
| `Entry request does not exist` | 申请不存在 |
| `This entry request has already been handled` | 重复批准，或拒绝一个不是 pending 的申请 |
| `This Discord account has been removed, restore it from the allowlist instead` | 批准一个已移除的 Discord 账号，或在用户管理里启用它 |
| `Only accounts on the allowlist can be enabled` | 在用户管理里启用一个没有白名单条目的账号 |
| `Account donation is not enabled on this site` | 捐赠：功能未开启（第 14 节） |
| `Enter a valid email address` | 捐赠：邮箱格式不对 |
| `You can donate at most {{max}} accounts` | 捐赠：超过每人 10 个 |
| `This account is already in the pool` | 捐赠：站内已有记录、号池里已有同名凭证，或收尾时文件名冲突 |
| `The account pool is temporarily unavailable, try again later` | 捐赠：任何号池调用失败 |
| `This donation session is invalid or has expired, start again` | 捐赠：flow 无效、过期或已消费 |
| `Paste the complete address from the browser's address bar` | 捐赠：回调地址解析失败、缺 state 或 code |
| `This address does not belong to the current donation` | 捐赠：state 不匹配 |
| `Google denied the authorization` | 捐赠：回调地址带 error |
| `Google authorization failed, start again` | 捐赠：号池报授权失败 |
| `Authorization timed out, start again` | 捐赠：号池超时或核验到截止 |
| `Could not confirm the donation result, start again` | 捐赠：号池忘记 state 且找不到凭证；站点重启后的收尾失败 |
| `The authorized Google account does not match the email you entered` | 捐赠：收尾兜底失败 |
| `Donation does not exist` | 捐赠：id 不存在或不属于本人 |
| `This donation is still being verified` | 捐赠：10 分钟内移除核验中的记录 |
| `Only an active donation can be tested` | 捐赠：测试非 active 记录 |
| `Wait a minute before testing again` | 捐赠：1 分钟内重复测试 |
| `This account has reached today's test limit` | 捐赠：当日 20 次用完 |
| `Could not fetch usage for this account` | 捐赠：用量查询失败 |
| `Donation removed` | 捐赠：移除成功 |
| `Donated account #{{id}} joined the pool` | 捐赠：成员系统日志 |
| `Donation #{{id}} failed` | 捐赠：成员系统日志 |

另外沿用上游已有的消息：`State parameter is empty or mismatched`（Discord 回调缺少或对不上登录 Cookie）、`User has been banned`、`Invalid parameters`，以及第 5 节的二次验证消息。

## 14. 捐赠账号

成员把自己不用的 Antigravity 账号授权给站点的号池（CLIProxyAPI，下称 CPA），大家共享其用量。设计全文见 `wind-tide/specs/2026-10-17-antigravity-donation-design.md`，术语见根目录 `CONTEXT.md`「捐赠」组。开关是第 1 节的两个环境变量和第 2 节的 `wt_donation_enabled`；功能关闭时下列接口一律返回 `Account donation is not enabled on this site`。

### 14.1 路由

全部经过 `DisableCache`、`UserAuth`，只接受浏览器会话（`accessTokenSessionRule`，个人访问令牌不能调用）。`UserCriticalRateLimit("wt-donation")` 由 start、callback、remove 共用，`UserCriticalRateLimit("wt-donation-test")` 给 test，都是每成员 20 次/20 分钟（`CRITICAL_RATE_LIMIT*` 环境变量）。

| 方法 | 路径 | 作用 |
|---|---|---|
| `POST` | `/api/wt/donations/start` | 开始捐赠或重新授权，拿到 Google 授权链接 |
| `POST` | `/api/wt/donations/callback` | 提交 Google 跳转到的回调地址 |
| `GET` | `/api/wt/donations` | 本人的捐赠列表（附号池实时状态） |
| `GET` | `/api/wt/donations/:id` | 单条状态（弹窗第 3 步每 3 秒轮询，只查数据库） |
| `GET` | `/api/wt/donations/:id/usage` | 剩余用量（缓存 60 秒） |
| `POST` | `/api/wt/donations/:id/test` | 测试账号（1 次/分钟、20 次/天） |
| `POST` | `/api/wt/donations/:id/remove` | 移除（同时删掉号池里的凭证） |

所有 `:id` 都只在本人的记录里找；不存在和不是本人的都返回 `Donation does not exist`，不区分。

### 14.2 开始 `POST /api/wt/donations/start`

请求 `{"email": "someone@example.com", "donation_id": 12}`，`donation_id` 可选，重新授权 `active`/`failed` 记录时带。检查顺序：邮箱格式与长度（最长 254）、每人最多 10 条（`verifying` + `active`）、站内同邮箱是否已有未移除的记录、号池里是否已有 `antigravity-<email>.json`；重新授权沿用原记录和它的邮箱，不再做数量、站内重复和号池重复这三项检查。然后向 CPA 要授权链接（`login_hint` 为该邮箱），建一条 `auth_flows`（purpose `wt_donation`，绑定成员和浏览器会话，5 分钟）。响应：

```json
{"flow_token": "…", "authorization_url": "https://accounts.google.com/…", "expires_at": 1760000300, "email": "someone@example.com"}
```

### 14.3 回调 `POST /api/wt/donations/callback`

请求 `{"flow_token": "…", "callback_url": "http://localhost:51121/oauth-callback?state=…&code=…"}`，地址最长 2048。服务端只解析不请求：缺 state 或 code → `Paste the complete address from the browser's address bar`；带 `error` → `Google denied the authorization`；state 与 flow 里存的不同 → `This address does not belong to the current donation`；flow 无效、过期或已消费 → `This donation session is invalid or has expired, start again`。通过后一次性消费 flow，建（或复用）`wt_donations` 记录为 `verifying`，把回调地址交给 CPA，并启动核验任务。响应 `{"id": 12, "status": "verifying"}`。无论受理成功还是失败都审计 `wt.donation_add`。

### 14.4 核验任务

每 2 秒问 CPA 一次授权状态，最多 120 秒。CPA 报成功后按 `antigravity-<email>.json` 找凭证，找到即 `active`（记下 `credential_name`、`auth_index`、`project_id`、`verified_at`）；CPA 说成功却找不到同名文件时，用提交前后的凭证列表差集兜底：恰好新增一个未被认领的凭证就按它登记，邮箱以实际授权的为准。失败原因写进 `fail_reason`，取值：`Authorization timed out, start again`、`Google denied the authorization`、`Google authorization failed, start again`、`Could not confirm the donation result, start again`、`The authorized Google account does not match the email you entered`、`This account is already in the pool`、`The account pool is temporarily unavailable, try again later`。成功或失败都给成员写一条系统日志（`Donated account #{{id}} joined the pool` / `Donation #{{id}} failed`）。站点启动时把遗留的 `verifying` 记录收尾一次。

### 14.5 列表 `GET /api/wt/donations`

```json
{"max": 10, "items": [{"id": 12, "email": "someone@example.com", "status": "active", "fail_reason": "", "pool": {"state": "cooldown", "message": "quota exhausted", "retry_at": 1760000600, "success": 120, "failed": 3}, "last_test": {"at": 1760000000, "ok": true, "latency_ms": 850, "message": ""}, "created_at": 1759990000, "verified_at": 1759990100}]}
```

只含 `status != removed` 的记录，按创建时间倒序。`pool` 只在 `active` 时给，否则 `null`，由一次凭证列表调用（进程内缓存 15 秒）按 `credential_name` 匹配；`pool.message` 是 CPA 的 `status_message`，截到 200 字符；`last_test` 没测过时为 `null`。`pool.state` 按顺序取第一个命中：

| 条件 | `state` |
|---|---|
| 凭证列表取不到（CPA 不可用） | `unknown` |
| 列表里没有这个文件名 | `missing` |
| `disabled` 为真 | `disabled` |
| `status_message` 含 `unauthori`、`invalid_grant`、`invalid token`、`refresh token` 或 `401`（不区分大小写） | `token_invalid` |
| `next_retry_after` 或任一 `cooldowns[].retry_at` 在未来，或 `unavailable` 为真 | `cooldown`（`retry_at` 取最晚的一个，没有时间时为 0） |
| `status == "error"` | `token_invalid` |
| 其余 | `ok` |

### 14.6 单条 `GET /api/wt/donations/:id`

`{"id": 12, "email": "someone@example.com", "status": "verifying", "fail_reason": ""}`，只查数据库，不调 CPA。

### 14.7 剩余用量 `GET /api/wt/donations/:id/usage`

记录必须是 `active`。按记录缓存 60 秒；失败不缓存，对外统一 `Could not fetch usage for this account`（`success:false`，HTTP 200）。

```json
{"fetched_at": 1760000000, "tier": "pro", "groups": [{"name": "Gemini 3 Pro", "buckets": [{"name": "Gemini 3 Pro", "window": "5h", "remaining_fraction": 0.73, "reset_at": 1760003600}]}]}
```

`tier` 是 `free`/`pro`/`ultra`/`ultra_lite`/`unknown`；`window` 原样透传（已知 `5h`、`weekly`）；`remaining_fraction` 是 0 到 1 的小数；`reset_at` 是 Unix 秒，没有时为 0。

### 14.8 测试 `POST /api/wt/donations/:id/test`

非 `active` → `Only an active donation can be tested`；距上次测试不足 60 秒 → `Wait a minute before testing again`；当天（`WtCheckinDay` 的北京时间自然日）已满 20 次 → `This account has reached today's test limit`。然后绕过缓存查一次用量并计时，更新 `last_test_*`、`tests_day`、`tests_today`，成功时顺带更新用量缓存，审计 `wt.donation_test`。响应 `{"ok": true, "latency_ms": 850, "message": "", "usage": {…}}`；失败时 `ok` 为 false、`usage` 为 `null`、`message` 为归类说明（`Account pool unreachable`、`HTTP 401 from Google` 一类、`Unreadable usage response from Google`、`The credential has no Google project`），HTTP 仍是 200 且 `success` 为 true。

### 14.9 移除 `POST /api/wt/donations/:id/remove`

`verifying` 且提交不足 10 分钟 → `This donation is still being verified`。有 `credential_name` 时先让 CPA 删凭证（200 或 404 算成功，其他 → `The account pool is temporarily unavailable, try again later`，记录不动），再把记录改成 `removed` 并写 `removed_at`，清掉它的用量缓存，审计 `wt.donation_remove`，返回 `Donation removed`。

### 14.10 常量与表

每人上限 10；flow 5 分钟；核验 2 秒一次、最多 120 秒；CPA 请求 15 秒超时、响应最多读 1 MB；凭证列表缓存 15 秒、用量缓存 60 秒；测试 1 次/分钟、20 次/天。表 `wt_donations` 的字段见规格第 5 节，CPA 的具体接口见规格第 6 节。日志里不记 code、回调地址、令牌、密钥、state 和邮箱。
