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

import { allowlistSearchSchema } from '../../../lib/allowlist'
import type { EntryRequest } from '../../../types'
import { AllowlistPage } from '../allowlist-page'

const applicant: EntryRequest = {
  id: 3,
  discord_id: '123456789012345678',
  discord_username: 'kaze',
  display_name: 'Kaze',
  avatar_url: '',
  status: 'pending',
  requested_at: 1760000000,
  decided_at: 0,
}

let requests: EntryRequest[]

function renderAllowlistPage() {
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
    path: '/allowlist/',
    validateSearch: allowlistSearchSchema,
    component: AllowlistPage,
  })
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren([page])]),
    history: createMemoryHistory({ initialEntries: ['/allowlist'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

async function findRowOf(name: string) {
  const row = (await screen.findByText(name)).closest('tr')
  if (!row) throw new Error(`No table row for ${name}`)
  return row
}

/** The server's side of a decision: the request leaves the pending list. */
function decide(id: number, status: EntryRequest['status']) {
  requests = requests.map((request) =>
    request.id === id ? { ...request, status, decided_at: 1760000600 } : request
  )
  return { data: { success: true, data: requests.find((r) => r.id === id) } }
}

beforeEach(() => {
  requests = [applicant]
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    const pending = requests.filter((request) => request.status === 'pending')
    if (url === '/api/wt/entry-requests/pending-count') {
      return { data: { success: true, data: { count: pending.length } } }
    }
    if (url === '/api/wt/entry-requests') {
      const items = requests.filter(
        (request) => request.status === config?.params?.status
      )
      return {
        data: {
          success: true,
          data: { page: 1, page_size: 20, total: items.length, items },
        },
      }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('entry requests', () => {
  test('a pending request shows the applicant and approving it calls the API and refreshes the list', async () => {
    const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/wt/entry-requests/3/approve') {
        return decide(3, 'approved')
      }
      throw new Error(`Unexpected POST ${url}`)
    })
    renderAllowlistPage()

    const row = await findRowOf('Kaze')
    expect(within(row).getByText('@kaze')).toBeInTheDocument()
    expect(within(row).getByText('123456789012345678')).toBeInTheDocument()

    await userEvent.click(within(row).getByRole('button', { name: 'Approve' }))

    expect(post).toHaveBeenCalledWith('/api/wt/entry-requests/3/approve')
    expect(await screen.findByText('No entry requests')).toBeInTheDocument()
    expect(screen.queryByText('Kaze')).toBeNull()
  })

  test('rejecting asks first and says the applicant can ask again after 24 hours', async () => {
    const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/wt/entry-requests/3/reject') {
        return decide(3, 'rejected')
      }
      throw new Error(`Unexpected POST ${url}`)
    })
    renderAllowlistPage()

    const row = await findRowOf('Kaze')
    await userEvent.click(within(row).getByRole('button', { name: 'Reject' }))

    const dialog = await screen.findByRole('alertdialog')
    expect(
      within(dialog).getByText('Kaze can apply again 24 hours from now.')
    ).toBeInTheDocument()
    expect(post).not.toHaveBeenCalled()

    await userEvent.click(
      within(dialog).getByRole('button', { name: 'Reject' })
    )

    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/api/wt/entry-requests/3/reject')
    )
    expect(await screen.findByText('No entry requests')).toBeInTheDocument()
  })
})
