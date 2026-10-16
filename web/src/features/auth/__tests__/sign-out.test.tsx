/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { SignOutDialog } from '@/components/sign-out-dialog'
import { api } from '@/lib/api'
import { Route as RootRoute } from '@/routes/__root'
import { Route as AuthenticatedRoute } from '@/routes/_authenticated/route'
import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

// The console chrome is beside the point; its pages render straight through.
vi.mock('@/components/layout', async () => {
  const { Outlet } = await import('@tanstack/react-router')
  return { AuthenticatedLayout: () => <Outlet /> }
})

const member: AuthBundle = {
  access_token: 'access-token',
  token_type: 'Bearer',
  access_expires_at: Math.floor(Date.now() / 1000) + 600,
  user: { id: 7, username: 'member', role: 1 },
  session: {
    sid: 'session-member',
    current: true,
    login_method: 'discord',
    ip: '127.0.0.1',
    user_agent: 'test',
    created_at: 100,
    last_active_at: 100,
    expires_at: 1000,
  },
}

afterEach(() => {
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

test('signing out leaves the console without fetching its data again without a session', async () => {
  vi.spyOn(api, 'get').mockResolvedValue({ data: { success: true, data: {} } })
  vi.spyOn(api, 'post').mockResolvedValue({
    data: { success: true, message: '' },
  })
  useAuthStore.getState().auth.setBundle(member)
  // Stands in for a console page's data, which needs a session to load.
  const loadUsage = vi.fn(async () => 'usage')
  const root = createRootRoute({ component: RootRoute.options.component })
  const signedInArea = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
    component: AuthenticatedRoute.options.component,
  })
  const dashboard = createRoute({
    getParentRoute: () => signedInArea,
    path: 'dashboard',
    component: function UsagePage() {
      // Like the dashboard, the page follows the signed-in user.
      const user = useAuthStore((state) => state.auth.user)
      const usage = useQuery({ queryKey: ['usage'], queryFn: loadUsage })
      return (
        <>
          <p>{user?.username}</p>
          <p>{usage.data}</p>
          <SignOutDialog open onOpenChange={() => undefined} />
        </>
      )
    },
  })
  const signIn = createRoute({
    getParentRoute: () => root,
    path: 'sign-in',
    component: () => <p>sign-in page</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([
      signedInArea.addChildren([dashboard]),
      signIn,
    ]),
    history: createMemoryHistory({ initialEntries: ['/dashboard'] }),
  })
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  expect(await screen.findByText('usage')).toBeVisible()

  await userEvent.click(screen.getByRole('button', { name: 'Sign out' }))

  expect(await screen.findByText('sign-in page')).toBeVisible()
  await waitFor(() => expect(router.state.location.pathname).toBe('/sign-in'))
  expect(loadUsage).toHaveBeenCalledTimes(1)
})
