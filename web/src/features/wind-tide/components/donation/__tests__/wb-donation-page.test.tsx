/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type { WbDonation, WbDonationPool } from '../../../types'
import { WbDonationPage } from '../wb-donation-page'

let items: WbDonation[]
let listCalls: number

function pool(overrides: Partial<WbDonationPool> = {}): WbDonationPool {
  return {
    state: 'available',
    message: 'Available',
    nickname: 'Alpha',
    realm: 'cn',
    credits: 300,
    credits_total: 500,
    retry_at: 0,
    success: 40,
    failed: 2,
    checked_in: true,
    ...overrides,
  }
}

function donation(
  id: number,
  nickname: string,
  overrides: Partial<WbDonation> = {}
): WbDonation {
  return {
    id,
    provider: 'workbuddy',
    email: '',
    nickname,
    status: 'active',
    fail_reason: '',
    pool: pool({ nickname }),
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
  render(
    <QueryClientProvider client={client}>
      <WbDonationPage />
    </QueryClientProvider>
  )
}

function cardOf(text: string) {
  const card = screen.getByText(text).closest('[data-slot="card"]')
  if (!card) throw new Error(`No card for ${text}`)
  return card as HTMLElement
}

beforeEach(() => {
  items = []
  listCalls = 0
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/wt/donations') {
      listCalls += 1
      return { data: { success: true, data: { max: 10, items } } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/wt/donations/1/test') {
      return {
        data: {
          success: true,
          data: {
            ok: true,
            latency_ms: 800,
            message: 'The pool can still use this account',
            usage: null,
          },
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

describe('the WorkBuddy page', () => {
  test('with nothing donated it shows the slogan, the empty state and an enabled button', async () => {
    renderPage()
    expect(
      await screen.findByText('No WorkBuddy accounts yet')
    ).toBeInTheDocument()
    expect(
      screen.getByRole('heading', { name: 'WorkBuddy' })
    ).toBeInTheDocument()
    expect(
      screen.getByText(/every member can call the WB- models/)
    ).toBeInTheDocument()
    expect(screen.getByText('Donated 0 / 10')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Donate a WorkBuddy account' })
    ).toBeEnabled()
    expect(api.get).toHaveBeenCalledWith('/api/wt/donations', {
      params: { provider: 'workbuddy' },
    })
  })

  test('accounts go by their nickname, with their credits, sign-in and the pool state', async () => {
    items = [
      donation(1, 'Alpha', {
        last_test: {
          at: 1760000500,
          ok: true,
          latency_ms: 800,
          message: 'The pool can still use this account',
        },
      }),
      donation(2, 'Beta', {
        pool: pool({
          nickname: 'Beta',
          realm: 'global',
          state: 'cooldown',
          message: 'Credits used up for today',
          credits: 0,
          retry_at: 1760040000,
          checked_in: false,
        }),
      }),
      donation(3, 'Gamma', {
        pool: pool({
          nickname: 'Gamma',
          state: 'paused',
          message: 'The pool owner paused this account',
        }),
      }),
      donation(4, 'Delta', {
        pool: pool({
          nickname: 'Delta',
          state: 'disabled',
          message:
            'The pool can no longer sign in with this account; remove it and donate again',
        }),
      }),
      donation(5, 'Epsilon', {
        pool: pool({
          nickname: 'Epsilon',
          state: 'missing',
          message: 'The pool no longer has this account',
          credits: 0,
          credits_total: 0,
        }),
      }),
      donation(6, '', { status: 'verifying', pool: null, verified_at: 0 }),
      donation(7, '', {
        status: 'failed',
        pool: null,
        fail_reason: 'The sign-in was not finished in time; start again',
        verified_at: 0,
      }),
    ]
    renderPage()
    await screen.findByText('Alpha')

    const fine = cardOf('Alpha')
    expect(within(fine).getByText('Available')).toBeInTheDocument()
    expect(within(fine).getByText('300 / 500 credits left')).toBeInTheDocument()
    expect(
      within(fine).getByText('CodeBuddy (China) · Checked in today')
    ).toBeInTheDocument()
    expect(
      within(fine).getByText(/40 succeeded · 2 failed/)
    ).toBeInTheDocument()
    expect(within(fine).getByText(/passed in 800 ms/)).toBeInTheDocument()
    expect(within(fine).queryByText(/Back at /)).not.toBeInTheDocument()

    const resting = cardOf('Beta')
    expect(within(resting).getByText('Cooling down')).toBeInTheDocument()
    expect(
      within(resting).getByText('0 / 500 credits left')
    ).toBeInTheDocument()
    expect(
      within(resting).getByText('WorkBuddy (international)')
    ).toBeInTheDocument()
    expect(within(resting).queryByText(/Checked in/)).not.toBeInTheDocument()
    expect(within(resting).getByText(/Back at /)).toBeInTheDocument()
    expect(
      within(resting).getByText('Credits used up for today')
    ).toBeInTheDocument()

    expect(within(cardOf('Gamma')).getByText('Paused')).toBeInTheDocument()
    expect(
      within(cardOf('Delta')).getByText('Sign-in expired')
    ).toBeInTheDocument()
    const gone = cardOf('Epsilon')
    expect(within(gone).getByText('Not in the pool')).toBeInTheDocument()
    // Nothing is left to count for an account the pool no longer has.
    expect(within(gone).queryByText(/credits left/)).not.toBeInTheDocument()
    expect(
      within(gone).getByText('The pool no longer has this account')
    ).toBeInTheDocument()

    // Rows the pool has not named yet are called by the pool.
    expect(screen.getAllByText('WorkBuddy account')).toHaveLength(2)
    expect(screen.getByText('Verifying')).toBeInTheDocument()
    expect(screen.getByText('Failed')).toBeInTheDocument()
    expect(
      screen.getByText('The sign-in was not finished in time; start again')
    ).toBeInTheDocument()
    expect(screen.queryByText(/@/)).not.toBeInTheDocument()

    // Five active cards can be tested; the failed one retries; no card is
    // ever re-authorized, as the pool keeps its own sign-in.
    expect(screen.getAllByRole('button', { name: 'Test' })).toHaveLength(5)
    expect(screen.getAllByRole('button', { name: 'Retry' })).toHaveLength(1)
    expect(
      screen.queryByRole('button', { name: 'Authorize again' })
    ).not.toBeInTheDocument()
    // The failed row holds no slot.
    expect(screen.getByText('Donated 6 / 10')).toBeInTheDocument()
    const urls = vi.mocked(api.get).mock.calls.map(([url]) => String(url))
    expect(urls.some((url) => url.includes('/usage'))).toBe(false)
  })

  test('Test calls the server and refreshes the list', async () => {
    items = [donation(1, 'Alpha')]
    renderPage()
    await screen.findByText('Alpha')
    const before = listCalls
    await userEvent.click(
      within(cardOf('Alpha')).getByRole('button', { name: 'Test' })
    )
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/wt/donations/1/test')
    )
    await waitFor(() => expect(listCalls).toBeGreaterThan(before))
  })

  test('Remove explains that the sign-in token goes too, then calls the server', async () => {
    items = [donation(1, 'Alpha')]
    renderPage()
    await screen.findByText('Alpha')
    await userEvent.click(
      within(cardOf('Alpha')).getByRole('button', { name: 'Remove' })
    )
    expect(api.post).not.toHaveBeenCalled()
    const confirm = await screen.findByRole('alertdialog')
    expect(
      within(confirm).getByText('Remove this donated account?')
    ).toBeInTheDocument()
    expect(
      within(confirm).getByText(
        'The account leaves the pool now and its sign-in token is deleted. You can donate it again later.'
      )
    ).toBeInTheDocument()
    await userEvent.click(
      within(confirm).getByRole('button', { name: 'Remove' })
    )
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/wt/donations/1/remove')
    )
  })

  test('Retry on a failed row opens the donate dialog from the start', async () => {
    items = [
      donation(7, '', {
        status: 'failed',
        pool: null,
        fail_reason: 'The sign-in was not finished in time; start again',
        verified_at: 0,
      }),
    ]
    renderPage()
    await userEvent.click(await screen.findByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Step 1 of 3: Consent')).toBeInTheDocument()
    expect(api.post).not.toHaveBeenCalled()
  })

  test('at ten slots the donate button is disabled and says why', async () => {
    items = Array.from({ length: 10 }, (_, index) =>
      donation(index + 1, `Buddy ${index + 1}`)
    )
    renderPage()
    expect(await screen.findByText('Donated 10 / 10')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Donate a WorkBuddy account' })
    ).toBeDisabled()
    expect(
      screen.getByText('You have reached the limit of 10 donated accounts')
    ).toBeInTheDocument()
  })
})
