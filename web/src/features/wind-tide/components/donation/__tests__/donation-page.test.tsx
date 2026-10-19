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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type {
  AntigravityDonation,
  Donation,
  DonationPool,
} from '../../../types'
import { DonationPage } from '../donation-page'

let items: Donation[]
let listCalls: number

function pool(overrides: Partial<DonationPool> = {}): DonationPool {
  return {
    state: 'ok',
    message: '',
    retry_at: 0,
    success: 12,
    failed: 1,
    ...overrides,
  }
}

function donation(
  id: number,
  email: string,
  overrides: Partial<AntigravityDonation> = {}
): Donation {
  return {
    id,
    provider: 'antigravity',
    email,
    status: 'active',
    fail_reason: '',
    pool: pool(),
    last_test: null,
    created_at: 1760000000 + id,
    verified_at: 1760000100 + id,
    ...overrides,
  }
}

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const root = createRootRoute()
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
  })
  const page = createRoute({
    getParentRoute: () => authenticated,
    path: '/donations/',
    component: DonationPage,
  })
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren([page])]),
    history: createMemoryHistory({ initialEntries: ['/donations'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

function cardOf(email: string) {
  const card = screen.getByText(email).closest('[data-slot="card"]')
  if (!card) throw new Error(`No card for ${email}`)
  return card as HTMLElement
}

beforeEach(() => {
  items = []
  listCalls = 0
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/wt/donations') {
      listCalls += 1
      return { data: { success: true, data: { max: 10, items } } }
    }
    if (/^\/api\/wt\/donations\/\d+\/usage$/.test(url)) {
      return {
        data: {
          success: true,
          data: {
            fetched_at: Math.floor(Date.now() / 1000),
            tier: 'pro',
            groups: [
              {
                name: 'Gemini 3 Pro',
                buckets: [
                  {
                    name: 'Gemini 3 Pro',
                    window: '5h',
                    remaining_fraction: 0.73,
                    reset_at: 1760003600,
                  },
                ],
              },
            ],
          },
        },
      }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/wt/donations/1/test') {
      return {
        data: {
          success: true,
          data: { ok: true, latency_ms: 850, message: '', usage: null },
        },
      }
    }
    if (url === '/api/wt/donations/1/remove') {
      return { data: { success: true, message: 'Donation removed' } }
    }
    throw new Error(`Unexpected POST ${url}`)
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the donated accounts page', () => {
  test('with nothing donated it shows the slogan, the empty state and an enabled button', async () => {
    renderPage()
    expect(
      await screen.findByText('No donated accounts yet')
    ).toBeInTheDocument()
    expect(screen.getByText(/all for one and one for all/)).toBeInTheDocument()
    expect(screen.getByText('0 / 10 donated')).toBeInTheDocument()
    // The list is the Antigravity pool's, named as such for the server.
    expect(api.get).toHaveBeenCalledWith('/api/wt/donations', {
      params: { provider: 'antigravity' },
    })
    expect(
      screen.getByRole('button', { name: 'Donate an account' })
    ).toBeEnabled()
  })

  test('every pool state has its badge, and only the right cards offer a re-authorization', async () => {
    items = [
      donation(1, 'fine@example.com', {
        last_test: { at: 1760000500, ok: true, latency_ms: 850, message: '' },
      }),
      donation(2, 'cooling@example.com', {
        pool: pool({
          state: 'cooldown',
          message: 'quota exhausted',
          retry_at: 1760000600,
        }),
      }),
      donation(3, 'revoked@example.com', {
        pool: pool({ state: 'token_invalid', message: 'invalid_grant' }),
      }),
      donation(4, 'gone@example.com', { pool: pool({ state: 'missing' }) }),
      donation(5, 'off@example.com', { pool: pool({ state: 'disabled' }) }),
      donation(6, 'dark@example.com', { pool: pool({ state: 'unknown' }) }),
      donation(7, 'pending@example.com', {
        status: 'verifying',
        pool: null,
        verified_at: 0,
      }),
      donation(8, 'denied@example.com', {
        status: 'failed',
        pool: null,
        fail_reason: 'Google denied the authorization',
        verified_at: 0,
      }),
    ]
    renderPage()
    await screen.findByText('fine@example.com')

    expect(
      await within(cardOf('fine@example.com')).findByText('In service')
    ).toBeInTheDocument()
    expect(
      within(cardOf('fine@example.com')).getByText(/passed in 850 ms/)
    ).toBeInTheDocument()
    expect(
      within(cardOf('fine@example.com')).getByText(/12 succeeded · 1 failed/)
    ).toBeInTheDocument()
    expect(
      within(cardOf('cooling@example.com')).getByText('Cooling down')
    ).toBeInTheDocument()
    expect(
      within(cardOf('cooling@example.com')).getByText(/Back at /)
    ).toBeInTheDocument()
    expect(
      within(cardOf('cooling@example.com')).getByText('quota exhausted')
    ).toBeInTheDocument()
    expect(
      within(cardOf('revoked@example.com')).getByText('Token invalid')
    ).toBeInTheDocument()
    expect(
      within(cardOf('gone@example.com')).getByText('Not in the pool')
    ).toBeInTheDocument()
    expect(
      within(cardOf('off@example.com')).getByText('Disabled')
    ).toBeInTheDocument()
    expect(
      within(cardOf('dark@example.com')).getByText('Pool unreachable')
    ).toBeInTheDocument()
    expect(
      within(cardOf('pending@example.com')).getByText('Verifying')
    ).toBeInTheDocument()
    expect(
      within(cardOf('denied@example.com')).getByText('Failed')
    ).toBeInTheDocument()
    expect(
      within(cardOf('denied@example.com')).getByText(
        'Google denied the authorization'
      )
    ).toBeInTheDocument()

    expect(
      screen.getAllByRole('button', { name: 'Authorize again' })
    ).toHaveLength(2)
    expect(screen.getAllByRole('button', { name: 'Retry' })).toHaveLength(1)
    // Six active cards can be tested; verifying and failed ones cannot.
    expect(screen.getAllByRole('button', { name: 'Test' })).toHaveLength(6)
    // The failed row holds no slot.
    expect(screen.getByText('7 / 10 donated')).toBeInTheDocument()
    // Usage is fetched for working and cooling accounts only.
    expect(
      await within(cardOf('fine@example.com')).findByText(/73% left/)
    ).toBeInTheDocument()
    expect(
      within(cardOf('fine@example.com')).getByText(/Pro tier/)
    ).toBeInTheDocument()
    expect(
      within(cardOf('revoked@example.com')).queryByText(/left/)
    ).not.toBeInTheDocument()
  })

  test('Test calls the server and refreshes the list', async () => {
    items = [donation(1, 'fine@example.com')]
    renderPage()
    await screen.findByText('fine@example.com')
    const before = listCalls
    await userEvent.click(
      within(cardOf('fine@example.com')).getByRole('button', { name: 'Test' })
    )
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/wt/donations/1/test')
    )
    await waitFor(() => expect(listCalls).toBeGreaterThan(before))
  })

  test('Remove asks for confirmation first, then calls the server', async () => {
    items = [donation(1, 'fine@example.com')]
    renderPage()
    await screen.findByText('fine@example.com')
    await userEvent.click(
      within(cardOf('fine@example.com')).getByRole('button', { name: 'Remove' })
    )
    expect(api.post).not.toHaveBeenCalled()
    const confirm = await screen.findByRole('alertdialog')
    expect(
      within(confirm).getByText('Remove this donated account?')
    ).toBeInTheDocument()
    await userEvent.click(
      within(confirm).getByRole('button', { name: 'Remove' })
    )
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/wt/donations/1/remove')
    )
  })

  test('at ten slots the donate button is disabled', async () => {
    items = Array.from({ length: 10 }, (_, index) =>
      donation(index + 1, `spare${index + 1}@example.com`)
    )
    renderPage()
    expect(await screen.findByText('10 / 10 donated')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Donate an account' })
    ).toBeDisabled()
  })
})
