# 风与潮之夜

只对白名单成员开放的公益 AI 接口中转站，由 New API 改版而来。成员每天签到领取余额，用 API 令牌调用站长接入的上游模型。

## 人

**站长** (Owner):
唯一的最高权限账号，绑定一个指定的 Discord 账号，负责审批入站申请、管理白名单和上游渠道。
_Avoid_: 管理员、超管、root

**成员** (Member):
Discord 账号在白名单中、并已在站内拥有账号的人。
_Avoid_: 用户、会员、旅人

**申请人** (Applicant):
用不在白名单中的 Discord 账号完成登录、正在等待站长处理入站申请的人。
_Avoid_: 待审核用户、游客

**管理视图** (Admin view):
站长看到的完整界面，包含成员功能和全部管理功能。
_Avoid_: 管理员界面、后台模式

**成员视图** (Member view):
站长用自己的账号、以普通成员看到的样子浏览本站的界面；只是隐藏管理功能，不是冒充其他成员。
_Avoid_: 用户界面、模拟登录、预览模式

## 准入

**白名单** (Allowlist):
允许进入本站的 Discord 账号清单，身份以 Discord 用户 ID 为准，不以用户名为准。
_Avoid_: 授权列表、邀请名单

**入站申请** (Entry request):
申请人登录时自动生成的待处理记录，由站长批准或拒绝；批准即把该账号加入白名单。
_Avoid_: 注册申请、审核单

**移出** (Removal):
站长把成员从白名单中删除；被移出的成员立即失去登录和调用能力。
_Avoid_: 禁用、拉黑、封号

**应急登录链接** (Emergency login link):
只能在服务器上生成、一次有效、很快过期的站长登录链接，供 Discord 登录不可用时使用。
_Avoid_: 后门、万能密码

## 余额

**余额** (Balance):
成员当前可用于调用的美元数额。
_Avoid_: 额度、金额、quota

**签到** (Check-in):
成员在每个签到日可以进行一次、领取签到奖励的动作。
_Avoid_: 打卡、每日登录

**签到日** (Check-in day):
以北京时间 0 点为分界的自然日。
_Avoid_: 今天（未注明时区时）

**签到奖励** (Check-in reward):
每次签到加到余额里的固定美元数。
_Avoid_: 签到额度、随机奖励

**连签** (Streak):
截至最近一个签到日、中间没有断过的签到天数。
_Avoid_: 连续登录

**调用** (Call):
一次成功并计费的 API 请求；失败的请求不算调用。
_Avoid_: 请求、使用次数

## 排行

**调用榜** (Call leaderboard):
按某个榜期内的调用次数给成员排名。
_Avoid_: 使用榜、活跃榜

**余额榜** (Balance leaderboard):
按当前余额给成员排名。
_Avoid_: 金额榜、财富榜

**榜期** (Leaderboard period):
调用榜的统计范围：今日、近 7 天、总榜；"今日"按签到日计算。
_Avoid_: 周期、时间段

**模型榜** (Model leaderboard):
按用量给模型排名的榜单，沿用上游。
_Avoid_: 模型排行

## 体验

**开幕** (Opening):
每次新打开网站时播放一次的剧场式开场动画。
_Avoid_: 启动页、加载动画、splash

**看板娘** (Mascot):
以 Q 版赛西莉亚为形象、在页面角落陪伴成员并给出提示的角色。
_Avoid_: 吉祥物、助手

**昼主题** (Day theme):
名为"风"的明亮视觉：奶油白、樱粉、抹茶绿与午后阳光。
_Avoid_: 亮色模式、白天模式

**夜主题** (Night theme):
名为"潮之夜"的深色视觉：午夜蓝、月光、海潮青与绯红点缀。
_Avoid_: 暗色模式、黑夜模式

**自动昼夜** (Auto day-night):
按北京时间在昼主题与夜主题之间自动切换的默认行为；成员手动选择后以其选择为准。
_Avoid_: 跟随系统

## 捐赠

**捐赠账号** (Donated account):
成员交给站点号池统一调配的账号：Antigravity 账号通过 Google 授权交，AI Studio 账号通过登录 Cookie 交。
_Avoid_: 捐号、上传账号

**号池** (Account pool):
站点接入的账号代理持有的全部凭证，站点渠道从里面轮询取号。现在有两个：Antigravity 号池（CLIProxyAPI）和 AIS 号池（aistudio-api）。
_Avoid_: 账号库

**AIS 账号** (AI Studio account):
成员以登录 Cookie 的形式捐给 AIS 号池的 Google AI Studio 账号；只取 Cookie 里浏览器的第一个账号。
_Avoid_: Cookie 号、AIS 号

**次数** (Calls):
成员当天在某个 AIS PG 模型上还能调用的次数，每个模型各记各的。每个在池的 AIS 账号每天给捐赠者 `AIS PG-gemini-4-argon` 50 次、`AIS PG-gemini-3.8-flash` 100 次，当天没用完不结转。
_Avoid_: 额度、配额、quota

**公共池** (Public pool):
当天由成员捐出的次数汇成的池子，每个 AIS PG 模型一个；有 AIS 账号的成员每天每个模型最多从里面取 100 次加到自己的次数里。
_Avoid_: 公共号池、次数池

**算力池** (Pool page):
「捐赠账号」页的第三个标签：两个号池的水位、本人次数与公共池、公告和排行榜。
_Avoid_: 水位页、算力榜

**水位** (Pool level):
Antigravity 号池里全部账号剩余用量的平均比例，按模型组和时间窗口分别算，每 5 分钟刷新。
_Avoid_: 总额度

**公告** (Announcement):
有人捐赠或移除账号、捐次数、取次数时自动生成的一条公开消息，带站内昵称（没有则用户名），只留最新 200 条。
_Avoid_: 通知、广播

**失败榜** (Failure board):
按某个 AIS PG 模型的调用失败次数排序的榜单，和调用次数榜一样按模型分开、分今日与累计。
_Avoid_: 黑榜

**剩余用量** (Remaining allowance):
Google 给某个捐赠账号的、按模型组和时间窗口计的剩余比例。不是成员余额。
_Avoid_: 额度、quota

**回调地址** (Callback address):
Google 授权完成后浏览器被跳转到的 localhost 地址，成员把它整个粘贴回来。
_Avoid_: 回调 URL（面向成员时）
