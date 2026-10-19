// wind-tide: AIS cookie helper popup (wind-tide/tools/ais-cookie-helper).
// Reads the Google sign-in cookies of this browser profile when the button is
// clicked, turns them into one "name=value; name=value" line and copies it to
// the clipboard. Nothing is stored and nothing leaves the browser.
'use strict';

// When the same cookie name exists on several Google hosts, keep the broadest
// one first: the pool re-applies that value to every host it needs.
const PREFERRED_DOMAINS = ['.google.com', 'google.com', 'aistudio.google.com', 'accounts.google.com'];
// Analytics cookies carry nothing the pool needs.
const SKIPPED = /^(_ga|_gid|_gat|_gcl)/;
// Each group must have at least one member, otherwise nobody is signed in.
const SIGN_IN_GROUPS = [
  ['SAPISID', '__Secure-1PAPISID', '__Secure-3PAPISID'],
  ['SID', '__Secure-1PSID', '__Secure-3PSID'],
];

function domainRank(cookie) {
  const index = PREFERRED_DOMAINS.indexOf(cookie.domain);
  return index === -1 ? PREFERRED_DOMAINS.length : index;
}

async function readCookies() {
  const all = await chrome.cookies.getAll({ domain: 'google.com' });
  const chosen = new Map();
  for (const cookie of all) {
    if (SKIPPED.test(cookie.name) || !cookie.value) {
      continue;
    }
    const previous = chosen.get(cookie.name);
    if (!previous || domainRank(cookie) < domainRank(previous)) {
      chosen.set(cookie.name, cookie);
    }
  }
  return [...chosen.values()];
}

function missingGroups(cookies) {
  const names = new Set(cookies.map((cookie) => cookie.name));
  return SIGN_IN_GROUPS.filter((group) => !group.some((name) => names.has(name)));
}

function headerString(cookies) {
  return cookies.map((cookie) => `${cookie.name}=${cookie.value}`).join('; ');
}

function show(status, text, tone) {
  status.textContent = text;
  status.className = tone || '';
}

async function copyCookies(button, status, fallback) {
  button.disabled = true;
  fallback.classList.remove('shown');
  fallback.value = '';
  show(status, '正在读取…');
  try {
    const cookies = await readCookies();
    if (cookies.length === 0 || missingGroups(cookies).length > 0) {
      show(
        status,
        '没检测到 Google 登录。\n请先在这个浏览器里登录备用账号，打开 aistudio.google.com 后再点一次。',
        'bad',
      );
      return;
    }
    const text = headerString(cookies);
    try {
      await navigator.clipboard.writeText(text);
      show(status, `已复制 ${cookies.length} 条 Cookie。\n回到中转站的捐赠页面，粘贴到输入框提交即可。`, 'ok');
    } catch {
      fallback.value = text;
      fallback.classList.add('shown');
      fallback.focus();
      fallback.select();
      show(status, '浏览器没允许自动复制，已把内容放在下面的框里，请手动全选复制。', 'bad');
    }
  } catch (error) {
    show(status, `读取失败：${error && error.message ? error.message : error}`, 'bad');
  } finally {
    button.disabled = false;
  }
}

document.addEventListener('DOMContentLoaded', () => {
  const button = document.getElementById('copy');
  const status = document.getElementById('status');
  const fallback = document.getElementById('fallback');
  button.addEventListener('click', () => {
    copyCookies(button, status, fallback);
  });
});
