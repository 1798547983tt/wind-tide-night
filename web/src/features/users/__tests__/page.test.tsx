/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { TooltipProvider } from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { Route as UsersRoute } from '@/routes/_authenticated/users/index'
import { useAuthStore } from '@/stores/auth-store'

import { Users } from '../index'
import type { User } from '../types'

// wind-tide: accounts come from Discord sign-in only and have no password
// (wind-tide/API.md §3). The server refuses creating an account or setting a
// password, so the users page offers neither.

const member: User = {
  id: 2,
  username: 'shio',
  display_name: '',
  role: 1,
  status: 1,
  quota: 0,
  used_quota: 0,
  request_count: 0,
  group: 'default',
}

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  localStorage.clear()
  useAuthStore.getState().auth.setUser({ id: 1, username: 'root', role: 100 })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/group/') {
      return { data: { success: true, data: ['default'] } }
    }
    if (url === `/api/user/${member.id}`) {
      return { data: { success: true, data: member } }
    }
    if (url === '/api/authz/catalog') {
      return { data: { success: true, data: { resources: [], roles: [] } } }
    }
    return { data: { success: true, data: { items: [member], total: 1 } } }
  })
})

afterEach(() => {
  cleanup()
  localStorage.clear()
  useAuthStore.getState().auth.reset()
  vi.restoreAllMocks()
})

async function renderUsersPage() {
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const users = createRoute({
    getParentRoute: () => auth,
    path: 'users/',
    validateSearch: UsersRoute.options.validateSearch,
    component: () => (
      <TooltipProvider>
        <Users />
      </TooltipProvider>
    ),
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([users])]),
    history: createMemoryHistory({ initialEntries: ['/users/'] }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  await router.load()
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByText('shio')
}

it('with accounts coming from Discord sign-in only, the page offers no way to add a user', async () => {
  await renderUsersPage()

  expect(screen.queryByRole('button', { name: 'Add User' })).toBeNull()
})

it('editing a user offers no password to reset, as no account has one', async () => {
  await renderUsersPage()

  await userEvent.click(screen.getByRole('button', { name: 'Edit' }))

  expect(await screen.findByDisplayValue('shio')).toBeInTheDocument()
  expect(screen.queryByLabelText('Password')).toBeNull()
})
