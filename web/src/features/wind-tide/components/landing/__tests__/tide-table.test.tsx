/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { publicStatsQueryOptions } from '../../../api'
import type { PublicStats } from '../../../types'
import { TideTable } from '../tide-table'

const stats: PublicStats = {
  members: 12,
  calls_today: 3456,
  // Two check-ins of $9,999 at 500,000 quota per dollar.
  rewarded_quota_total: 9_999_000_000,
}

function renderTable(client = new QueryClient()) {
  render(
    <QueryClientProvider client={client}>
      <TideTable />
    </QueryClientProvider>
  )
  return client
}

beforeEach(() => {
  // jsdom has no IntersectionObserver; the figures never scroll into view.
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('tide table', () => {
  test('visitors read the figures the server reports for the whole site', async () => {
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: stats },
    })

    renderTable()

    expect(await screen.findByText('3,456')).toBeInTheDocument()
    expect(screen.getByText('12')).toBeInTheDocument()
    expect(screen.getByText('$19,998')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Live figures for the whole site, refreshed every minute.'
      )
    ).toBeVisible()
  })

  test('a first load that fails says the tide is out and holds placeholders', async () => {
    vi.spyOn(api, 'get').mockRejectedValue(new Error('Network Error'))

    renderTable()

    expect(
      await screen.findByText('The tide has not come in yet.')
    ).toBeVisible()
    expect(screen.getAllByText('∿')).toHaveLength(3)
  })

  test('a refresh that fails keeps the last figures on screen', async () => {
    const get = vi
      .spyOn(api, 'get')
      .mockRejectedValue(new Error('Network Error'))
    const client = new QueryClient()
    // Figures from a minute and a half ago, due for a refresh.
    client.setQueryData(publicStatsQueryOptions.queryKey, stats, {
      updatedAt: Date.now() - 90_000,
    })

    renderTable(client)

    await waitFor(() => expect(get).toHaveBeenCalled())
    await waitFor(() =>
      expect(
        client.getQueryState(publicStatsQueryOptions.queryKey)?.status
      ).toBe('error')
    )
    expect(screen.getByText('3,456')).toBeInTheDocument()
    expect(screen.queryByText('The tide has not come in yet.')).toBeNull()
  })
})
