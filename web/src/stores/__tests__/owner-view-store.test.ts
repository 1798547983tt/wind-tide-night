/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

const VIEW_KEY = 'wt.owner.view'

// The store reads the saved choice when its module loads, so each case loads
// a fresh copy after arranging storage.
async function loadStore() {
  vi.resetModules()
  const module = await import('../owner-view-store')
  return module.useOwnerViewStore
}

beforeEach(() => {
  window.localStorage.clear()
})

afterEach(() => {
  vi.restoreAllMocks()
  window.localStorage.clear()
})

describe('owner view store', () => {
  test('a browser without a saved choice starts on the admin view', async () => {
    const store = await loadStore()

    expect(store.getState().view).toBe('admin')
  })

  test('a browser that saved the member view starts on it', async () => {
    window.localStorage.setItem(VIEW_KEY, 'member')

    const store = await loadStore()

    expect(store.getState().view).toBe('member')
  })

  test('choosing a view remembers it for the next visit', async () => {
    const store = await loadStore()

    store.getState().setView('member')

    expect(window.localStorage.getItem(VIEW_KEY)).toBe('member')
  })

  test('unavailable storage still switches the view for this visit', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('storage disabled')
    })

    const store = await loadStore()
    expect(store.getState().view).toBe('admin')
    store.getState().setView('member')

    expect(store.getState().view).toBe('member')
  })
})
