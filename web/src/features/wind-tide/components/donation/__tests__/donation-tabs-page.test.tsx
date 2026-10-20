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
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { donationsSearchSchema } from '../../../lib/donation-tabs'
import type { PoolOverview } from '../../../types'
import { DonationTabsPage } from '../donation-tabs-page'

type Flags = {
  donation?: boolean
  ais?: boolean
  build?: boolean
  wb?: boolean
}

function overview(): PoolOverview {
  return {
    antigravity: {
      fetched_at: Math.floor(Date.now() / 1000) - 60,
      accounts: 4,
      healthy: 3,
      buckets: [
        { name: 'Gemini 3 Pro', window: '5h', average_remaining_fraction: 0.8 },
      ],
    },
    aistudio: {
      provider: 'aistudio',
      day: '2026-10-19',
      accounts: 1,
      models: [
        {
          provider: 'aistudio',
          model: 'gemini-4-argon',
          relay_model: 'AIS PG-gemini-4-argon',
          per_account: 50,
          take_max: 100,
          allowance: 50,
          calls: 5,
          failed_today: 0,
          given: 0,
          taken: 0,
          remaining: 45,
          take_left: 100,
          pool_balance: 20,
          pool_given: 20,
          pool_taken: 0,
        },
      ],
    },
    build: null,
    workbuddy: null,
    events: [],
  }
}

function renderPage(flags: Flags, initialEntry = '/donations') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  // The status query is seeded (and fresh for five minutes), so the page
  // never asks the server for it.
  client.setQueryData(['status'], {
    wt_donation_enabled: flags.donation === true,
    wt_ais_enabled: flags.ais === true,
    wt_build_enabled: flags.build === true,
    wt_wb_enabled: flags.wb === true,
  })
  const root = createRootRoute()
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
  })
  const page = createRoute({
    getParentRoute: () => authenticated,
    path: '/donations/',
    validateSearch: donationsSearchSchema,
    component: DonationTabsPage,
  })
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren([page])]),
    history: createMemoryHistory({ initialEntries: [initialEntry] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return router
}

function tabNames() {
  return screen.queryAllByRole('tab').map((tab) => tab.textContent?.trim())
}

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/wt/donations') {
      return { data: { success: true, data: { max: 10, items: [] } } }
    }
    if (url === '/api/wt/pool') {
      return { data: { success: true, data: overview() } }
    }
    if (url === '/api/wt/pool/events') {
      return { data: { success: true, data: { items: [] } } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the tabs of the donated accounts page', () => {
  test('with every pool on, all five tabs show and Antigravity opens first', async () => {
    renderPage({ donation: true, ais: true, build: true, wb: true })
    expect(
      await screen.findByText('No donated accounts yet')
    ).toBeInTheDocument()
    expect(tabNames()).toEqual([
      'Antigravity',
      'AIS playground',
      'BUILD',
      'WorkBuddy',
      'Compute pool',
    ])
    expect(screen.getByRole('tab', { name: 'Antigravity' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(screen.getByText(/all for one and one for all/)).toBeInTheDocument()
  })

  test('a tab in the address opens, and clicking another writes it back', async () => {
    const router = renderPage(
      { donation: true, ais: true },
      '/donations?tab=pool'
    )
    expect(
      await screen.findByRole('heading', { name: 'Compute pool' })
    ).toBeInTheDocument()
    expect(
      await screen.findByText('3 / 4 accounts healthy')
    ).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Compute pool' })).toHaveAttribute(
      'aria-selected',
      'true'
    )

    await userEvent.click(screen.getByRole('tab', { name: 'AIS playground' }))
    expect(
      await screen.findByText('No AI Studio accounts yet')
    ).toBeInTheDocument()
    await waitFor(() =>
      expect(router.state.location.search).toEqual({ tab: 'aistudio' })
    )
    expect(api.get).toHaveBeenCalledWith('/api/wt/donations', {
      params: { provider: 'aistudio' },
    })
  })

  test('the BUILD tab opens its own page, which asks for its own donations', async () => {
    renderPage(
      { donation: true, ais: true, build: true },
      '/donations?tab=build'
    )
    expect(await screen.findByText('No BUILD accounts yet')).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'BUILD' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(
      screen.getByRole('button', { name: 'Donate a BUILD account' })
    ).toBeEnabled()
    expect(api.get).toHaveBeenCalledWith('/api/wt/donations', {
      params: { provider: 'build' },
    })
  })

  test('the WorkBuddy tab opens its own page, which asks for its own donations', async () => {
    renderPage(
      { donation: true, ais: true, wb: true },
      '/donations?tab=workbuddy'
    )
    expect(
      await screen.findByText('No WorkBuddy accounts yet')
    ).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'WorkBuddy' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    expect(
      screen.getByRole('button', { name: 'Donate a WorkBuddy account' })
    ).toBeEnabled()
    expect(api.get).toHaveBeenCalledWith('/api/wt/donations', {
      params: { provider: 'workbuddy' },
    })
    expect(screen.queryByRole('tab', { name: 'BUILD' })).not.toBeInTheDocument()
  })

  test('without the WorkBuddy pool its tab is absent and its address falls back', async () => {
    renderPage({ donation: true, build: true }, '/donations?tab=workbuddy')
    expect(
      await screen.findByText('No donated accounts yet')
    ).toBeInTheDocument()
    expect(tabNames()).toEqual(['Antigravity', 'BUILD', 'Compute pool'])
  })

  test('without the BUILD pool its tab is absent and its address falls back', async () => {
    renderPage({ donation: true, ais: true }, '/donations?tab=build')
    expect(
      await screen.findByText('No donated accounts yet')
    ).toBeInTheDocument()
    expect(tabNames()).toEqual([
      'Antigravity',
      'AIS playground',
      'Compute pool',
    ])
  })

  test('with only the Antigravity pool, the AIS tab is absent and its address falls back', async () => {
    renderPage({ donation: true }, '/donations?tab=aistudio')
    expect(
      await screen.findByText('No donated accounts yet')
    ).toBeInTheDocument()
    expect(tabNames()).toEqual(['Antigravity', 'Compute pool'])
  })

  test('with only the AI Studio pool, its page opens first', async () => {
    renderPage({ ais: true })
    expect(
      await screen.findByText('No AI Studio accounts yet')
    ).toBeInTheDocument()
    expect(tabNames()).toEqual(['AIS playground', 'Compute pool'])
  })

  test('with no pool there is nothing to switch, so no strip', async () => {
    renderPage({})
    expect(
      await screen.findByText('No donated accounts yet')
    ).toBeInTheDocument()
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
  })

  test('an unknown tab in the address is ignored', async () => {
    renderPage({ donation: true, ais: true }, '/donations?tab=nonsense')
    expect(
      await screen.findByText('No donated accounts yet')
    ).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Antigravity' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
  })
})
