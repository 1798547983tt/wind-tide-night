# AIS Cookie Helper（风与潮之夜）

一个很小的浏览器扩展：点一下，把这个浏览器里 Google 账号的登录 Cookie 复制到剪贴板，
然后回到中转站的「捐赠账号 → AIS playground」粘贴提交。

## 安装（Chrome / Edge）

1. 把下载的压缩包解压，得到 `ais-cookie-helper` 文件夹。
2. 打开 `chrome://extensions`（Edge 是 `edge://extensions`）。
3. 打开右上角的「开发者模式」。
4. 点「加载已解压的扩展程序」，选择刚才解压出来的 `ais-cookie-helper` 文件夹。
5. 工具栏（拼图图标）里会出现饼干图标，可以把它固定出来。

## 使用

1. 建议新建一个浏览器用户（Profile），只登录要捐的备用 Google 账号。
   只有浏览器里的第一个账号（u/0）会被使用。
2. 打开 <https://aistudio.google.com>，确认能正常进入。
3. 点饼干图标 → 「复制 Cookie」。
4. 回到中转站的捐赠页面，粘贴到输入框，提交。

## 它做了什么 / 没做什么

- 只读 `google.com` 域名下的 Cookie，只在你点按钮的时候读。
- 只复制到剪贴板，不保存、不联网、不发送到任何地方。
- 源码就是这个文件夹里的 `manifest.json`、`popup.html`、`popup.js`，几十行，可以自己看。
