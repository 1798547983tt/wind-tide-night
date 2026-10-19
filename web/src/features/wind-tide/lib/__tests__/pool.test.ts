/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { describe, expect, test } from 'vitest'

import type { AisModelPool, AntigravityPoolLevel, PoolEvent } from '../../types'
import {
  aisLevel,
  antigravityLevel,
  mergePoolEvents,
  parseCallCount,
  poolEventSentence,
  rotatePoolEvents,
} from '../pool'

function event(id: number, overrides: Partial<PoolEvent> = {}): PoolEvent {
  return {
    id,
    kind: 'ais_given',
    provider: 'aistudio',
    display_name: 'Mizuki',
    amount: 5,
    model: 'gemini-4-argon',
    created_at: 1760000000 + id,
    ...overrides,
  }
}

function level(buckets: AntigravityPoolLevel['buckets']): AntigravityPoolLevel {
  return { fetched_at: 1760000000, accounts: 4, healthy: 3, buckets }
}

function modelPool(overrides: Partial<AisModelPool> = {}): AisModelPool {
  return {
    model: 'gemini-4-argon',
    relay_model: 'AIS PG-gemini-4-argon',
    per_account: 50,
    take_max: 100,
    allowance: 100,
    calls: 25,
    failed_today: 1,
    given: 0,
    taken: 0,
    remaining: 75,
    take_left: 100,
    pool_balance: 40,
    ...overrides,
  }
}

describe('tank levels', () => {
  test('the Antigravity tank is the mean of the bucket averages, clamped', () => {
    expect(antigravityLevel(level([]))).toBe(0)
    expect(
      antigravityLevel(
        level([
          {
            name: 'Gemini 3 Pro',
            window: '5h',
            average_remaining_fraction: 0.5,
          },
          {
            name: 'Gemini 3 Pro',
            window: 'weekly',
            average_remaining_fraction: 1,
          },
        ])
      )
    ).toBe(0.75)
    expect(
      antigravityLevel(
        level([{ name: 'x', window: '5h', average_remaining_fraction: 1.4 }])
      )
    ).toBe(1)
    expect(
      antigravityLevel(
        level([{ name: 'x', window: '5h', average_remaining_fraction: -0.2 }])
      )
    ).toBe(0)
  })

  test('an AIS gauge is what the member has left of the allowance', () => {
    expect(aisLevel(modelPool())).toBe(0.75)
    expect(aisLevel(modelPool({ allowance: 0, remaining: 0 }))).toBe(0)
    expect(aisLevel(modelPool({ remaining: 120 }))).toBe(1)
  })
})

describe('announcement sentences', () => {
  test.each([
    ['antigravity_donated', '{{name}} donated an Antigravity account'],
    ['antigravity_removed', '{{name}} removed an Antigravity account'],
    ['ais_account_donated', '{{name}} donated an AI Studio account'],
    ['ais_account_removed', '{{name}} removed an AI Studio account'],
    ['ais_given', '{{name}} gave {{count}} {{model}} calls to the pool'],
    ['ais_taken', '{{name}} took {{count}} {{model}} calls from the pool'],
  ] as const)('%s reads as %s', (kind, key) => {
    expect(
      poolEventSentence(
        event(1, { kind, amount: 7, model: 'gemini-3.8-flash' }),
        'Someone'
      )
    ).toEqual({
      key,
      params: { name: 'Mizuki', count: 7, model: 'gemini-3.8-flash' },
    })
  })

  test('a member without a name goes by the fallback', () => {
    expect(
      poolEventSentence(event(1, { display_name: '' }), 'Someone')?.params.name
    ).toBe('Someone')
  })

  test('a kind this build does not know is left out', () => {
    const unknown = { kind: 'something_new' as PoolEvent['kind'] }
    expect(poolEventSentence(event(1, unknown), 'Someone')).toBeNull()
  })
})

describe('merging polled announcements', () => {
  test('newest first, no duplicates, capped', () => {
    const merged = mergePoolEvents(
      [event(5), event(4), event(3)],
      [event(6), event(5), event(7)]
    )
    expect(merged.map((item) => item.id)).toEqual([7, 6, 5, 4, 3])
    expect(
      mergePoolEvents([event(2), event(1)], [event(3)], 2).map(
        (item) => item.id
      )
    ).toEqual([3, 2])
  })

  test('an empty poll keeps what is on show', () => {
    expect(
      mergePoolEvents([event(2), event(1)], []).map((item) => item.id)
    ).toEqual([2, 1])
  })

  test('the fresh copy wins when an id comes back changed', () => {
    const merged = mergePoolEvents(
      [event(1, { amount: 1 })],
      [event(1, { amount: 9 })]
    )
    expect(merged[0]?.amount).toBe(9)
  })
})

describe('rotating the rows on show', () => {
  const events = [5, 4, 3, 2, 1].map((id) => event(id))

  test('fewer events than the window show as they are', () => {
    expect(rotatePoolEvents(events, 3, 5)).toBe(events)
  })

  test('the window slides and wraps around', () => {
    const ids = (offset: number) =>
      rotatePoolEvents(events, offset, 3).map((item) => item.id)
    expect(ids(0)).toEqual([5, 4, 3])
    expect(ids(1)).toEqual([4, 3, 2])
    expect(ids(4)).toEqual([1, 5, 4])
    expect(ids(5)).toEqual([5, 4, 3])
  })
})

describe('the number of calls typed', () => {
  test.each([
    ['5', 5],
    [' 12 ', 12],
    ['0', null],
    ['-3', null],
    ['2.5', null],
    ['abc', null],
    ['', null],
    ['1e3', null],
  ])('%j parses as %j', (raw, count) => {
    expect(parseCallCount(raw)).toBe(count)
  })
})
