/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import type { QueryClient } from '@tanstack/react-query'

import type {
  AisModelPool,
  AisPoolStatus,
  AntigravityPoolLevel,
  PoolEvent,
  PoolEventKind,
  WbPoolLevel,
} from '../types'

// The compute pool tab (wind-tide/API.md §15).

export const POOL_QUERY_ROOT = ['wind-tide', 'pool'] as const

export const POOL_EVENTS_QUERY_KEY = ['wind-tide', 'pool', 'events'] as const

/** Giving, taking and donating all move the figures on the tab. */
export function invalidatePoolQueries(queryClient: QueryClient) {
  return queryClient.invalidateQueries({ queryKey: POOL_QUERY_ROOT })
}

/** How many announcements the ticker keeps on this side. */
export const POOL_EVENTS_KEEP = 50

function clampFraction(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.min(1, Math.max(0, value))
}

/** How full the Antigravity tank is: the mean of the bucket averages. */
export function antigravityLevel(level: AntigravityPoolLevel): number {
  if (level.buckets.length === 0) return 0
  const sum = level.buckets.reduce(
    (total, bucket) => total + bucket.average_remaining_fraction,
    0
  )
  return clampFraction(sum / level.buckets.length)
}

/** How full the WorkBuddy tank is: the credits left against what the accounts had. */
export function wbLevel(level: WbPoolLevel): number {
  if (level.credits_total <= 0) return 0
  return clampFraction(level.credits / level.credits_total)
}

/** How full a model's gauge is: what the member has left of the day's allowance. */
export function aisLevel(pool: AisModelPool): number {
  if (pool.allowance <= 0) return 0
  return clampFraction(pool.remaining / pool.allowance)
}

/**
 * How full a model's public pool is: what is left of everything given today.
 * Nothing given yet is an empty pool, whatever stray balance it reports.
 */
export function sharedPoolLevel(pool: AisModelPool): number {
  if (pool.pool_given <= 0) return 0
  return clampFraction(pool.pool_balance / pool.pool_given)
}

/** The public pools of every Cookie pool the site runs, in the site's order. */
export function sharedPools(
  statuses: (AisPoolStatus | null)[]
): AisModelPool[] {
  return statuses.flatMap((status) => status?.models ?? [])
}

export type PoolEventSentence = {
  key: string
  params: { name: string; count: number; model: string }
}

// Sentences are i18n keys; {{name}} is the member, {{count}} the calls and
// {{model}} the upstream model the calls belong to.
const EVENT_SENTENCES: Record<PoolEventKind, string> = {
  antigravity_donated: '{{name}} donated an Antigravity account',
  antigravity_removed: '{{name}} removed an Antigravity account',
  ais_account_donated: '{{name}} donated an AI Studio account',
  ais_account_removed: '{{name}} removed an AI Studio account',
  ais_given: '{{name}} gave {{count}} {{model}} calls to the pool',
  ais_taken: '{{name}} took {{count}} {{model}} calls from the pool',
  build_account_donated: '{{name}} donated a BUILD account',
  build_account_removed: '{{name}} removed a BUILD account',
  build_given: '{{name}} gave {{count}} {{model}} calls to the pool',
  build_taken: '{{name}} took {{count}} {{model}} calls from the pool',
  wb_account_donated: '{{name}} donated a WorkBuddy account',
  wb_account_removed: '{{name}} removed a WorkBuddy account',
}

/**
 * The sentence an announcement reads as, or null for a kind this build does
 * not know, which the ticker leaves out rather than showing a raw code.
 */
export function poolEventSentence(
  event: PoolEvent,
  anonymous: string
): PoolEventSentence | null {
  const key = EVENT_SENTENCES[event.kind]
  if (!key) return null
  return {
    key,
    params: {
      name: event.display_name || anonymous,
      count: event.amount,
      model: event.model,
    },
  }
}

/**
 * Folds freshly polled announcements into the ones on show: newest first, no
 * duplicates, at most `keep`. Ids only grow, so they order the feed.
 */
export function mergePoolEvents(
  current: PoolEvent[],
  incoming: PoolEvent[],
  keep = POOL_EVENTS_KEEP
): PoolEvent[] {
  const byId = new Map<number, PoolEvent>()
  for (const event of [...current, ...incoming]) byId.set(event.id, event)
  return [...byId.values()].sort((a, b) => b.id - a.id).slice(0, keep)
}

/** The announcements on show, starting `offset` rows into the feed. */
export function rotatePoolEvents(
  events: PoolEvent[],
  offset: number,
  window: number
): PoolEvent[] {
  if (events.length <= window) return events
  const start = ((offset % events.length) + events.length) % events.length
  return [...events.slice(start), ...events.slice(0, start)].slice(0, window)
}

/** A whole number of calls, 1 or more; null for anything else. */
export function parseCallCount(raw: string): number | null {
  const trimmed = raw.trim()
  if (!/^\d+$/.test(trimmed)) return null
  const count = Number(trimmed)
  if (!Number.isSafeInteger(count) || count < 1) return null
  return count
}
