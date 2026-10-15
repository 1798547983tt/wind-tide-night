# 只用 Discord 登录，用白名单和入站申请控制准入

本站只服务站长认识的人，所以关闭其他所有登录方式（包括站长本人的密码登录），只保留 Discord OAuth。Discord 账号不在白名单里，只会生成一条入站申请，等站长批准。站长最初想用 QQ 登录，后来改成 Discord：上游已内置 Discord OAuth，改动最小。

## Considered Options

- QQ 互联：网站接入一般要求 ICP 备案，而本站服务器在美国（本项目未进一步核实）。
- QQ 号 + QQ 邮箱验证码：可行，但要维护发信邮箱，登录也要多走几步。
- 保留密码登录：多一个可被撞库的入口，和"只有站长允许的人能进"相冲突。

## Consequences

- 大陆成员点"Discord 登录"的那一下需要能访问 discord.com。
- 身份以 Discord 用户 ID 为准，成员改 Discord 用户名不影响账号。
- Discord 登录不可用时，站长只能用服务器上生成的应急登录链接进入。
