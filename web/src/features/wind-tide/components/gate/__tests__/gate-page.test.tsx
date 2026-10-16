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
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { Route as GateRoute } from '@/routes/(auth)/gate'
import { useGateStore } from '@/stores/gate-store'

import type { GateRefusal } from '../../../types'
import { GatePage } from '../gate-page'

const HOUR = 3600

function refusal(gate: GateRefusal['wt_gate'], retryAfter = 0): GateRefusal {
  return {
    wt_gate: gate,
    applicant: {
      discord_id: '200000000000000002',
      username: 'tidewalker',
      display_name: 'Tide Walker',
      avatar_url: '',
    },
    requested_at: 1_760_000_000,
    retry_after: retryAfter,
  }
}

function nowSeconds() {
  return Math.floor(Date.now() / 1000)
}

function openGate() {
  const root = createRootRoute()
  const gate = createRoute({
    getParentRoute: () => root,
    path: 'gate',
    component: GatePage,
    // The app's guard, typed for the app's root; it reads none of its options.
    beforeLoad: () => GateRoute.options.beforeLoad?.({} as never),
  })
  const signIn = createRoute({
    getParentRoute: () => root,
    path: 'sign-in',
    component: () => <p>sign-in page</p>,
  })
  const router = createRouter({
    routeTree: root.addChildren([gate, signIn]),
    history: createMemoryHistory({ initialEntries: ['/gate'] }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  // A fresh status entry, so the page reads it without fetching.
  client.setQueryData(['status'], {
    discord_oauth: true,
    discord_client_id: 'discord-client',
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

describe('gate page', () => {
  test('a pending applicant sees who signed in, when they asked, and how to get in once approved', async () => {
    useGateStore.setState({ refusal: refusal('pending') })

    openGate()

    expect(
      await screen.findByRole('heading', {
        name: 'Waiting for the site owner to approve you',
      })
    ).toBeVisible()
    expect(screen.getByText('Tide Walker')).toBeVisible()
    expect(screen.getByText('@tidewalker')).toBeVisible()
    expect(screen.getByText(/^Requested at \d{4}-\d{2}-\d{2} /)).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Sign in with Discord again' })
    ).toBeEnabled()
  })

  test('a rejected applicant inside the waiting period sees when they may ask again', async () => {
    useGateStore.setState({
      refusal: refusal('rejected', nowSeconds() + HOUR),
    })

    openGate()

    expect(
      await screen.findByRole('heading', {
        name: 'Your entry request was not approved',
      })
    ).toBeVisible()
    expect(
      screen.getByText(
        /^You can send a new request after \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.$/
      )
    ).toBeVisible()
  })

  test('a rejected applicant past the waiting period is told to ask again now', async () => {
    useGateStore.setState({
      refusal: refusal('rejected', nowSeconds() - HOUR),
    })

    openGate()

    expect(
      await screen.findByText(
        'You can send a new request now by signing in with Discord again.'
      )
    ).toBeVisible()
  })

  test('a long display name wraps instead of widening a phone screen', async () => {
    const long = 'Tide Walker of the Long Night Shore Who Never Sleeps'
    const applicant = { ...refusal('pending').applicant, display_name: long }
    useGateStore.setState({ refusal: { ...refusal('pending'), applicant } })

    openGate()

    const name = await screen.findByText(long)
    expect(name).toHaveClass('wrap-anywhere')
    expect(name).not.toHaveClass('truncate')
  })

  test('a removed member is pointed to the owner without a sign-in button', async () => {
    useGateStore.setState({ refusal: refusal('removed') })

    openGate()

    expect(
      await screen.findByRole('heading', {
        name: 'Your access to this site has been removed',
      })
    ).toBeVisible()
    expect(
      screen.getByText('Only the site owner can restore your access.')
    ).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Sign in with Discord again' })
    ).toBeNull()
  })

  test('signing in again sends the applicant to Discord with a fresh sign-in state', async () => {
    useGateStore.setState({ refusal: refusal('pending') })
    vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/oauth/state') {
        return { data: { success: true, data: 'fresh-state' } }
      }
      return { data: { success: true, message: '' } }
    })
    const open = vi.spyOn(window, 'open').mockReturnValue(null)

    openGate()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Sign in with Discord again' })
    )

    await waitFor(() => expect(open).toHaveBeenCalledTimes(1))
    const target = new URL(String(open.mock.calls[0][0]))
    expect(target.origin + target.pathname).toBe(
      'https://discord.com/oauth2/authorize'
    )
    expect(target.searchParams.get('client_id')).toBe('discord-client')
    expect(target.searchParams.get('state')).toBe('fresh-state')
  })

  test('a visit without a refusal goes to the sign-in page', async () => {
    const router = openGate()

    await waitFor(() => expect(router.state.location.pathname).toBe('/sign-in'))
    expect(screen.getByText('sign-in page')).toBeVisible()
  })
})
