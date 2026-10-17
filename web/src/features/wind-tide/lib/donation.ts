/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import type { QueryClient } from '@tanstack/react-query'

import type { StatusVariant } from '@/components/status-badge'
import { formatPercent } from '@/lib/format'

import type { Donation, PoolState } from '../types'

// The member's donated accounts page (wind-tide/API.md §14).

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

export function donationBadge(donation: Donation): DonationBadge {
  if (donation.status === 'verifying') {
    return { label: 'Verifying', variant: 'info' }
  }
  if (donation.status === 'failed') {
    return { label: 'Failed', variant: 'danger' }
  }
  return POOL_BADGES[donation.pool?.state ?? 'unknown']
}

/** An active account whose token the pool can no longer use. */
export function needsReauthorization(donation: Donation): boolean {
  if (donation.status !== 'active' || !donation.pool) return false
  return (
    donation.pool.state === 'token_invalid' || donation.pool.state === 'missing'
  )
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
