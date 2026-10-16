/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { NavGroup } from '@/components/layout/components/nav-group'
import { SidebarProvider } from '@/components/ui/sidebar'
import { pendingEntryCountQueryOptions } from '@/features/wind-tide/api'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { useOwnerViewStore, type OwnerView } from '@/stores/owner-view-store'

import { useSidebarView } from '../use-sidebar-view'

const ENTRY_TITLE = 'Allowlist'
const COUNT_URL = '/api/wt/entry-requests/pending-count'

/** The console sidebar's admin group, rendered by the real nav components. */
function AdminNav() {
  const { navGroups } = useSidebarView()
  return (
    <SidebarProvider>
      <span>sidebar ready</span>
      {navGroups
        .filter((group) => group.id === 'admin')
        .map((group) => (
          <NavGroup key={group.id} {...group} />
        ))}
    </SidebarProvider>
  )
}

async function renderSidebarFor(
  role: number,
  view: OwnerView,
  pendingCount: number
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(['status'], { SidebarModulesAdmin: '' })
  useAuthStore.getState().auth.setUser({ id: 1, username: 'someone', role })
  useOwnerViewStore.setState({ view })
  const get = vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === COUNT_URL) {
      return { data: { success: true, data: { count: pendingCount } } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: AdminNav }),
    history: createMemoryHistory({ initialEntries: ['/dashboard/overview'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByText('sidebar ready')
  return { client, get }
}

afterEach(() => {
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
  useOwnerViewStore.setState({ view: 'admin' })
})

describe('allowlist entry in the sidebar', () => {
  test('the owner in the admin view sees the pending entry request count on it', async () => {
    await renderSidebarFor(ROLE.SUPER_ADMIN, 'admin', 3)

    const link = await screen.findByRole('link', {
      name: new RegExp(ENTRY_TITLE),
    })
    expect(link).toHaveAttribute('href', '/allowlist')
    expect(await within(link).findByText('3')).toBeInTheDocument()
  })

  test('with no pending entry requests the entry shows no count', async () => {
    const { client } = await renderSidebarFor(ROLE.SUPER_ADMIN, 'admin', 0)

    await waitFor(() =>
      expect(
        client.getQueryState(pendingEntryCountQueryOptions.queryKey)?.status
      ).toBe('success')
    )
    expect(screen.getByRole('link', { name: ENTRY_TITLE })).toHaveTextContent(
      /^Allowlist$/
    )
  })

  test.each([
    ['a member', ROLE.USER, 'admin'],
    ['an admin who is not the owner', ROLE.ADMIN, 'admin'],
    ['the owner in the member view', ROLE.SUPER_ADMIN, 'member'],
  ] as const)(
    '%s gets no entry and never asks for the count',
    async (_who, role, view) => {
      const { get } = await renderSidebarFor(role, view, 3)

      expect(
        screen.queryByRole('link', { name: new RegExp(ENTRY_TITLE) })
      ).toBeNull()
      expect(get).not.toHaveBeenCalledWith(COUNT_URL)
    }
  )
})
