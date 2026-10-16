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
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { artManifestQueryOptions } from '../../../api'
import type { ArtEntry, ArtManifest } from '../../../lib/art'
import type { CheckinMonth } from '../../../types'
import { DeskMascot } from '../desk-mascot'

const picture: ArtEntry = {
  width: 760,
  height: 788,
  sizes: [160, 320],
  depth: false,
}

function month(checkedInToday: boolean): CheckinMonth {
  return {
    enabled: true,
    today: '2026-10-15',
    checked_in_today: checkedInToday,
    streak: 0,
    reward_quota: 4_999_500_000,
    stats: {
      checked_in_today: checkedInToday,
      total_checkins: 0,
      total_quota: 0,
      checkin_count: 0,
      records: [],
    },
  }
}

/**
 * The mascot on a console page (the overview unless `path` says otherwise) at
 * the given Beijing time, for a member checked in or not.
 */
function renderMascot(options: {
  beijing: string
  checkedInToday?: boolean
  manifest?: ArtManifest
  path?: string
}) {
  vi.useFakeTimers({
    toFake: ['Date'],
    now: new Date(`${options.beijing}+08:00`),
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/user/checkin') {
      return {
        data: { success: true, data: month(options.checkedInToday ?? true) },
      }
    }
    return { data: { success: true, data: { checkin_enabled: true } } }
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(
    artManifestQueryOptions.queryKey,
    options.manifest ?? { 'mascot-base': picture }
  )
  client.setQueryData(['status'], { checkin_enabled: true })
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <>
          <p>Console page</p>
          <DeskMascot />
        </>
      ),
    }),
    history: createMemoryHistory({
      initialEntries: [options.path ?? '/dashboard/overview'],
    }),
  })
  return render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.restoreAllMocks()
  localStorage.clear()
})

describe('desk mascot', () => {
  test.each([
    ['03:00', "Still up? I'll keep you company tonight."],
    ['09:00', 'Good morning~ The wind is gentle today.'],
    ['12:00', "It's noon~ Don't forget lunch."],
    ['14:00', 'Good afternoon~ Rest your eyes between calls.'],
    ['20:00', 'Good evening~ The tide has come in.'],
  ])(
    'a click at %s Beijing time brings a greeting for the hour',
    async (time, greeting) => {
      renderMascot({ beijing: `2026-10-15T${time}:00` })

      await userEvent.click(
        await screen.findByRole('button', { name: 'Talk to Cecilia' })
      )

      expect(screen.getByText(greeting)).toBeVisible()
    }
  )

  test('a member who has not checked in hears about the day’s reward next', async () => {
    const user = userEvent.setup()
    renderMascot({ beijing: '2026-10-15T09:00:00', checkedInToday: false })
    const mascot = await screen.findByRole('button', {
      name: 'Talk to Cecilia',
    })

    await user.click(mascot)
    await user.click(mascot)

    expect(
      await screen.findByText("Today's $9,999 is still waiting for your stamp~")
    ).toBeVisible()
    expect(screen.queryByText(/Good morning/)).toBeNull()
  })

  test('a member who has checked in is not reminded and hears the next line instead', async () => {
    const user = userEvent.setup()
    renderMascot({ beijing: '2026-10-15T09:00:00', checkedInToday: true })
    const mascot = await screen.findByRole('button', {
      name: 'Talk to Cecilia',
    })

    await user.click(mascot)
    await user.click(mascot)

    expect(
      screen.getByText(
        'The wind carries the question away, the tide brings the answer back.'
      )
    ).toBeVisible()
    expect(screen.queryByText(/still waiting for your stamp/)).toBeNull()
  })

  test('her line is announced as it appears and goes away after a few seconds', async () => {
    renderMascot({ beijing: '2026-10-15T09:00:00' })
    const mascot = await screen.findByRole('button', {
      name: 'Talk to Cecilia',
    })
    // The bubble's own timer, under the test's control from here on.
    vi.useFakeTimers({
      toFake: ['Date', 'setTimeout', 'clearTimeout'],
      now: new Date('2026-10-15T09:00:00+08:00'),
    })

    fireEvent.click(mascot)
    const line = screen.getByText('Good morning~ The wind is gentle today.')
    expect(line.closest('[aria-live="polite"]')).not.toBeNull()

    act(() => {
      vi.advanceTimersByTime(6000)
    })

    expect(screen.queryByText(/Good morning/)).toBeNull()
  })

  test('she peeks out of the corner, stands up to say her line and settles back after it', async () => {
    renderMascot({ beijing: '2026-10-15T09:00:00' })
    const mascot = await screen.findByRole('button', {
      name: 'Talk to Cecilia',
    })
    const corner = mascot.closest('[data-pose]')
    expect(corner).toHaveAttribute('data-pose', 'peek')
    vi.useFakeTimers({
      toFake: ['Date', 'setTimeout', 'clearTimeout'],
      now: new Date('2026-10-15T09:00:00+08:00'),
    })

    fireEvent.click(mascot)

    expect(corner).toHaveAttribute('data-pose', 'stand')

    act(() => {
      vi.advanceTimersByTime(6000)
    })

    expect(corner).toHaveAttribute('data-pose', 'peek')
  })

  test('letting her rest leaves a portrait that calls her back, and the choice is remembered', async () => {
    const user = userEvent.setup()
    renderMascot({ beijing: '2026-10-15T09:00:00' })

    await user.click(
      await screen.findByRole('button', { name: 'Let Cecilia rest' })
    )

    expect(screen.queryByRole('button', { name: 'Talk to Cecilia' })).toBeNull()
    expect(localStorage.getItem('wt.mascot.resting')).toBe('1')

    await user.click(screen.getByRole('button', { name: 'Call Cecilia back' }))

    expect(
      screen.getByRole('button', { name: 'Talk to Cecilia' })
    ).toBeVisible()
    expect(localStorage.getItem('wt.mascot.resting')).toBeNull()
  })

  test('a member who let her rest finds her resting on the next visit', async () => {
    localStorage.setItem('wt.mascot.resting', '1')

    renderMascot({ beijing: '2026-10-15T09:00:00' })

    expect(
      await screen.findByRole('button', { name: 'Call Cecilia back' })
    ).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Talk to Cecilia' })).toBeNull()
  })

  test('she keeps to the bottom-right corner of desktops, out of the way on phones', async () => {
    renderMascot({ beijing: '2026-10-15T09:00:00' })

    const corner = (
      await screen.findByRole('button', { name: 'Talk to Cecilia' })
    ).closest('.fixed')
    expect(corner).toHaveClass('hidden', 'lg:block', 'right-4', 'bottom-0')
  })

  test.each(['/playground', '/chat/0'])(
    'on %s, where the corner holds the send button, she steps aside',
    async (path) => {
      renderMascot({ beijing: '2026-10-15T09:00:00', path })

      await screen.findByText('Console page')

      expect(
        screen.queryByRole('button', { name: 'Talk to Cecilia' })
      ).toBeNull()
      expect(
        screen.queryByRole('button', { name: 'Call Cecilia back' })
      ).toBeNull()
    }
  )

  test('without any mascot art uploaded she does not appear at all', async () => {
    renderMascot({
      beijing: '2026-10-15T09:00:00',
      manifest: {},
    })

    await screen.findByText('Console page')

    expect(screen.queryByRole('button', { name: /Cecilia/ })).toBeNull()
  })
})
