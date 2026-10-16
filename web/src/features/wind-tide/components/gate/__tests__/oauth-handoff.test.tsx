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
import { render, waitFor } from '@testing-library/react'
import { toast } from 'sonner'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { Route as OAuthCallbackRoute } from '@/routes/oauth/$provider'
import { useGateStore } from '@/stores/gate-store'

import type { GateRefusal } from '../../../types'

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

// The upstream callback page, mounted at its own path so its params resolve.
function openDiscordCallback() {
  const root = createRootRoute()
  const callback = createRoute({
    getParentRoute: () => root,
    path: 'oauth/$provider',
    component: OAuthCallbackRoute.options.component,
  })
  const gate = createRoute({
    getParentRoute: () => root,
    path: 'gate',
    component: () => <p>gate page</p>,
  })
  const signIn = createRoute({
    getParentRoute: () => root,
    path: 'sign-in',
    component: () => <p>sign-in page</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([callback, gate, signIn]),
    history: createMemoryHistory({
      initialEntries: ['/oauth/discord?code=discord-code&state=login-state'],
    }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return router
}

afterEach(() => {
  vi.restoreAllMocks()
  useGateStore.setState({ refusal: null })
  window.sessionStorage.clear()
})

describe('Discord sign-in handoff to the gate', () => {
  test('an account the gate turns away lands on the gate page instead of an error', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: false,
        message: 'Your entry request is waiting for approval',
        data: pending,
      },
    })
    const error = vi.spyOn(toast, 'error')

    const router = openDiscordCallback()

    await waitFor(() => expect(router.state.location.pathname).toBe('/gate'))
    expect(useGateStore.getState().refusal).toEqual(pending)
    expect(error).not.toHaveBeenCalled()
  })

  test('any other failed sign-in still reports the error and returns to sign-in', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: false, message: 'User has been banned' },
    })
    const error = vi.spyOn(toast, 'error')

    const router = openDiscordCallback()

    await waitFor(() => expect(router.state.location.pathname).toBe('/sign-in'))
    expect(error).toHaveBeenCalled()
    expect(useGateStore.getState().refusal).toBeNull()
  })
})
