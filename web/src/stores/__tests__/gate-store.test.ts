/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import type { GateRefusal } from '@/features/wind-tide/types'

const GATE_KEY = 'wt.gate'

const pending: GateRefusal = {
  wt_gate: 'pending',
  applicant: {
    discord_id: '200000000000000002',
    username: 'tidewalker',
    display_name: 'Tide Walker',
    avatar_url: '/api/wt/avatar/200000000000000002?v=a1',
  },
  requested_at: 1_760_000_000,
  retry_after: 0,
}

// The store reads the saved refusal when its module loads, so each case loads
// a fresh copy after arranging storage.
async function loadStore() {
  vi.resetModules()
  const module = await import('../gate-store')
  return module.useGateStore
}

beforeEach(() => {
  window.sessionStorage.clear()
})

afterEach(() => {
  vi.restoreAllMocks()
  window.sessionStorage.clear()
})

describe('gate store', () => {
  test('a tab without a refusal starts empty', async () => {
    const store = await loadStore()

    expect(store.getState().refusal).toBeNull()
  })

  test('a refusal survives a reload of the tab', async () => {
    const first = await loadStore()
    first.getState().setRefusal(pending)

    const reloaded = await loadStore()

    expect(reloaded.getState().refusal).toEqual(pending)
  })

  test.each([
    ['broken JSON', '{"wt_gate":'],
    ['an unknown state', JSON.stringify({ ...pending, wt_gate: 'approved' })],
    ['no applicant', JSON.stringify({ ...pending, applicant: undefined })],
  ])('saved data with %s is ignored', async (_case, saved) => {
    window.sessionStorage.setItem(GATE_KEY, saved)

    const store = await loadStore()

    expect(store.getState().refusal).toBeNull()
  })

  test('unavailable storage still shows the refusal for this visit', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })

    const store = await loadStore()
    store.getState().setRefusal(pending)

    expect(store.getState().refusal).toEqual(pending)
  })
})
