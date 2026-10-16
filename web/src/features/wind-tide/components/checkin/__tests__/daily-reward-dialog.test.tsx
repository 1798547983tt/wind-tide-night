/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { artManifestQueryOptions } from '../../../api'
import { DailyRewardDialog } from '../daily-reward-dialog'

const REWARD = 4_999_500_000
const SEEN_DAY_KEY = 'wt.daily-reward.seen'

let today = '2026-10-15'
let checkedInToday = false

function serveCheckin() {
  return vi.spyOn(api, 'get').mockImplementation(async () => ({
    data: {
      success: true,
      data: {
        enabled: true,
        today,
        checked_in_today: checkedInToday,
        streak: 3,
        reward_quota: REWARD,
        stats: {
          checked_in_today: checkedInToday,
          total_checkins: 3,
          total_quota: 3 * REWARD,
          checkin_count: 3,
          records: [],
        },
      },
    },
  }))
}

function openConsole(status: { checkin_enabled: boolean }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  // A fresh status entry, so the dialog reads it without fetching.
  client.setQueryData(['status'], status)
  client.setQueryData(artManifestQueryOptions.queryKey, {})
  return render(
    <QueryClientProvider client={client}>
      <DailyRewardDialog />
    </QueryClientProvider>
  )
}

afterEach(() => {
  today = '2026-10-15'
  checkedInToday = false
  window.localStorage.clear()
  vi.restoreAllMocks()
})

describe('daily reward dialog', () => {
  test('the first console visit of a check-in day offers the day’s gift', async () => {
    serveCheckin()
    openConsole({ checkin_enabled: true })

    expect(
      await screen.findByRole('dialog', { name: "Today's check-in gift" })
    ).toBeVisible()
  })

  test('claiming stamps the check-in, shows the reward and then closes for the day', async () => {
    const user = userEvent.setup()
    serveCheckin()
    const post = vi.spyOn(api, 'post').mockImplementation(async () => {
      checkedInToday = true
      return {
        data: {
          success: true,
          data: { quota_awarded: REWARD, checkin_date: today, streak: 4 },
        },
      }
    })
    openConsole({ checkin_enabled: true })

    await user.click(
      await screen.findByRole('button', { name: 'Stamp and claim' })
    )

    expect(post).toHaveBeenCalledWith('/api/user/checkin')
    expect(
      await screen.findByRole('dialog', { name: 'Claimed!' })
    ).toHaveTextContent(/has been added to your balance/)

    await user.click(screen.getByRole('button', { name: 'Thanks!' }))

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(window.localStorage.getItem(SEEN_DAY_KEY)).toBe('2026-10-15')
  })

  test('put off with “later”, it stays away for the rest of that Beijing day', async () => {
    const user = userEvent.setup()
    serveCheckin()
    const first = openConsole({ checkin_enabled: true })

    await user.click(await screen.findByRole('button', { name: 'Later' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    first.unmount()

    const get = serveCheckin()
    openConsole({ checkin_enabled: true })
    await waitFor(() => expect(get).toHaveBeenCalled())
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  test('on the next Beijing day the gift is offered again', async () => {
    window.localStorage.setItem(SEEN_DAY_KEY, '2026-10-15')
    today = '2026-10-16'
    serveCheckin()
    openConsole({ checkin_enabled: true })

    expect(
      await screen.findByRole('dialog', { name: "Today's check-in gift" })
    ).toBeVisible()
  })

  test('a member who already checked in today is not asked again', async () => {
    checkedInToday = true
    const get = serveCheckin()
    openConsole({ checkin_enabled: true })

    await waitFor(() => expect(get).toHaveBeenCalled())
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  test('with check-in turned off the console neither asks nor fetches', () => {
    const get = serveCheckin()
    openConsole({ checkin_enabled: false })

    expect(get).not.toHaveBeenCalled()
    expect(screen.queryByRole('dialog')).toBeNull()
  })
})
