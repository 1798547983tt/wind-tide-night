/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { useOwnerViewStore } from '@/stores/owner-view-store'

import { MemberViewBanner } from '../member-view-banner'
import { OwnerViewSwitch } from '../owner-view-switch'

const navigate = vi.fn()

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useNavigate: () => navigate,
}))

function signIn(role: number) {
  useAuthStore.getState().auth.setUser({ id: 1, username: 'someone', role })
}

afterEach(() => {
  navigate.mockReset()
  useAuthStore.getState().auth.reset()
  useOwnerViewStore.setState({ view: 'admin' })
  window.localStorage.clear()
})

describe('owner view switch', () => {
  test('a member is not offered the view switch', () => {
    signIn(ROLE.USER)

    render(<OwnerViewSwitch />)

    expect(screen.queryByRole('button', { name: 'Member view' })).toBeNull()
  })

  test('the owner starts with the admin view selected', () => {
    signIn(ROLE.SUPER_ADMIN)

    render(<OwnerViewSwitch />)

    expect(screen.getByRole('button', { name: 'Admin view' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
    expect(screen.getByRole('button', { name: 'Member view' })).toHaveAttribute(
      'aria-pressed',
      'false'
    )
  })

  test('a phone-width header keeps both views named while their labels collapse', () => {
    signIn(ROLE.SUPER_ADMIN)

    render(<OwnerViewSwitch />)

    for (const name of ['Admin view', 'Member view']) {
      const item = screen.getByRole('button', { name })
      expect(within(item).getByText(name)).toHaveClass('hidden', 'sm:inline')
    }
  })

  test('choosing the member view switches the console and opens the dashboard', async () => {
    signIn(ROLE.SUPER_ADMIN)
    render(<OwnerViewSwitch />)

    await userEvent.click(screen.getByRole('button', { name: 'Member view' }))

    expect(useOwnerViewStore.getState().view).toBe('member')
    expect(navigate).toHaveBeenCalledWith({ to: '/dashboard' })
  })
})

describe('member view banner', () => {
  test('the owner in the admin view sees no reminder', () => {
    signIn(ROLE.SUPER_ADMIN)

    render(<MemberViewBanner />)

    expect(screen.queryByRole('status')).toBeNull()
  })

  test('the reminder in the member view takes the owner back to the admin view', async () => {
    signIn(ROLE.SUPER_ADMIN)
    useOwnerViewStore.setState({ view: 'member' })
    render(<MemberViewBanner />)
    expect(screen.getByRole('status')).toHaveTextContent(
      'You are browsing as a member'
    )

    await userEvent.click(
      screen.getByRole('button', { name: 'Back to admin view' })
    )

    expect(useOwnerViewStore.getState().view).toBe('admin')
    expect(screen.queryByRole('status')).toBeNull()
  })
})
