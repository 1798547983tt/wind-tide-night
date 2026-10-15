# 风与潮之夜 · 前后端接口约定

前端和后端都以本文件为准。新增后端代码放在带 `wt_` 前缀的新文件里（`model/wt_*.go`、`controller/wt_*.go`、`service/wt_*.go`、`router/wt_*.go`）。新路由挂在 `/api/wt/` 下。响应沿用上游格式 `{success, message, data}`。金额字段一律是原始 quota，由前端按 `quota_per_unit` 换算成美元。

## 1. 部署配置（环境变量）

| 变量 | 含义 |
|---|---|
| `WT_OWNER_DISCORD_ID` | 站长的 Discord 用户 ID。启动时确保存在绑定该 ID 的 root 账号，并把初始化标记为完成；不再出现上游的初始化向导 |
| `WT_AVATAR_DIR` | 头像缓存目录，默认 `./wt-avatars`（容器内即 `/data/wt-avatars`） |

## 2. `/api/status` 新增字段

- `wt_discord_only: true`

## 3. Discord 登录闸门

`GET /api/oauth/discord?code=&state=` 沿用上游流程。拿到 Discord 用户（id、username、global_name、avatar）后，按以下顺序判断：

1. id 等于 `WT_OWNER_DISCORD_ID`：以站长（root）身份登录。
2. id 在白名单中且状态为 active：已有账号就登录，没有就自动创建成员账号。成员账号：role 1，初始余额 0，display_name 取 global_name，没有就取 username。
3. 白名单里有状态为 active、用户名相同（忽略大小写）且尚未绑定 ID 的预填条目：把 ID 锁定到该条目，然后按第 2 步处理。
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
    "applicant": { "discord_id": "123", "username": "abc", "display_name": "ABC", "avatar_url": "/api/wt/avatar/123?v=hash" },
    "requested_at": 1760000000,
    "retry_after": 0
  }
}
```

每次成功登录都刷新头像缓存（异步），并把 display_name 同步为最新的 global_name。

**强制只走 Discord**：以下入口一律在服务端拒绝，不依赖后台开关：
- 密码登录与注册、邮箱验证码注册；
- Passkey 登录；
- GitHub、OIDC、LinuxDO、WeChat、Telegram、自定义 OAuth 的登录与注册；
- 成员自行解绑 Discord。

## 4. 应急登录链接

- 在服务器上执行 `/new-api wt emergency-login`（仿照 `main.go` 里 `plugin` 子命令的写法）。它会连接数据库、生成令牌并打印链接后退出，不启动服务。
- 链接格式为 `{ServerAddress}/emergency-login#token=<token>`。令牌放在 `#` 后面，不会出现在服务器或代理的访问日志里。
- 令牌是 32 字节随机数，数据库只存 SHA-256；10 分钟过期，只能用一次，生成新令牌时作废旧令牌。
- `POST /api/wt/emergency-login`，body 为 `{ "token": "..." }`：成功则以站长身份建立会话，返回值与普通登录一致；失败一律返回同一条通用错误。挂 `CriticalRateLimit`。
- 审计日志只记录"使用了应急登录"和 IP，不记录令牌。

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
  "display_name": "ABC",
  "avatar_url": "/api/wt/avatar/123?v=hash",
  "created_at": 0,
  "updated_at": 0
}
```

`discord_id` 可以为空（只填了用户名、尚未登录过）；`user_id` 在对方还没账号时为 0。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/wt/allowlist?p=1&page_size=20&keyword=&status=` | `{items, total}` |
| POST | `/api/wt/allowlist` | body `{discord_id?, discord_username?, note?}`，两个标识至少填一个；重复就报错 |
| PUT | `/api/wt/allowlist/:id` | body `{note}` |
| POST | `/api/wt/allowlist/:id/remove` | 状态改为 removed；对应账号立即禁用，会话和令牌全部失效（复用上游禁用用户的逻辑） |
| POST | `/api/wt/allowlist/:id/restore` | 状态改为 active；重新启用原账号，余额不变 |
| GET | `/api/wt/entry-requests?status=pending&p=1&page_size=20` | `{items, total}`，字段见下 |
| GET | `/api/wt/entry-requests/pending-count` | `{count}`，用于后台红点 |
| POST | `/api/wt/entry-requests/:id/approve` | 写入 active 白名单条目，申请状态改为 approved |
| POST | `/api/wt/entry-requests/:id/reject` | 申请状态改为 rejected，记录拒绝时间 |

入站申请条目：`{id, discord_id, discord_username, display_name, avatar_url, status, requested_at, decided_at}`。

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
  "items": [ { "rank": 1, "user_id": 5, "display_name": "ABC", "avatar_url": "/api/wt/avatar/123?v=hash", "value": 42 } ],
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

- `GET /api/wt/avatar/:discord_id`：返回本地缓存的头像文件。
  - 只为已有账号或已有入站申请的 Discord ID 提供，其他 ID 返回 404。
  - 带 `Cache-Control: public, max-age=86400`。
- 头像从 `https://cdn.discordapp.com/avatars/{id}/{hash}.png?size=128` 下载。没有自定义头像的账号返回 404，前端显示默认头像。
- `GET /api/user/self` 增加 `avatar_url` 字段。

## 9. 首页公开数据

`GET /api/wt/public-stats`，无需登录，缓存 60 秒：

```json
{ "members": 12, "calls_today": 345, "rewarded_quota_total": 123456789 }
```

## 10. 频率限制默认值

- **新库默认值**：`ModelRequestRateLimitEnabled = true`，`DurationMinutes = 1`，`Count = 5`，`SuccessCount` 保持上游默认。
- **站长豁免**：root 不受该限制。

## 11. 新库默认选项

- `SystemName = 风与潮之夜`
- `QuotaForNewUser = 0`
- `RegisterEnabled = true`：Discord 自动建号需要它；闸门在第 3 节。
- 关闭密码登录和注册。
- 开启 `discord.enabled`。Client ID 和 Secret 部署时写入数据库，不写进代码或文件。
- 额度显示单位为 USD。
