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
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { Route as RankingsRoute } from '@/routes/rankings/index'
import { useAuthStore } from '@/stores/auth-store'

import { artManifestQueryOptions } from '../../../api'
import type {
  Leaderboard,
  LeaderboardBoard,
  LeaderboardItem,
  LeaderboardPeriod,
} from '../../../types'
import { RankingsPage } from '../rankings-page'

// The model board's charts draw on a canvas, which jsdom lacks.
vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))

// First place on each board; the streak boards also tell the run right now.
const FIRST_PLACE: Record<
  LeaderboardBoard,
  Pick<LeaderboardItem, 'value' | 'current'>
> = {
  calls: { value: 40 },
  failed: { value: 3 },
  success_streak: { value: 12, current: 5 },
  fail_streak: { value: 2, current: 1 },
  balance: { value: 4_999_500_000 },
}

/** The server's answer: the board asked for, over the period asked for. */
function board(
  kind: LeaderboardBoard,
  period: Leaderboard['period']
): Leaderboard {
  return {
    board: kind,
    period,
    items: [
      {
        rank: 1,
        user_id: 100,
        display_name: 'Member 1',
        avatar_url: '',
        ...FIRST_PLACE[kind],
      },
    ],
    me: { rank: 2, value: 0 },
    updated_at: 1_760_000_000,
  }
}

/** The rankings page at its app route id, with the app's search schema. */
function openRankings(entry: string) {
  useAuthStore
    .getState()
    .auth.setUser({ id: 7, username: 'tide', role: ROLE.USER })
  const get = vi.spyOn(api, 'get').mockImplementation(async (url, config) => {
    if (url === '/api/wt/leaderboard') {
      const params = config?.params as {
        board: LeaderboardBoard
        period?: LeaderboardPeriod
      }
      return {
        data: {
          success: true,
          data: board(params.board, params.period ?? ''),
        },
      }
    }
    if (url === '/api/rankings') throw new Error('Network Error')
    if (url === '/api/status') return { data: { success: true, data: {} } }
    // The header's notice and anything else it asks for is empty.
    return { data: { success: true, data: '' } }
  })
  const root = createRootRoute()
  const rankings = createRoute({
    getParentRoute: () => root,
    path: 'rankings',
  })
  const index = createRoute({
    getParentRoute: () => rankings,
    path: '/',
    validateSearch: RankingsRoute.options.validateSearch,
    component: RankingsPage,
  })
  const router = createRouter({
    routeTree: root.addChildren([rankings.addChildren([index])]),
    history: createMemoryHistory({ initialEntries: [entry] }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(artManifestQueryOptions.queryKey, {})
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return { router, get }
}

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

describe('rankings page', () => {
  test('opening the rankings shows the call board for the last 7 days', async () => {
    const { get } = openRankings('/rankings')

    const main = await screen.findByRole('main')
    expect(
      within(main).getByRole('heading', { level: 1, name: 'Rankings' })
    ).toBeVisible()
    expect(screen.getByRole('tab', { name: 'Call board' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    const periods = screen.getByRole('group', { name: 'Period' })
    expect(
      within(periods).getByRole('button', { name: 'Last 7 days' })
    ).toHaveAttribute('aria-pressed', 'true')
    expect(await screen.findByText('40 calls')).toBeVisible()
    expect(get).toHaveBeenCalledWith('/api/wt/leaderboard', {
      params: { board: 'calls', period: 'week' },
    })
  })

  test('the six boards are offered as tabs, the members’ five before the model board', async () => {
    openRankings('/rankings')

    const tabs = within(await screen.findByRole('tablist')).getAllByRole('tab')
    expect(tabs.map((tab) => tab.textContent)).toEqual([
      'Call board',
      'Failed board',
      'Success streak board',
      'Fail streak board',
      'Balance board',
      'Model board',
    ])
  })

  test('six tabs outgrow a phone, so the strip scrolls sideways instead of wrapping or widening the page', async () => {
    openRankings('/rankings')

    const tablist = await screen.findByRole('tablist')
    expect(tablist.parentElement).toHaveClass('overflow-x-auto', 'min-w-0')
    expect(tablist).toHaveClass('flex-nowrap')
    for (const tab of within(tablist).getAllByRole('tab')) {
      expect(tab).toHaveClass('flex-none', 'whitespace-nowrap')
    }
  })

  test('the title starts below the fixed 4rem header on phones and wider screens', async () => {
    openRankings('/rankings')

    const title = await screen.findByRole('heading', { level: 1 })
    // The header is h-16; pt-20 and sm:pt-24 leave a clear gap under it.
    expect(title.closest('header')?.parentElement).toHaveClass(
      'pt-20',
      'sm:pt-24'
    )
  })

  test('choosing the balance board asks for it, keeps it in the address and drops the period', async () => {
    const user = userEvent.setup()
    const { router, get } = openRankings('/rankings')
    await screen.findByText('40 calls')

    await user.click(screen.getByRole('tab', { name: 'Balance board' }))

    expect(await screen.findByText('$9,999')).toBeVisible()
    expect(router.state.location.search).toMatchObject({ board: 'balance' })
    expect(get).toHaveBeenCalledWith('/api/wt/leaderboard', {
      params: { board: 'balance' },
    })
    // The balance board stands for the moment, so it has no period.
    expect(screen.queryByRole('group', { name: 'Period' })).toBeNull()
  })

  test('choosing the failed board asks for failed calls over the same period and counts them as failed', async () => {
    const user = userEvent.setup()
    const { router, get } = openRankings('/rankings')
    await screen.findByText('40 calls')

    await user.click(screen.getByRole('tab', { name: 'Failed board' }))

    expect(await screen.findByText('3 failed')).toBeVisible()
    expect(router.state.location.search).toMatchObject({ board: 'failed' })
    expect(get).toHaveBeenCalledWith('/api/wt/leaderboard', {
      params: { board: 'failed', period: 'week' },
    })
    const periods = screen.getByRole('group', { name: 'Period' })
    expect(
      within(periods).getByRole('button', { name: 'Last 7 days' })
    ).toHaveAttribute('aria-pressed', 'true')
  })

  test.each<[LeaderboardBoard, string, string, string]>([
    ['success_streak', 'Success streak board', '12 in a row', 'Now 5 in a row'],
    ['fail_streak', 'Fail streak board', '2 in a row', 'Now 1 in a row'],
  ])(
    'the %s board is asked for without a period, offers none, and shows the run right now under the longest',
    async (kind, name, longest, now) => {
      const { get } = openRankings(`/rankings?board=${kind}`)

      expect(await screen.findByRole('tab', { name })).toHaveAttribute(
        'aria-selected',
        'true'
      )
      expect(await screen.findByText(longest)).toBeVisible()
      expect(screen.getByText(now)).toBeVisible()
      expect(screen.queryByRole('group', { name: 'Period' })).toBeNull()
      expect(get).toHaveBeenCalledWith('/api/wt/leaderboard', {
        params: { board: kind },
      })
    }
  )

  test('choosing another period asks for that period and keeps it in the address', async () => {
    const user = userEvent.setup()
    const { router, get } = openRankings('/rankings')
    await screen.findByText('40 calls')

    await user.click(screen.getByRole('button', { name: 'Today' }))

    expect(router.state.location.search).toMatchObject({ range: 'today' })
    expect(screen.getByRole('button', { name: 'Today' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
    expect(get).toHaveBeenCalledWith('/api/wt/leaderboard', {
      params: { board: 'calls', period: 'today' },
    })
  })

  test('a shared link opens the board it names', async () => {
    openRankings('/rankings?board=balance')

    expect(
      await screen.findByRole('tab', { name: 'Balance board' })
    ).toHaveAttribute('aria-selected', 'true')
    expect(await screen.findByText('$9,999')).toBeVisible()
  })

  test('a link naming an unknown board or period falls back to the call board for the last 7 days', async () => {
    openRankings('/rankings?board=bogus&range=year')

    expect(
      await screen.findByRole('tab', { name: 'Call board' })
    ).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('button', { name: 'Last 7 days' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
  })

  test('the model board shows the model rankings with their own periods, not a members’ board', async () => {
    const { get } = openRankings('/rankings?board=models')

    expect(await screen.findByRole('button', { name: 'Week' })).toHaveAttribute(
      'aria-pressed',
      'true'
    )
    expect(screen.getByRole('button', { name: 'Year' })).toBeVisible()
    expect(get).toHaveBeenCalledWith('/api/rankings', {
      params: { period: 'week' },
    })
    expect(get).not.toHaveBeenCalledWith(
      '/api/wt/leaderboard',
      expect.anything()
    )
  })
})
