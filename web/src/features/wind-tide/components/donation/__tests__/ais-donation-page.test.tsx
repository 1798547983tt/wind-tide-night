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

import type {
  AisDonation,
  AisDonationPool,
  BuildDonation,
  BuildDonationPool,
  CookieProvider,
} from '../../../types'
import { AisDonationPage } from '../ais-donation-page'

let items: (AisDonation | BuildDonation)[]
let listCalls: number

function pool(overrides: Partial<AisDonationPool> = {}): AisDonationPool {
  return {
    state: 'available',
    message: 'Available',
    retry_at: 0,
    success: 40,
    failed: 2,
    requests_today: 12,
    cooling: [],
    ...overrides,
  }
}

function donation(
  id: number,
  email: string,
  overrides: Partial<AisDonation> = {}
): AisDonation {
  return {
    id,
    provider: 'aistudio',
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

function buildPool(
  overrides: Partial<BuildDonationPool> = {}
): BuildDonationPool {
  return {
    state: 'available',
    message: 'Available',
    index: 3,
    current: false,
    has_context: true,
    ...overrides,
  }
}

function buildDonation(
  id: number,
  overrides: Partial<BuildDonation> = {}
): BuildDonation {
  return {
    id,
    provider: 'build',
    email: '',
    status: 'active',
    fail_reason: '',
    pool: buildPool({ index: id }),
    last_test: null,
    created_at: 1760000000 + id,
    verified_at: 1760000100 + id,
    ...overrides,
  }
}

function renderPage(provider: CookieProvider = 'aistudio') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <AisDonationPage provider={provider} />
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
            latency_ms: 1200,
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

describe('the AIS playground page', () => {
  test('with nothing donated it shows the slogan, the empty state and an enabled button', async () => {
    renderPage()
    expect(
      await screen.findByText('No AI Studio accounts yet')
    ).toBeInTheDocument()
    expect(screen.getByText('AIS playground')).toBeInTheDocument()
    expect(
      screen.getByText(/every account you donate gives you 50 calls a day/)
    ).toBeInTheDocument()
    expect(screen.getByText('Donated 0 / 10')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Donate an AI Studio account' })
    ).toBeEnabled()
    expect(api.get).toHaveBeenCalledWith('/api/wt/donations', {
      params: { provider: 'aistudio' },
    })
  })

  test('every pool state has its badge; emails are masked and nothing asks for usage', async () => {
    items = [
      donation(1, 'spare@example.com', {
        last_test: {
          at: 1760000500,
          ok: true,
          latency_ms: 1200,
          message: 'The pool can still use this account',
        },
        // One model is resting; the account still serves the other.
        pool: pool({
          cooling: [{ model: 'gemini-3.8-flash', retry_at: 1760050000 }],
        }),
      }),
      donation(2, 'tired@example.com', {
        pool: pool({
          state: 'exhausted',
          message: "Today's upstream quota of this account is used up",
          retry_at: 1760040000,
          requests_today: 50,
          cooling: [
            { model: 'gemini-4-argon', retry_at: 1760040000 },
            { model: 'gemini-3.8-flash', retry_at: 1760050000 },
          ],
        }),
      }),
      donation(3, 'paused@example.com', {
        pool: pool({
          state: 'cooldown',
          message: 'The pool paused this account after a sign-in error',
          retry_at: 1760003600,
        }),
      }),
      donation(4, 'stale@example.com', {
        pool: pool({
          state: 'expired',
          message:
            'The pool can no longer sign in with this account; remove it and donate again',
        }),
      }),
      donation(5, 'gone@example.com', {
        pool: pool({
          state: 'missing',
          message: 'The pool no longer has this account',
        }),
      }),
      donation(6, '', { status: 'verifying', pool: null, verified_at: 0 }),
      donation(7, '', {
        status: 'failed',
        pool: null,
        fail_reason: 'The pool lost the account; donate again',
        verified_at: 0,
      }),
    ]
    renderPage()
    await screen.findByText('sp***@example.com')

    const fine = cardOf('sp***@example.com')
    expect(within(fine).getByText('Available')).toBeInTheDocument()
    expect(
      within(fine).getByText('Used upstream today: 12')
    ).toBeInTheDocument()
    expect(
      within(fine).getByText(/40 succeeded · 2 failed/)
    ).toBeInTheDocument()
    expect(within(fine).getByText(/passed in 1200 ms/)).toBeInTheDocument()
    expect(
      within(fine).getByText(/^gemini-3.8-flash quota used up until /)
    ).toBeInTheDocument()
    expect(
      within(fine).queryByText(/^gemini-4-argon quota used up until /)
    ).not.toBeInTheDocument()
    expect(screen.queryByText('spare@example.com')).not.toBeInTheDocument()

    const tired = cardOf('ti***@example.com')
    expect(within(tired).getByText('Quota used up today')).toBeInTheDocument()
    expect(
      within(tired).getByText('Used upstream today: 50')
    ).toBeInTheDocument()
    expect(within(tired).getByText(/Back at /)).toBeInTheDocument()
    expect(
      within(tired).getByText(/^gemini-4-argon quota used up until /)
    ).toBeInTheDocument()
    expect(
      within(tired).getByText(/^gemini-3.8-flash quota used up until /)
    ).toBeInTheDocument()
    expect(
      within(tired).getByText(
        "Today's upstream quota of this account is used up"
      )
    ).toBeInTheDocument()
    expect(
      within(cardOf('pa***@example.com')).getByText('Paused')
    ).toBeInTheDocument()
    expect(
      within(cardOf('st***@example.com')).getByText('Sign-in expired')
    ).toBeInTheDocument()
    expect(
      within(cardOf('go***@example.com')).getByText('Not in the pool')
    ).toBeInTheDocument()

    // Rows whose email the pool has not read yet are named generically.
    const generic = screen.getAllByText('Google account')
    expect(generic).toHaveLength(2)
    expect(screen.getByText('Verifying')).toBeInTheDocument()
    expect(screen.getByText('Failed')).toBeInTheDocument()
    expect(
      screen.getByText('The pool lost the account; donate again')
    ).toBeInTheDocument()

    // Five active cards can be tested; the failed one retries; no card is
    // ever re-authorized, as a Cookie cannot be refreshed.
    expect(screen.getAllByRole('button', { name: 'Test' })).toHaveLength(5)
    expect(screen.getAllByRole('button', { name: 'Retry' })).toHaveLength(1)
    expect(
      screen.queryByRole('button', { name: 'Authorize again' })
    ).not.toBeInTheDocument()
    // The failed row holds no slot.
    expect(screen.getByText('Donated 6 / 10')).toBeInTheDocument()
    // The AI Studio pool reports no usage, so nothing is asked for.
    expect(screen.queryByText(/left/)).not.toBeInTheDocument()
    const urls = vi.mocked(api.get).mock.calls.map(([url]) => String(url))
    expect(urls.some((url) => url.includes('/usage'))).toBe(false)
  })

  test('Test calls the server and refreshes the list', async () => {
    items = [donation(1, 'spare@example.com')]
    renderPage()
    await screen.findByText('sp***@example.com')
    const before = listCalls
    await userEvent.click(
      within(cardOf('sp***@example.com')).getByRole('button', { name: 'Test' })
    )
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/wt/donations/1/test')
    )
    await waitFor(() => expect(listCalls).toBeGreaterThan(before))
  })

  test('Remove explains that the Cookie goes too, then calls the server', async () => {
    items = [donation(1, 'spare@example.com')]
    renderPage()
    await screen.findByText('sp***@example.com')
    await userEvent.click(
      within(cardOf('sp***@example.com')).getByRole('button', {
        name: 'Remove',
      })
    )
    expect(api.post).not.toHaveBeenCalled()
    const confirm = await screen.findByRole('alertdialog')
    expect(
      within(confirm).getByText('Remove this donated account?')
    ).toBeInTheDocument()
    expect(
      within(confirm).getByText(
        'The account leaves the pool now and its Cookie is deleted. You can donate it again later.'
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
        fail_reason: 'The pool lost the account; donate again',
        verified_at: 0,
      }),
    ]
    renderPage()
    await userEvent.click(await screen.findByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Step 1 of 3: Consent')).toBeInTheDocument()
  })

  test('at ten slots the donate button is disabled and says why', async () => {
    items = Array.from({ length: 10 }, (_, index) =>
      donation(index + 1, `spare${index + 1}@example.com`)
    )
    renderPage()
    expect(await screen.findByText('Donated 10 / 10')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Donate an AI Studio account' })
    ).toBeDisabled()
    expect(
      screen.getByText('You have reached the limit of 10 donated accounts')
    ).toBeInTheDocument()
  })
})

describe('the BUILD page', () => {
  test('with nothing donated it shows its own words and asks for its own donations', async () => {
    renderPage('build')
    expect(await screen.findByText('No BUILD accounts yet')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'BUILD' })).toBeInTheDocument()
    expect(
      screen.getByText(
        /every account you donate gives you 100 Build-gemini-3.8-flash calls a day/
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Donate a BUILD account' })
    ).toBeEnabled()
    expect(api.get).toHaveBeenCalledWith('/api/wt/donations', {
      params: { provider: 'build' },
    })
    expect(screen.queryByText('AIS playground')).not.toBeInTheDocument()
  })

  test('accounts go by their slot, with the pool state and whose turn it is', async () => {
    items = [
      buildDonation(1, { pool: buildPool({ index: 1, current: true }) }),
      buildDonation(2),
      buildDonation(3, {
        pool: buildPool({
          index: 3,
          state: 'expired',
          message:
            'The pool can no longer sign in with this account; remove it and donate again',
        }),
      }),
      buildDonation(4, {
        pool: buildPool({
          index: 4,
          state: 'invalid',
          message: 'The pool could not read the sign-in of this account',
        }),
      }),
      buildDonation(5, {
        pool: buildPool({
          index: 5,
          state: 'missing',
          message: 'The pool no longer has this account',
        }),
      }),
      buildDonation(6, { status: 'verifying', pool: null, verified_at: 0 }),
    ]
    renderPage('build')
    await screen.findByText('BUILD account #1')

    const serving = cardOf('BUILD account #1')
    expect(within(serving).getByText('Available')).toBeInTheDocument()
    expect(within(serving).getByText('Slot 1 in the pool')).toBeInTheDocument()
    expect(
      within(serving).getByText('Serving requests right now')
    ).toBeInTheDocument()
    const waiting = cardOf('BUILD account #2')
    expect(
      within(waiting).getByText('Waiting its turn in the rotation')
    ).toBeInTheDocument()

    const expired = cardOf('BUILD account #3')
    expect(within(expired).getByText('Sign-in expired')).toBeInTheDocument()
    expect(
      within(expired).getByText(
        'The pool can no longer sign in with this account; remove it and donate again'
      )
    ).toBeInTheDocument()
    expect(within(expired).queryByText(/turn/)).not.toBeInTheDocument()
    expect(
      within(cardOf('BUILD account #4')).getByText('Unreadable')
    ).toBeInTheDocument()
    expect(
      within(cardOf('BUILD account #5')).getByText('Not in the pool')
    ).toBeInTheDocument()

    // A row the pool has not placed yet has no slot, and no email either way.
    const pending = cardOf('BUILD account')
    expect(within(pending).getByText('Verifying')).toBeInTheDocument()
    expect(screen.queryByText('Google account')).not.toBeInTheDocument()
    expect(screen.queryByText(/@/)).not.toBeInTheDocument()

    expect(
      screen.queryByRole('button', { name: 'Authorize again' })
    ).not.toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: 'Test' })).toHaveLength(5)
    expect(screen.getByText('Donated 6 / 10')).toBeInTheDocument()
  })

  test('Remove explains that the sign-in goes too, then calls the server', async () => {
    items = [buildDonation(1)]
    renderPage('build')
    await screen.findByText('BUILD account #1')
    await userEvent.click(
      within(cardOf('BUILD account #1')).getByRole('button', {
        name: 'Remove',
      })
    )
    const confirm = await screen.findByRole('alertdialog')
    expect(
      within(confirm).getByText(
        'The account leaves the pool now and its sign-in is deleted. You can donate it again later.'
      )
    ).toBeInTheDocument()
    await userEvent.click(
      within(confirm).getByRole('button', { name: 'Remove' })
    )
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/wt/donations/1/remove')
    )
  })
})
