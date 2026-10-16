/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { cleanup, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { ROLE, type RoleValue } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { artManifestQueryOptions } from '../../../api'
import type {
  Leaderboard,
  LeaderboardBoard,
  LeaderboardPeriod,
} from '../../../types'
import { MemberBoard } from '../member-board'

const ME = 7

/** A board listing the given [rank, value]s, members named by position. */
function leaderboard(
  ranked: [number, number][],
  me: Leaderboard['me'],
  options: { board?: LeaderboardBoard; myIndex?: number } = {}
): Leaderboard {
  const board = options.board ?? 'calls'
  return {
    board,
    period: board === 'calls' ? 'week' : '',
    items: ranked.map(([rank, value], index) => ({
      rank,
      user_id: index === options.myIndex ? ME : 100 + index,
      display_name: `Member ${index + 1}`,
      avatar_url: '',
      value,
    })),
    me,
    updated_at: 1_760_000_000,
  }
}

function signIn(role: RoleValue = ROLE.USER) {
  useAuthStore.getState().auth.setUser({ id: ME, username: 'tide', role })
}

function newClient() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  // No mascot art uploaded; she shows as the fallback picture.
  client.setQueryData(artManifestQueryOptions.queryKey, {})
  return client
}

function renderBoard(board: LeaderboardBoard, range: LeaderboardPeriod) {
  return render(
    <QueryClientProvider client={newClient()}>
      <MemberBoard board={board} range={range} />
    </QueryClientProvider>
  )
}

afterEach(() => {
  // Unmount first: signing out under a mounted board would render the
  // visitor's sign-in link outside a router.
  cleanup()
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
})

describe('members’ leaderboard', () => {
  test('the first three stand on the podium in rank order and everyone else is listed below', async () => {
    signIn()
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: leaderboard(
          [
            [1, 40],
            [2, 25],
            [3, 12],
            [4, 9],
            [5, 3],
          ],
          { rank: 5, value: 3 },
          { myIndex: 4 }
        ),
      },
    })

    renderBoard('calls', 'week')

    const podium = await screen.findByRole('list', { name: 'Top three' })
    const standing = within(podium).getAllByRole('listitem')
    expect(standing.map((place) => place.textContent)).toEqual([
      expect.stringMatching(/^No\. 1.*Member 1.*40 calls/),
      expect.stringMatching(/^No\. 2.*Member 2.*25 calls/),
      expect.stringMatching(/^No\. 3.*Member 3.*12 calls/),
    ])
    const others = within(
      screen.getByRole('list', { name: 'Other members' })
    ).getAllByRole('listitem')
    expect(others).toHaveLength(2)
    expect(others[0]).toHaveTextContent(/No\. 4.*Member 4.*9 calls/)
    expect(within(others[1]).getByText('Me')).toBeVisible()
    expect(within(others[0]).queryByText('Me')).toBeNull()
  })

  test('a member below the podium sees their place and how far the place above is', async () => {
    signIn()
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: leaderboard(
          [
            [1, 40],
            [2, 25],
            [3, 12],
            [4, 9],
          ],
          { rank: 4, value: 9 },
          { myIndex: 3 }
        ),
      },
    })

    renderBoard('calls', 'week')

    const place = await screen.findByRole('complementary', { name: 'My place' })
    expect(place).toHaveTextContent('No. 4')
    expect(place).toHaveTextContent('3 calls behind the place above')
    expect(place).toHaveTextContent('9 calls')
  })

  test('the member in first place is told they are at the top', async () => {
    signIn()
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: leaderboard([[1, 40]], { rank: 1, value: 40 }, { myIndex: 0 }),
      },
    })

    renderBoard('calls', 'week')

    const place = await screen.findByRole('complementary', { name: 'My place' })
    expect(place).toHaveTextContent('No. 1')
    expect(place).toHaveTextContent("You're at the top!")
  })

  test('a member without calls in the period is not on the board yet and learns how to get on it', async () => {
    signIn()
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: leaderboard([[1, 40]], { rank: 2, value: 0 }),
      },
    })

    renderBoard('calls', 'today')

    const place = await screen.findByRole('complementary', { name: 'My place' })
    expect(place).toHaveTextContent('Not on the board yet')
    expect(place).toHaveTextContent('One call puts you on the board~')
    expect(place).not.toHaveTextContent('No. 2')
  })

  test('the owner, who is not ranked, is told so instead of a place', async () => {
    signIn(ROLE.SUPER_ADMIN)
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: leaderboard([[1, 40]], null) },
    })

    renderBoard('calls', 'week')

    const place = await screen.findByRole('complementary', { name: 'My place' })
    expect(place).toHaveTextContent('The owner is not ranked')
    expect(place).not.toHaveTextContent(/calls/)
  })

  test('the balance board reads each value as money', async () => {
    signIn()
    // $9,999 and $4,999.50 at 500,000 quota per dollar; whole dollars shown.
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: leaderboard(
          [
            [1, 4_999_500_000],
            [2, 2_499_750_000],
          ],
          { rank: 2, value: 2_499_750_000 },
          { board: 'balance', myIndex: 1 }
        ),
      },
    })

    renderBoard('balance', 'week')

    const podium = await screen.findByRole('list', { name: 'Top three' })
    expect(within(podium).getByText('$9,999')).toBeVisible()
    expect(
      screen.getByRole('complementary', { name: 'My place' })
    ).toHaveTextContent('$5,000 behind the place above')
    expect(screen.queryByText(/Counted by Beijing time/)).toBeNull()
  })

  test('the calls board says its days follow Beijing time, except for the all-time board', async () => {
    signIn()
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: leaderboard([[1, 40]], { rank: 2, value: 0 }),
      },
    })

    const view = renderBoard('calls', 'today')
    expect(await screen.findByText(/Counted by Beijing time/)).toBeVisible()

    view.unmount()
    renderBoard('calls', 'all')
    await screen.findByRole('list', { name: 'Top three' })
    expect(screen.queryByText(/Counted by Beijing time/)).toBeNull()
  })

  test('a period nobody has called in shows the dozing mascot instead of an empty podium', async () => {
    signIn()
    vi.spyOn(api, 'get').mockResolvedValue({
      data: { success: true, data: leaderboard([], { rank: 1, value: 0 }) },
    })

    renderBoard('calls', 'today')

    expect(await screen.findByText('No calls yet')).toBeVisible()
    expect(screen.getByText('First place is waiting for you~')).toBeVisible()
    expect(screen.queryByRole('list', { name: 'Top three' })).toBeNull()
  })

  test('an empty balance board asks members to check in', async () => {
    signIn()
    vi.spyOn(api, 'get').mockResolvedValue({
      data: {
        success: true,
        data: leaderboard([], { rank: 1, value: 0 }, { board: 'balance' }),
      },
    })

    renderBoard('balance', 'week')

    expect(await screen.findByText('Nobody has a balance yet')).toBeVisible()
    expect(screen.getByText('Check in to get on the board~')).toBeVisible()
  })

  test('a board that fails to load can be retried', async () => {
    signIn()
    const get = vi
      .spyOn(api, 'get')
      .mockRejectedValueOnce(new Error('Network Error'))
      .mockResolvedValue({
        data: {
          success: true,
          data: leaderboard([[1, 40]], { rank: 1, value: 40 }, { myIndex: 0 }),
        },
      })

    renderBoard('calls', 'week')

    expect(await screen.findByText('Unable to load rankings')).toBeVisible()
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))

    expect(
      await screen.findByRole('list', { name: 'Top three' })
    ).toHaveTextContent('Member 1')
    expect(get).toHaveBeenCalledTimes(2)
  })

  test('another period keeps the current board on show, marked busy, until its figures arrive', async () => {
    signIn()
    let answerToday: (board: Leaderboard) => void = () => undefined
    vi.spyOn(api, 'get').mockImplementation(async (_url, config) => {
      const params = config?.params as { period?: LeaderboardPeriod }
      if (params.period === 'today') {
        const board = await new Promise<Leaderboard>((resolve) => {
          answerToday = resolve
        })
        return { data: { success: true, data: board } }
      }
      return {
        data: {
          success: true,
          data: leaderboard([[1, 40]], { rank: 2, value: 0 }),
        },
      }
    })
    const client = newClient()
    const view = render(
      <QueryClientProvider client={client}>
        <MemberBoard board='calls' range='week' />
      </QueryClientProvider>
    )
    await screen.findByText('40 calls')

    view.rerender(
      <QueryClientProvider client={client}>
        <MemberBoard board='calls' range='today' />
      </QueryClientProvider>
    )

    const shown = screen.getByText('40 calls').closest('[aria-busy]')
    expect(shown).toHaveAttribute('aria-busy', 'true')
    answerToday(leaderboard([[1, 6]], { rank: 2, value: 0 }))
    expect(await screen.findByText('6 calls')).toBeVisible()
    expect(screen.queryByText('40 calls')).toBeNull()
    expect(screen.getByText('6 calls').closest('[aria-busy]')).toHaveAttribute(
      'aria-busy',
      'false'
    )
  })

  test('another board does not stand in for the one asked for, as its values mean something else', async () => {
    signIn()
    let answerBalance: (board: Leaderboard) => void = () => undefined
    vi.spyOn(api, 'get').mockImplementation(async (_url, config) => {
      const params = config?.params as { board: LeaderboardBoard }
      if (params.board === 'balance') {
        const board = await new Promise<Leaderboard>((resolve) => {
          answerBalance = resolve
        })
        return { data: { success: true, data: board } }
      }
      return {
        data: {
          success: true,
          data: leaderboard([[1, 40]], { rank: 2, value: 0 }),
        },
      }
    })
    const client = newClient()
    const view = render(
      <QueryClientProvider client={client}>
        <MemberBoard board='calls' range='week' />
      </QueryClientProvider>
    )
    await screen.findByText('40 calls')

    view.rerender(
      <QueryClientProvider client={client}>
        <MemberBoard board='balance' range='week' />
      </QueryClientProvider>
    )

    expect(screen.queryByText('40 calls')).toBeNull()
    expect(screen.queryByRole('list', { name: 'Top three' })).toBeNull()
    answerBalance(
      leaderboard(
        [[1, 4_999_500_000]],
        { rank: 2, value: 0 },
        {
          board: 'balance',
        }
      )
    )
    expect(await screen.findByText('$9,999')).toBeVisible()
  })

  test('a visitor is asked to sign in and the board is not requested', async () => {
    const get = vi.spyOn(api, 'get')
    const router = createRouter({
      routeTree: createRootRoute({
        component: () => <MemberBoard board='calls' range='week' />,
      }),
      history: createMemoryHistory({ initialEntries: ['/rankings'] }),
    })

    render(
      <QueryClientProvider client={newClient()}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    )

    // A link drawn as a button, as the project's buttons render links.
    expect(
      await screen.findByRole('button', { name: 'Sign in' })
    ).toHaveAttribute('href', '/sign-in?redirect=%2Frankings')
    expect(screen.getByText('Please sign in to view Rankings.')).toBeVisible()
    expect(get).not.toHaveBeenCalled()
  })
})
