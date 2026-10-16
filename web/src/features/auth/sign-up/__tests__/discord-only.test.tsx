/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { Route as SignUpRoute } from '@/routes/(auth)/sign-up'

// The app's sign-up guard in front of stand-in pages.
function openSignUp(status: Record<string, unknown>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  // A fresh status entry, so the guard reads it without fetching.
  client.setQueryData(['status'], status)
  const root = createRootRouteWithContext<{ queryClient: QueryClient }>()()
  const signUp = createRoute({
    getParentRoute: () => root,
    path: 'sign-up',
    component: () => <p>sign-up page</p>,
    beforeLoad: (options) => SignUpRoute.options.beforeLoad?.(options as never),
  })
  const signIn = createRoute({
    getParentRoute: () => root,
    path: 'sign-in',
    component: () => <p>sign-in page</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([signUp, signIn]),
    context: { queryClient: client },
    history: createMemoryHistory({ initialEntries: ['/sign-up'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return router
}

describe('sign-up page of a Discord-only site', () => {
  test('a visit goes to the sign-in page, where Discord also asks for entry', async () => {
    const router = openSignUp({ wt_discord_only: true, register_enabled: true })

    await waitFor(() => expect(router.state.location.pathname).toBe('/sign-in'))
    expect(screen.getByText('sign-in page')).toBeVisible()
  })

  test('a site that takes registrations still opens the sign-up page', async () => {
    openSignUp({ wt_discord_only: false, register_enabled: true })

    expect(await screen.findByText('sign-up page')).toBeVisible()
  })
})
