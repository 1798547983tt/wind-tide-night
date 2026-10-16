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
import { render, screen, waitFor } from '@testing-library/react'
import { AxiosError, AxiosHeaders } from 'axios'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore, type AuthBundle } from '@/stores/auth-store'

import { EmergencyLoginPage } from '../emergency-login-page'

const ownerBundle: AuthBundle = {
  access_token: 'access-token',
  token_type: 'Bearer',
  access_expires_at: Math.floor(Date.now() / 1000) + 600,
  user: { id: 1, username: 'owner', role: 100 },
  session: {
    sid: 'session-owner',
    current: true,
    login_method: 'emergency',
    ip: '127.0.0.1',
    user_agent: 'test',
    created_at: 100,
    last_active_at: 100,
    expires_at: 1000,
  },
}

function openLink(fragment: string) {
  window.history.replaceState(null, '', `/emergency-login${fragment}`)
  const root = createRootRoute()
  const page = createRoute({
    getParentRoute: () => root,
    path: 'emergency-login',
    component: EmergencyLoginPage,
  })
  const dashboard = createRoute({
    getParentRoute: () => root,
    path: 'dashboard',
    component: () => <p>dashboard</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([page, dashboard]),
    history: createMemoryHistory({ initialEntries: ['/emergency-login'] }),
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
  useAuthStore.getState().auth.reset()
  window.history.replaceState(null, '', '/')
})

describe('emergency sign-in page', () => {
  test('a valid link signs the owner in once and clears the token from the address', async () => {
    const post = vi
      .spyOn(api, 'post')
      .mockResolvedValue({ data: { success: true, data: ownerBundle } })

    const router = openLink('#token=one-time-token')

    await waitFor(() =>
      expect(router.state.location.pathname).toBe('/dashboard')
    )
    expect(post).toHaveBeenCalledTimes(1)
    expect(post).toHaveBeenCalledWith(
      '/api/wt/emergency-login',
      { token: 'one-time-token' },
      expect.objectContaining({ skipAuthRefresh: true })
    )
    expect(window.location.hash).toBe('')
    expect(useAuthStore.getState().auth.user?.role).toBe(100)
  })

  test('a used or expired link explains how to get a new one', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({
      data: { success: false, message: 'Emergency login failed' },
    })

    openLink('#token=spent-token')

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'This emergency link is invalid or has expired.'
    )
    expect(
      screen.getByRole('button', { name: 'Back to sign-in' })
    ).toBeVisible()
    expect(window.location.hash).toBe('')
  })

  test('a failed request is treated as an unusable link', async () => {
    vi.spyOn(api, 'post').mockRejectedValue(new Error('Network Error'))

    openLink('#token=unanswered-token')

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'This emergency link is invalid or has expired.'
    )
  })

  test('a rate-limited attempt says to open the same link again later', async () => {
    // The rate limit answers before the server looks at the link, so the
    // link is not used up.
    vi.spyOn(api, 'post').mockRejectedValue(
      new AxiosError(
        'Request failed with status code 429',
        AxiosError.ERR_BAD_REQUEST,
        undefined,
        undefined,
        {
          status: 429,
          statusText: 'Too Many Requests',
          data: '',
          headers: {},
          config: { headers: new AxiosHeaders() },
        }
      )
    )

    openLink('#token=unspent-token')

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(
      'Too many sign-in attempts from this network.'
    )
    expect(alert).toHaveTextContent('Wait a while, then open the link again.')
    expect(alert).not.toHaveTextContent('invalid or has expired')
  })

  test('a link without a token never contacts the server', async () => {
    const post = vi.spyOn(api, 'post')

    openLink('')

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'This emergency link is incomplete.'
    )
    expect(post).not.toHaveBeenCalled()
  })
})
