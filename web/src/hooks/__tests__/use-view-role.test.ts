/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { useOwnerViewStore } from '@/stores/owner-view-store'

import { useViewRole } from '../use-view-role'

function signIn(role: number) {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'someone', role })
}

afterEach(() => {
  useAuthStore.getState().auth.reset()
  useOwnerViewStore.setState({ view: 'admin' })
  window.localStorage.clear()
})

describe('useViewRole', () => {
  test('the owner in the admin view gets the owner console', () => {
    signIn(ROLE.SUPER_ADMIN)

    const { result } = renderHook(() => useViewRole())

    expect(result.current).toBe(ROLE.SUPER_ADMIN)
  })

  test('the owner in the member view gets the member console', () => {
    signIn(ROLE.SUPER_ADMIN)
    useOwnerViewStore.setState({ view: 'member' })

    const { result } = renderHook(() => useViewRole())

    expect(result.current).toBe(ROLE.USER)
  })

  test('an admin who is not the owner keeps the admin console in the member view', () => {
    signIn(ROLE.ADMIN)
    useOwnerViewStore.setState({ view: 'member' })

    const { result } = renderHook(() => useViewRole())

    expect(result.current).toBe(ROLE.ADMIN)
  })

  test('switching back to the admin view restores the owner console at once', () => {
    signIn(ROLE.SUPER_ADMIN)
    useOwnerViewStore.setState({ view: 'member' })
    const { result } = renderHook(() => useViewRole())

    act(() => {
      useOwnerViewStore.getState().setView('admin')
    })

    expect(result.current).toBe(ROLE.SUPER_ADMIN)
  })
})
