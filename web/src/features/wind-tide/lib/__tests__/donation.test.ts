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
  Donation,
  DonationPool,
} from '../../types'
import {
  CALLBACK_PRECHECK_MESSAGES,
  donationBadge,
  formatCountdown,
  formatRemaining,
  formatResetTime,
  maskEmail,
  needsReauthorization,
  precheckCallbackAddress,
  tierLabel,
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
