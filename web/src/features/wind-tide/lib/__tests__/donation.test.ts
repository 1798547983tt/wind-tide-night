/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { describe, expect, test } from 'vitest'

import type {
  AisDonation,
  AisDonationPool,
  AntigravityDonation,
  BuildDonation,
  BuildDonationPool,
  Donation,
  DonationPool,
  WbDonation,
  WbDonationPool,
} from '../../types'
import {
  CALLBACK_PRECHECK_MESSAGES,
  COOKIE_POOL_TEXTS,
  donationBadge,
  formatCountdown,
  formatRemaining,
  formatResetTime,
  maskEmail,
  needsReauthorization,
  precheckCallbackAddress,
  tierLabel,
  WB_REALM_LABELS,
  WB_TEXTS,
  windowLabel,
} from '../donation'

function pool(overrides: Partial<DonationPool> = {}): DonationPool {
  return {
    state: 'ok',
    message: '',
    retry_at: 0,
    success: 0,
    failed: 0,
    ...overrides,
  }
}

function donation(overrides: Partial<AntigravityDonation> = {}): Donation {
  return {
    id: 1,
    provider: 'antigravity',
    email: 'someone@example.com',
    status: 'active',
    fail_reason: '',
    pool: pool(),
    last_test: null,
    created_at: 1760000000,
    verified_at: 1760000100,
    ...overrides,
  }
}

describe('the badge of a donation', () => {
  test.each([
    ['Verifying', 'info', donation({ status: 'verifying', pool: null })],
    [
      'Failed',
      'danger',
      donation({
        status: 'failed',
        pool: null,
        fail_reason: 'Google denied the authorization',
      }),
    ],
    ['In service', 'success', donation()],
    [
      'Cooling down',
      'warning',
      donation({
        pool: pool({
          state: 'cooldown',
          message: 'quota exhausted',
          retry_at: 1760000600,
        }),
      }),
    ],
    [
      'Token invalid',
      'danger',
      donation({
        pool: pool({ state: 'token_invalid', message: 'invalid_grant' }),
      }),
    ],
    [
      'Not in the pool',
      'danger',
      donation({ pool: pool({ state: 'missing' }) }),
    ],
    ['Disabled', 'neutral', donation({ pool: pool({ state: 'disabled' }) })],
    [
      'Pool unreachable',
      'neutral',
      donation({ pool: pool({ state: 'unknown' }) }),
    ],
    // An active row the server could not attach a pool verdict to.
    ['Pool unreachable', 'neutral', donation({ pool: null })],
  ])('reads %s in %s', (label, variant, row) => {
    expect(donationBadge(row)).toEqual({ label, variant })
  })
})

describe('re-authorization', () => {
  test('is offered when the pool cannot use the token any more', () => {
    expect(
      needsReauthorization(donation({ pool: pool({ state: 'token_invalid' }) }))
    ).toBe(true)
    expect(
      needsReauthorization(donation({ pool: pool({ state: 'missing' }) }))
    ).toBe(true)
  })

  test('is not offered otherwise; a failed donation retries instead', () => {
    for (const state of ['ok', 'cooldown', 'disabled', 'unknown'] as const) {
      expect(needsReauthorization(donation({ pool: pool({ state }) }))).toBe(
        false
      )
    }
    expect(needsReauthorization(donation({ pool: null }))).toBe(false)
    expect(
      needsReauthorization(donation({ status: 'failed', pool: null }))
    ).toBe(false)
    expect(
      needsReauthorization(donation({ status: 'verifying', pool: null }))
    ).toBe(false)
  })
})

describe('the callback address precheck', () => {
  test.each([
    ['', 'empty'],
    ['   ', 'empty'],
    ['not an address', 'no_state'],
    ['http://localhost:51121/oauth-callback?code=4/abc', 'no_state'],
    ['http://localhost:51121/oauth-callback?state=abc', 'no_code'],
    [
      'http://localhost:51121/oauth-callback?state=abc&code=4/abc&scope=email',
      'ok',
    ],
    [
      'http://localhost:51121/oauth-callback?state=abc&error=access_denied',
      'ok',
    ],
    ['  http://localhost:51121/oauth-callback?state=abc&code=4/abc  ', 'ok'],
  ])('%j is %s', (value, verdict) => {
    expect(precheckCallbackAddress(value)).toBe(verdict)
  })

  test('each hint names the parameter it misses', () => {
    expect(CALLBACK_PRECHECK_MESSAGES.no_state).toContain('state')
    expect(CALLBACK_PRECHECK_MESSAGES.no_code).toContain('code')
  })
})

describe('usage formatting', () => {
  test('remaining fractions become whole percentages', () => {
    expect(formatRemaining(0.73)).toBe('73%')
    expect(formatRemaining(0)).toBe('0%')
    expect(formatRemaining(1)).toBe('100%')
    expect(formatRemaining(0.005)).toBe('1%')
  })

  test('known windows and tiers get labels; others pass through or say so', () => {
    expect(windowLabel('5h')).toBe('Every 5 hours')
    expect(windowLabel('weekly')).toBe('Every week')
    expect(windowLabel('daily')).toBe('daily')
    expect(tierLabel('free')).toBe('Free')
    expect(tierLabel('pro')).toBe('Pro')
    expect(tierLabel('ultra')).toBe('Ultra')
    expect(tierLabel('ultra_lite')).toBe('Ultra Lite')
    expect(tierLabel('unknown')).toBe('Unknown tier')
    expect(tierLabel('')).toBe('Unknown tier')
  })

  test('reset times show month, day and clock time; none is a dash', () => {
    // Noon UTC stays on the 15th in every time zone west of UTC+12.
    const noonUtc = Date.UTC(2026, 9, 15, 12, 0) / 1000
    expect(formatResetTime(noonUtc, 'en-US')).toMatch(/10\/15/)
    expect(formatResetTime(noonUtc, 'en-US')).toMatch(/\d{2}:\d{2}/)
    expect(formatResetTime(0)).toBe('-')
  })

  test('countdowns read m:ss and never go negative', () => {
    expect(formatCountdown(299)).toBe('4:59')
    expect(formatCountdown(60)).toBe('1:00')
    expect(formatCountdown(5)).toBe('0:05')
    expect(formatCountdown(0)).toBe('0:00')
    expect(formatCountdown(-3)).toBe('0:00')
  })
})

function aisPool(overrides: Partial<AisDonationPool> = {}): AisDonationPool {
  return {
    state: 'available',
    message: 'Available',
    retry_at: 0,
    success: 0,
    failed: 0,
    requests_today: 0,
    cooling: [],
    ...overrides,
  }
}

function aisDonation(overrides: Partial<AisDonation> = {}): Donation {
  return {
    id: 2,
    provider: 'aistudio',
    email: 'spare@example.com',
    status: 'active',
    fail_reason: '',
    pool: aisPool(),
    last_test: null,
    created_at: 1760000000,
    verified_at: 1760000100,
    ...overrides,
  }
}

describe('the badge of an AI Studio donation', () => {
  test.each([
    ['Verifying', 'info', aisDonation({ status: 'verifying', pool: null })],
    [
      'Failed',
      'danger',
      aisDonation({
        status: 'failed',
        pool: null,
        fail_reason: 'The pool lost the account; donate again',
      }),
    ],
    ['Available', 'success', aisDonation()],
    [
      'Quota used up today',
      'warning',
      aisDonation({
        pool: aisPool({ state: 'exhausted', retry_at: 1760040000 }),
      }),
    ],
    [
      'Paused',
      'warning',
      aisDonation({
        pool: aisPool({ state: 'cooldown', retry_at: 1760003600 }),
      }),
    ],
    [
      'Sign-in expired',
      'danger',
      aisDonation({ pool: aisPool({ state: 'expired' }) }),
    ],
    [
      'Not in the pool',
      'danger',
      aisDonation({ pool: aisPool({ state: 'missing' }) }),
    ],
    // An active row the server could not attach a pool verdict to.
    ['Pool unreachable', 'neutral', aisDonation({ pool: null })],
  ])('reads %s in %s', (label, variant, row) => {
    expect(donationBadge(row)).toEqual({ label, variant })
  })

  test('an AI Studio account is never re-authorized; it is removed and donated again', () => {
    expect(
      needsReauthorization(aisDonation({ pool: aisPool({ state: 'expired' }) }))
    ).toBe(false)
    expect(
      needsReauthorization(aisDonation({ pool: aisPool({ state: 'missing' }) }))
    ).toBe(false)
  })
})

describe('masking an email', () => {
  test.each([
    ['someone@example.com', 'so***@example.com'],
    ['a@example.com', 'a***@example.com'],
    ['Google account', 'Google account'],
    ['', ''],
  ])('%j shows as %j', (email, masked) => {
    expect(maskEmail(email)).toBe(masked)
  })
})

function buildPool(
  overrides: Partial<BuildDonationPool> = {}
): BuildDonationPool {
  return {
    state: 'available',
    message: 'Available',
    index: 3,
    current: false,
    has_context: true,
    ...overrides,
  }
}

function buildDonation(overrides: Partial<BuildDonation> = {}): Donation {
  return {
    id: 3,
    provider: 'build',
    email: '',
    status: 'active',
    fail_reason: '',
    pool: buildPool(),
    last_test: null,
    created_at: 1760000000,
    verified_at: 1760000100,
    ...overrides,
  }
}

describe('the badge of a BUILD donation', () => {
  test.each([
    ['Verifying', 'info', buildDonation({ status: 'verifying', pool: null })],
    [
      'Failed',
      'danger',
      buildDonation({
        status: 'failed',
        pool: null,
        fail_reason: 'The pool could not sign in with the account',
      }),
    ],
    ['Available', 'success', buildDonation()],
    [
      'Sign-in expired',
      'danger',
      buildDonation({ pool: buildPool({ state: 'expired' }) }),
    ],
    [
      'Unreadable',
      'danger',
      buildDonation({ pool: buildPool({ state: 'invalid' }) }),
    ],
    [
      'Not in the pool',
      'danger',
      buildDonation({ pool: buildPool({ state: 'missing' }) }),
    ],
    // An active row the server could not attach a pool verdict to.
    ['Pool unreachable', 'neutral', buildDonation({ pool: null })],
  ])('reads %s in %s', (label, variant, row) => {
    expect(donationBadge(row)).toEqual({ label, variant })
  })

  test('a BUILD account is never re-authorized either; it is removed and donated again', () => {
    expect(
      needsReauthorization(
        buildDonation({ pool: buildPool({ state: 'expired' }) })
      )
    ).toBe(false)
    expect(
      needsReauthorization(
        buildDonation({ pool: buildPool({ state: 'missing' }) })
      )
    ).toBe(false)
  })
})

describe('the words of each Cookie pool', () => {
  test('name the pool, its model and the way in', () => {
    expect(COOKIE_POOL_TEXTS.aistudio.name).toBe('AIS playground')
    expect(COOKIE_POOL_TEXTS.build.name).toBe('BUILD')
    for (const texts of Object.values(COOKIE_POOL_TEXTS)) {
      expect(texts.unlock).toMatch(/to unlock$/)
      expect(texts.donate).toMatch(/^Donate a/)
      expect(texts.emptyTitle).toMatch(/^No .* accounts yet$/)
    }
    expect(COOKIE_POOL_TEXTS.aistudio.addedWithEmail).toContain('{{email}}')
    // The BUILD pool never reads the email, so both verdicts read the same.
    expect(COOKIE_POOL_TEXTS.build.addedWithEmail).toBe(
      COOKIE_POOL_TEXTS.build.added
    )
    expect(COOKIE_POOL_TEXTS.aistudio.slogan).toContain('50 calls a day')
    expect(COOKIE_POOL_TEXTS.build.slogan).toContain(
      '100 Build-gemini-3.8-flash calls a day'
    )
    expect(COOKIE_POOL_TEXTS.build.added).toContain('BUILD pool')
    expect(COOKIE_POOL_TEXTS.aistudio.removal).toContain('Cookie')
    expect(COOKIE_POOL_TEXTS.build.removal).toContain('sign-in')
  })
})

function wbPool(overrides: Partial<WbDonationPool> = {}): WbDonationPool {
  return {
    state: 'available',
    message: 'Available',
    nickname: 'Alpha',
    realm: 'cn',
    credits: 300,
    credits_total: 500,
    retry_at: 0,
    success: 40,
    failed: 2,
    checked_in: true,
    ...overrides,
  }
}

function wbDonation(overrides: Partial<WbDonation> = {}): Donation {
  return {
    id: 4,
    provider: 'workbuddy',
    email: '',
    nickname: 'Alpha',
    status: 'active',
    fail_reason: '',
    pool: wbPool(),
    last_test: null,
    created_at: 1760000000,
    verified_at: 1760000100,
    ...overrides,
  }
}

describe('the badge of a WorkBuddy donation', () => {
  test.each([
    ['Verifying', 'info', wbDonation({ status: 'verifying', pool: null })],
    [
      'Failed',
      'danger',
      wbDonation({
        status: 'failed',
        pool: null,
        fail_reason: 'The sign-in was not finished in time; start again',
      }),
    ],
    ['Available', 'success', wbDonation()],
    [
      'Cooling down',
      'warning',
      wbDonation({ pool: wbPool({ state: 'cooldown', retry_at: 1760003600 }) }),
    ],
    ['Paused', 'neutral', wbDonation({ pool: wbPool({ state: 'paused' }) })],
    [
      'Sign-in expired',
      'danger',
      wbDonation({ pool: wbPool({ state: 'disabled' }) }),
    ],
    [
      'Not in the pool',
      'danger',
      wbDonation({ pool: wbPool({ state: 'missing' }) }),
    ],
    // An active row the server could not attach a pool verdict to.
    ['Pool unreachable', 'neutral', wbDonation({ pool: null })],
  ])('reads %s in %s', (label, variant, row) => {
    expect(donationBadge(row)).toEqual({ label, variant })
  })

  test('a WorkBuddy account is never re-authorized either; it is removed and donated again', () => {
    expect(
      needsReauthorization(wbDonation({ pool: wbPool({ state: 'disabled' }) }))
    ).toBe(false)
    expect(
      needsReauthorization(wbDonation({ pool: wbPool({ state: 'missing' }) }))
    ).toBe(false)
  })
})

describe('the words of the WorkBuddy pool', () => {
  test('name the pool, say the credits are shared and name both sign-ins', () => {
    expect(WB_TEXTS.name).toBe('WorkBuddy')
    expect(WB_TEXTS.donate).toMatch(/^Donate a/)
    expect(WB_TEXTS.emptyTitle).toMatch(/^No .* accounts yet$/)
    // No call count: the slogan promises the WB- models to every member.
    expect(WB_TEXTS.slogan).toContain('WB- models')
    expect(WB_TEXTS.slogan).toContain('every member')
    expect(WB_TEXTS.added).toContain('shared with everyone')
    expect(WB_TEXTS.addedWithNickname).toContain('{{nickname}}')
    expect(WB_TEXTS.removal).toContain('sign-in token')
    expect(WB_REALM_LABELS).toEqual({
      cn: 'CodeBuddy (China)',
      global: 'WorkBuddy (international)',
    })
  })
})
