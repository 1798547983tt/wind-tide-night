/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import type { QueryClient } from '@tanstack/react-query'

import type { StatusVariant } from '@/components/status-badge'
import { formatPercent } from '@/lib/format'

import type {
  AisPoolState,
  BuildPoolState,
  CookieProvider,
  Donation,
  PoolState,
  WbPoolState,
  WbRealm,
} from '../types'

// The member's donated accounts page (wind-tide/API.md §14 to §16).

export const DONATIONS_QUERY_ROOT = ['wind-tide', 'donations'] as const

/** Every write (donate, test, remove) refreshes the list and what hangs off it. */
export function invalidateDonationQueries(queryClient: QueryClient) {
  return queryClient.invalidateQueries({ queryKey: DONATIONS_QUERY_ROOT })
}

export type DonationBadge = { label: string; variant: StatusVariant }

// Labels are i18n keys.
const POOL_BADGES: Record<PoolState, DonationBadge> = {
  ok: { label: 'In service', variant: 'success' },
  cooldown: { label: 'Cooling down', variant: 'warning' },
  token_invalid: { label: 'Token invalid', variant: 'danger' },
  missing: { label: 'Not in the pool', variant: 'danger' },
  disabled: { label: 'Disabled', variant: 'neutral' },
  unknown: { label: 'Pool unreachable', variant: 'neutral' },
}

// The AI Studio pool's states (wind-tide/API.md §15); labels are i18n keys.
const AIS_POOL_BADGES: Record<AisPoolState, DonationBadge> = {
  available: { label: 'Available', variant: 'success' },
  exhausted: { label: 'Quota used up today', variant: 'warning' },
  cooldown: { label: 'Paused', variant: 'warning' },
  expired: { label: 'Sign-in expired', variant: 'danger' },
  missing: { label: 'Not in the pool', variant: 'danger' },
}

// The BUILD pool's states (wind-tide/API.md §15); labels are i18n keys.
const BUILD_POOL_BADGES: Record<BuildPoolState, DonationBadge> = {
  available: { label: 'Available', variant: 'success' },
  expired: { label: 'Sign-in expired', variant: 'danger' },
  invalid: { label: 'Unreadable', variant: 'danger' },
  missing: { label: 'Not in the pool', variant: 'danger' },
}

// The WorkBuddy pool's states (wind-tide/API.md §16); labels are i18n keys.
const WB_POOL_BADGES: Record<WbPoolState, DonationBadge> = {
  available: { label: 'Available', variant: 'success' },
  cooldown: { label: 'Cooling down', variant: 'warning' },
  paused: { label: 'Paused', variant: 'neutral' },
  disabled: { label: 'Sign-in expired', variant: 'danger' },
  missing: { label: 'Not in the pool', variant: 'danger' },
}

export function donationBadge(donation: Donation): DonationBadge {
  if (donation.status === 'verifying') {
    return { label: 'Verifying', variant: 'info' }
  }
  if (donation.status === 'failed') {
    return { label: 'Failed', variant: 'danger' }
  }
  if (donation.provider === 'aistudio') {
    if (!donation.pool) return POOL_BADGES.unknown
    return AIS_POOL_BADGES[donation.pool.state]
  }
  if (donation.provider === 'build') {
    if (!donation.pool) return POOL_BADGES.unknown
    return BUILD_POOL_BADGES[donation.pool.state]
  }
  if (donation.provider === 'workbuddy') {
    if (!donation.pool) return POOL_BADGES.unknown
    return WB_POOL_BADGES[donation.pool.state]
  }
  return POOL_BADGES[donation.pool?.state ?? 'unknown']
}

/** What the member reads about each Cookie pool; every value is an i18n key. */
export type CookiePoolTexts = {
  /** The pool's name, as the tab and the tank call it. */
  name: string
  /** The owner's words at the top of the page. */
  slogan: string
  emptyTitle: string
  emptyDescription: string
  /** The donate button and the dialog's title. */
  donate: string
  /** The way into a locked tank. */
  unlock: string
  /** Step 3's good news, without and with the account's email. */
  added: string
  addedWithEmail: string
  /** What leaves with the account when it is removed. */
  removal: string
}

export const COOKIE_POOL_TEXTS: Record<CookieProvider, CookiePoolTexts> = {
  aistudio: {
    name: 'AIS playground',
    slogan:
      'Spare AI Studio accounts become shared AIS PG calls: every account you donate gives you 50 calls a day, and the pool shares the load.',
    emptyTitle: 'No AI Studio accounts yet',
    emptyDescription:
      'Donate a spare AI Studio account: every active account gives you AIS PG calls each day, and the pool shares the load.',
    donate: 'Donate an AI Studio account',
    unlock: 'Donate an AI Studio account to unlock',
    added:
      'The account is now in the AIS playground pool and raises your AIS PG calls for every day it stays. Thank you!',
    addedWithEmail:
      '{{email}} is now in the AIS playground pool and raises your AIS PG calls for every day it stays. Thank you!',
    removal:
      'The account leaves the pool now and its Cookie is deleted. You can donate it again later.',
  },
  build: {
    name: 'BUILD',
    slogan:
      'Spare AI Studio accounts become shared BUILD calls: every account you donate gives you 100 Build-gemini-3.8-flash calls a day, and the pool shares the load.',
    emptyTitle: 'No BUILD accounts yet',
    emptyDescription:
      'Donate a spare AI Studio account to the BUILD pool: every active account gives you 100 Build-gemini-3.8-flash calls a day, and the pool shares the load.',
    donate: 'Donate a BUILD account',
    unlock: 'Donate a BUILD account to unlock',
    added:
      'The account is now in the BUILD pool and raises your Build-gemini-3.8-flash calls for every day it stays. Thank you!',
    addedWithEmail:
      'The account is now in the BUILD pool and raises your Build-gemini-3.8-flash calls for every day it stays. Thank you!',
    removal:
      'The account leaves the pool now and its sign-in is deleted. You can donate it again later.',
  },
}

/** What the member reads about the WorkBuddy pool; every value is an i18n key. */
export type WbPoolTexts = {
  /** The pool's name, as the tab and the tank call it. */
  name: string
  /** The owner's words at the top of the page. */
  slogan: string
  emptyTitle: string
  emptyDescription: string
  /** The donate button and the dialog's title. */
  donate: string
  /** Step 3's good news, without and with the account's nickname. */
  added: string
  addedWithNickname: string
  /** What leaves with the account when it is removed. */
  removal: string
}

// The WorkBuddy pool (wind-tide/API.md §16) has no call count: whatever is
// donated is shared by every member, so its words say so.
export const WB_TEXTS: WbPoolTexts = {
  name: 'WorkBuddy',
  slogan:
    'Spare CodeBuddy and WorkBuddy accounts join one shared pool: every member can call the WB- models, and the pool spreads the requests over every donated account.',
  emptyTitle: 'No WorkBuddy accounts yet',
  emptyDescription:
    "Donate a spare CodeBuddy or WorkBuddy account: one sign-in on Tencent's page, and its credits are shared with everyone here.",
  donate: 'Donate a WorkBuddy account',
  added:
    'The account is now in the WorkBuddy pool and its credits are shared with everyone on this site. Thank you!',
  addedWithNickname:
    '{{nickname}} is now in the WorkBuddy pool and its credits are shared with everyone on this site. Thank you!',
  removal:
    'The account leaves the pool now and its sign-in token is deleted. You can donate it again later.',
}

/** The two sign-ins the WorkBuddy pool knows; labels are i18n keys. */
export const WB_REALM_LABELS: Record<WbRealm, string> = {
  cn: 'CodeBuddy (China)',
  global: 'WorkBuddy (international)',
}

/**
 * An active Antigravity account whose token the pool can no longer use. An
 * AI Studio or WorkBuddy account cannot be authorized again: it is removed
 * and donated afresh.
 */
export function needsReauthorization(donation: Donation): boolean {
  if (donation.provider !== 'antigravity') return false
  if (donation.status !== 'active' || !donation.pool) return false
  return (
    donation.pool.state === 'token_invalid' || donation.pool.state === 'missing'
  )
}

/**
 * `someone@example.com` → `so***@example.com`: the member's own card names the
 * account without spelling it out to anyone looking over their shoulder.
 */
export function maskEmail(email: string): string {
  const at = email.indexOf('@')
  if (at <= 0) return email
  const local = email.slice(0, at)
  const domain = email.slice(at)
  const keep = Math.min(2, local.length)
  return `${local.slice(0, keep)}***${domain}`
}

export type CallbackPrecheck = 'empty' | 'no_state' | 'no_code' | 'ok'

/**
 * A hint while the member pastes. The server makes the real decision
 * (wind-tide/API.md §14), so a bad verdict here never blocks the submit.
 */
export function precheckCallbackAddress(value: string): CallbackPrecheck {
  const trimmed = value.trim()
  if (!trimmed) return 'empty'
  let params: URLSearchParams
  try {
    params = new URL(trimmed).searchParams
  } catch {
    return 'no_state'
  }
  if (!params.has('state')) return 'no_state'
  if (!params.has('code') && !params.has('error')) return 'no_code'
  return 'ok'
}

export const CALLBACK_PRECHECK_MESSAGES: Record<
  'no_state' | 'no_code',
  string
> = {
  no_state:
    'This does not look like the address Google sent you to: it has no state parameter',
  no_code:
    'The address has no code yet. Finish signing in first, then copy the address of the page that appears',
}

/** 0.73 → "73%"; formatPercent takes a percentage, not a fraction. */
export function formatRemaining(fraction: number): string {
  return formatPercent(Math.round(fraction * 100))
}

const WINDOW_LABELS: Record<string, string> = {
  '5h': 'Every 5 hours',
  weekly: 'Every week',
}

/** An i18n key for a known window; an unknown one shows as Google names it. */
export function windowLabel(window: string): string {
  return WINDOW_LABELS[window] ?? window
}

export function formatResetTime(resetAt: number, locale?: string): string {
  if (!resetAt) return '-'
  return new Intl.DateTimeFormat(locale, {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(resetAt * 1000))
}

/** 299 → "4:59"; never negative. */
export function formatCountdown(seconds: number): string {
  const left = Math.max(0, Math.floor(seconds))
  const minutes = Math.floor(left / 60)
  const rest = left % 60
  return `${minutes}:${rest.toString().padStart(2, '0')}`
}

// The short names service.WtTierName hands out; labels are i18n keys.
const TIER_LABELS: Record<string, string> = {
  free: 'Free',
  pro: 'Pro',
  ultra: 'Ultra',
  ultra_lite: 'Ultra Lite',
}

export function tierLabel(tier: string): string {
  return TIER_LABELS[tier] ?? 'Unknown tier'
}
