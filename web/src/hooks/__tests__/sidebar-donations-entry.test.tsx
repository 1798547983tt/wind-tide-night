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
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { NavGroup } from '@/components/layout/components/nav-group'
import { SidebarProvider } from '@/components/ui/sidebar'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { useSidebarView } from '../use-sidebar-view'

/** The console sidebar's personal group, rendered by the real nav components. */
function PersonalNav() {
  const { navGroups } = useSidebarView()
  return (
    <SidebarProvider>
      <span>sidebar ready</span>
      {navGroups
        .filter((group) => group.id === 'personal')
        .map((group) => (
          <NavGroup key={group.id} {...group} />
        ))}
    </SidebarProvider>
  )
}

async function renderSidebar(donationEnabled: boolean) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  // The status query is seeded (and fresh for five minutes), so the hook
  // never asks the server for it.
  client.setQueryData(['status'], {
    SidebarModulesAdmin: '',
    wt_donation_enabled: donationEnabled,
  })
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'someone',
    role: ROLE.USER,
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    throw new Error(`Unexpected GET ${url}`)
  })
  const router = createRouter({
    routeTree: createRootRoute({ component: PersonalNav }),
    history: createMemoryHistory({ initialEntries: ['/wallet'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByText('sidebar ready')
}

afterEach(() => {
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

describe('the donated accounts entry in the sidebar', () => {
  test('sits right after Wallet when the site has an account pool', async () => {
    await renderSidebar(true)
    const names = (await screen.findAllByRole('link')).map((link) =>
      link.textContent?.trim()
    )
    const wallet = names.indexOf('Wallet')
    expect(wallet).toBeGreaterThanOrEqual(0)
    expect(names[wallet + 1]).toBe('Donated accounts')
    expect(
      screen.getByRole('link', { name: 'Donated accounts' })
    ).toHaveAttribute('href', '/donations')
  })

  test('is absent when the site has no account pool', async () => {
    await renderSidebar(false)
    expect(
      await screen.findByRole('link', { name: 'Wallet' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('link', { name: 'Donated accounts' })
    ).not.toBeInTheDocument()
  })
})
