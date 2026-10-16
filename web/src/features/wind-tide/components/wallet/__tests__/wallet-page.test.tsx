/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { formatQuota } from '@/lib/format'

import { artManifestQueryOptions } from '../../../api'
import { WalletPage } from '../wallet-page'

const STARTING_BALANCE = 1_000_000
const REWARD = 4_999_500_000
const TODAY = '2026-10-15'

let balance = STARTING_BALANCE

function renderWallet(status: { checkin_enabled: boolean }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  // A fresh status entry, so the page reads it without fetching.
  client.setQueryData(['status'], status)
  // No artwork on this deployment: the check-in shows without the mascot.
  client.setQueryData(artManifestQueryOptions.queryKey, {})
  render(
    <QueryClientProvider client={client}>
      <WalletPage />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  balance = STARTING_BALANCE
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/user/self') {
      return {
        data: {
          success: true,
          data: {
            id: 1,
            username: 'member',
            quota: balance,
            used_quota: 0,
            request_count: 0,
          },
        },
      }
    }
    const checkedIn = balance !== STARTING_BALANCE
    return {
      data: {
        success: true,
        data: {
          enabled: true,
          today: TODAY,
          checked_in_today: checkedIn,
          streak: checkedIn ? 1 : 0,
          reward_quota: REWARD,
          stats: {
            checked_in_today: checkedIn,
            total_checkins: checkedIn ? 1 : 0,
            total_quota: checkedIn ? REWARD : 0,
            checkin_count: checkedIn ? 1 : 0,
            records: checkedIn
              ? [{ checkin_date: TODAY, quota_awarded: REWARD }]
              : [],
          },
        },
      },
    }
  })
  vi.spyOn(api, 'post').mockImplementation(async () => {
    balance += REWARD
    return {
      data: {
        success: true,
        data: { quota_awarded: REWARD, checkin_date: TODAY, streak: 1 },
      },
    }
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('wallet page', () => {
  test('a member sees the balance and the daily check-in with nothing to buy or redeem', async () => {
    renderWallet({ checkin_enabled: true })

    expect(
      await screen.findByText(formatQuota(STARTING_BALANCE))
    ).toBeInTheDocument()
    expect(
      await screen.findByRole('button', { name: /Stamp to check in/ })
    ).toBeEnabled()
    expect(screen.queryByRole('textbox')).toBeNull()
    expect(screen.queryByRole('spinbutton')).toBeNull()
  })

  test('checking in raises the balance shown on the wallet', async () => {
    const user = userEvent.setup()
    renderWallet({ checkin_enabled: true })

    await user.click(
      await screen.findByRole('button', { name: /Stamp to check in/ })
    )

    expect(
      await screen.findByText(formatQuota(STARTING_BALANCE + REWARD))
    ).toBeInTheDocument()
  })

  test('with the check-in turned off the wallet shows only the figures', async () => {
    renderWallet({ checkin_enabled: false })

    expect(
      await screen.findByText(formatQuota(STARTING_BALANCE))
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /Stamp to check in/ })
    ).toBeNull()
  })
})
