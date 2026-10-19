/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { createInstance } from 'i18next'
import { describe, expect, test } from 'vitest'

import zh from '@/i18n/locales/zh.json'

import { renderAuditContent } from '../format'

describe('wind-tide audit entries', () => {
  // The console is Simplified Chinese only, so the Chinese sentence is what
  // the owner reads in the audit log.
  test.each([
    ['wt.allowlist_add', { id: 7 }, '添加了白名单条目 7'],
    ['wt.allowlist_update', { id: 7 }, '修改了白名单条目 7 的备注'],
    ['wt.allowlist_remove', { id: 7 }, '移出了白名单条目 7'],
    [
      'wt.allowlist_remove',
      { id: 7, access_tokens: 2, relay_keys: 1, verification_method: 'totp' },
      '移出了白名单条目 7：删除了 2 个访问令牌，关闭了 1 个 API 密钥',
    ],
    ['wt.allowlist_restore', { id: 7 }, '恢复了白名单条目 7'],
    [
      'wt.allowlist_pin',
      { id: 7, discord_id: '123456789012345678', discord_username: 'mizuki' },
      '把白名单条目 7 锁定到了 Discord 账号 mizuki（123456789012345678）',
    ],
    ['wt.entry_approve', { id: 3 }, '批准了入站申请 3'],
    ['wt.entry_reject', { id: 3 }, '拒绝了入站申请 3'],
    ['wt.emergency_login', {}, '使用了应急登录'],
    [
      'wt.emergency_link_issued',
      { expires_at: 1792000000 },
      '签发了应急登录链接',
    ],
    ['wt.emergency_login_failed', {}, '应急登录尝试失败'],
    [
      'wt.takeover',
      { accounts: 3, removed_entries: 2 },
      '接管已有数据库时，停用了 3 个不在白名单里的账号',
    ],
    [
      'wt.takeover',
      { accounts: 3, removed_entries: 2, access_tokens: 2, relay_keys: 1 },
      '接管已有数据库时，停用了 3 个不在白名单里的账号：删除了 2 个访问令牌，关闭了 1 个 API 密钥',
    ],
    [
      'wt.passwords_cleared',
      { accounts: 2 },
      '清除了 2 个账号的密码：本站只用 Discord 登录和验证身份',
    ],
    [
      'wt.owner_rebound',
      { access_tokens: 2, relay_keys: 1, passkeys: 1, two_factor: 1 },
      '把 root 账号绑定到了新的站长 Discord 账号并清除了密码：删除了 2 个访问令牌、1 个 Passkey 和 1 个两步验证，关闭了 1 个 API 密钥',
    ],
    ['wt.donation_add', { id: 12 }, '向号池捐赠了账号 12'],
    ['wt.donation_remove', { id: 12 }, '从号池移除了捐赠账号 12'],
    ['wt.donation_test', { id: 12 }, '测试了捐赠账号 12'],
  ])(
    'a %s entry reads as a Chinese sentence with its details',
    async (action, params, sentence) => {
      const i18n = createInstance()
      await i18n.init({
        lng: 'zh',
        resources: { zh },
        nsSeparator: false,
        interpolation: { escapeValue: false },
      })

      expect(renderAuditContent({ op: { action, params } }, i18n.t)).toBe(
        sentence
      )
    }
  )
})

describe('wind-tide AIS pool audit entries', () => {
  // Their Chinese sentences arrive with the locale rows the operator loads
  // (the build here carries the English template), so the English reading
  // with its count is what this test pins down.
  test.each([
    [
      'wt.ais_give',
      { count: 5, model: 'gemini-4-argon' },
      'Gave 5 gemini-4-argon calls to the public pool',
    ],
    [
      'wt.ais_take',
      { count: 12, model: 'gemini-3.8-flash' },
      'Took 12 gemini-3.8-flash calls from the public pool',
    ],
  ])(
    'a %s entry reads as an English sentence with its count and model',
    async (action, params, sentence) => {
      const i18n = createInstance()
      await i18n.init({
        lng: 'en',
        resources: { en: { translation: {} } },
        nsSeparator: false,
        interpolation: { escapeValue: false },
      })

      expect(renderAuditContent({ op: { action, params } }, i18n.t)).toBe(
        sentence
      )
    }
  )
})
