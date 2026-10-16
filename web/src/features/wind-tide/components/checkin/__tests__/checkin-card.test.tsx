/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { artManifestQueryOptions } from '../../../api'
import type { CheckinMonth } from '../../../types'
import { CheckinCard } from '../checkin-card'

const REWARD = 4_999_500_000
const mascot = { width: 760, height: 788, sizes: [160, 320], depth: false }

type MonthFixture = {
  today: string
  checkedInToday?: boolean
  /** Days stamped in the month asked for. */
  days?: string[]
}

function month(fixture: MonthFixture): CheckinMonth {
  const days = fixture.days ?? []
  return {
    enabled: true,
    today: fixture.today,
    checked_in_today: fixture.checkedInToday ?? false,
    streak: days.length,
    reward_quota: REWARD,
    stats: {
      checked_in_today: fixture.checkedInToday ?? false,
      total_checkins: days.length,
      total_quota: days.length * REWARD,
      checkin_count: days.length,
      records: days.map((day) => ({
        checkin_date: day,
        quota_awarded: REWARD,
      })),
    },
  }
}

/** Answers GET /api/user/checkin per month: `current` without a month. */
function serveMonths(months: Record<string, CheckinMonth>) {
  return vi.spyOn(api, 'get').mockImplementation(async (_url, config) => {
    const asked = (config?.params as { month?: string } | undefined)?.month
    return { data: { success: true, data: months[asked ?? 'current'] } }
  })
}

function renderCard(options: { mascotArt?: boolean } = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  client.setQueryData(
    artManifestQueryOptions.queryKey,
    options.mascotArt ? { 'mascot-base': mascot } : {}
  )
  render(
    <QueryClientProvider client={client}>
      <CheckinCard />
    </QueryClientProvider>
  )
}

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('check-in card', () => {
  test('the calendar seals each day already checked in and marks today', async () => {
    serveMonths({
      current: month({
        today: '2026-10-15',
        days: ['2026-10-03', '2026-10-04'],
      }),
    })
    renderCard()

    const calendar = await screen.findByRole('list')
    expect(within(calendar).getAllByText('Checked in')).toHaveLength(2)
    expect(within(calendar).getByText('3').closest('li')).toHaveTextContent(
      'Checked in'
    )
    expect(within(calendar).getByText('15').closest('li')).toHaveTextContent(
      'Today'
    )
  })

  test('stamping checks in, leaves the print and says the reward is in the balance', async () => {
    const user = userEvent.setup()
    const months = { current: month({ today: '2026-10-15' }) }
    serveMonths(months)
    const post = vi.spyOn(api, 'post').mockImplementation(async () => {
      months.current = month({
        today: '2026-10-15',
        checkedInToday: true,
        days: ['2026-10-15'],
      })
      return {
        data: {
          success: true,
          data: {
            quota_awarded: REWARD,
            checkin_date: '2026-10-15',
            streak: 1,
          },
        },
      }
    })
    renderCard()

    await user.click(
      await screen.findByRole('button', { name: /Stamp to check in/ })
    )

    expect(post).toHaveBeenCalledWith('/api/user/checkin')
    expect(await screen.findByRole('status')).toHaveTextContent(
      /has been added to your balance/
    )
    expect(screen.getByText('Stamped for today')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: /Stamp to check in/ })
    ).toBeNull()
  })

  test('a member who checked in earlier today sees the print instead of the seal', async () => {
    serveMonths({
      current: month({
        today: '2026-10-15',
        checkedInToday: true,
        days: ['2026-10-15'],
      }),
    })
    renderCard()

    expect(await screen.findByText('Stamped for today')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: /Stamp to check in/ })
    ).toBeNull()
  })

  test('the calendar follows the Beijing day the server names, not the browser clock', async () => {
    // Still 30 September by the browser's UTC clock.
    vi.useFakeTimers({ toFake: ['Date'] })
    vi.setSystemTime(new Date('2026-09-30T17:00:00Z'))
    serveMonths({ current: month({ today: '2026-10-01' }) })
    renderCard()

    expect(
      await screen.findByRole('heading', { name: 'October 2026' })
    ).toBeVisible()
    expect(
      within(screen.getByRole('list')).getByText('1').closest('li')
    ).toHaveTextContent('Today')
  })

  test('leafing back a month shows that month with its own stamps', async () => {
    const user = userEvent.setup()
    serveMonths({
      current: month({ today: '2026-10-15' }),
      '2026-09': month({ today: '2026-10-15', days: ['2026-09-10'] }),
    })
    renderCard()

    const next = await screen.findByRole('button', { name: 'Next month' })
    expect(next).toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'Previous month' }))

    expect(
      await screen.findByRole('heading', { name: 'September 2026' })
    ).toBeVisible()
    expect(
      await within(screen.getByRole('list')).findByText('Checked in')
    ).toBeInTheDocument()
    expect(next).toBeEnabled()
  })

  test('on a deployment with the mascot she cheers the member on beside the seal', async () => {
    serveMonths({ current: month({ today: '2026-10-15' }) })
    renderCard({ mascotArt: true })

    expect(
      await screen.findByText("Today's seal is waiting for you~")
    ).toBeVisible()
  })

  test('dates and the streak keep lining figures, which the global tabular-nums rule would turn old-style', async () => {
    serveMonths({
      current: month({
        today: '2026-10-15',
        days: ['2026-10-13', '2026-10-14'],
      }),
    })
    renderCard()

    const calendar = await screen.findByRole('list')
    const figures = [
      within(calendar).getByText('15'),
      screen.getByText('2', { selector: 'dd' }),
    ]
    for (const figure of figures) {
      expect(figure).toHaveClass(
        '[font-variant-numeric:lining-nums_tabular-nums]'
      )
      expect(figure).not.toHaveClass('tabular-nums')
    }
  })

  test('on desktops the card leaves the mascot to the console’s corner', async () => {
    serveMonths({ current: month({ today: '2026-10-15' }) })
    renderCard({ mascotArt: true })

    const line = await screen.findByText("Today's seal is waiting for you~")
    expect(line.closest('figure')).toHaveClass('lg:hidden')
  })

  test('the calendar moves beside the seal only from desktop width, where half the card still fits it', async () => {
    serveMonths({ current: month({ today: '2026-10-15' }) })
    renderCard()

    const calendar = await screen.findByRole('region', { name: 'October 2026' })
    expect(calendar.parentElement).toHaveClass(
      'lg:grid-cols-[minmax(0,0.95fr)_minmax(0,1.05fr)]'
    )
    expect(calendar).toHaveClass('lg:border-l')
    expect(calendar).not.toHaveClass('md:border-l')
  })

  test('on phones the streak heads the figures and the other three share one row', async () => {
    serveMonths({ current: month({ today: '2026-10-15' }) })
    renderCard()

    const streak = await screen.findByText('Check-in streak')
    expect(streak.closest('div')).toHaveClass('col-span-3')
    expect(streak.closest('dl')).toHaveClass('grid-cols-3')
  })

  test('without mascot art the card still offers the seal, with no empty speech bubble', async () => {
    serveMonths({ current: month({ today: '2026-10-15' }) })
    renderCard({ mascotArt: false })

    expect(
      await screen.findByRole('button', { name: /Stamp to check in/ })
    ).toBeEnabled()
    expect(screen.queryByText("Today's seal is waiting for you~")).toBeNull()
  })
})
