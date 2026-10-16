/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

const SEEN_KEY = 'wt.opening.seen'

// The store reads the session flag when its module loads, so each case loads
// a fresh copy after arranging storage.
async function loadStore() {
  vi.resetModules()
  const module = await import('../opening-store')
  return module.useOpeningStore
}

beforeEach(() => {
  window.sessionStorage.clear()
})

afterEach(() => {
  vi.restoreAllMocks()
  window.sessionStorage.clear()
})

describe('opening store', () => {
  test('a new browser session starts with the opening playing', async () => {
    const store = await loadStore()

    expect(store.getState().stage).toBe('playing')
  })

  test('a session that has already seen the opening starts with it done', async () => {
    window.sessionStorage.setItem(SEEN_KEY, '1')

    const store = await loadStore()

    expect(store.getState().stage).toBe('done')
  })

  test('finishing the opening remembers it for the rest of the session', async () => {
    const store = await loadStore()

    store.getState().wash()
    expect(store.getState().stage).toBe('washing')
    store.getState().finish()

    expect(store.getState().stage).toBe('done')
    expect(window.sessionStorage.getItem(SEEN_KEY)).toBe('1')
  })

  test('unavailable storage still plays the opening and lets it finish', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })

    const store = await loadStore()
    expect(store.getState().stage).toBe('playing')
    store.getState().finish()

    expect(store.getState().stage).toBe('done')
  })
})
