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

import type {
  AisModelPool,
  AisPoolStatus,
  PoolEvent,
  PoolOverview,
  WbPoolLevel,
} from '../../../types'
import { PoolPage } from '../pool-page'

const ARGON = 'AIS PG-gemini-4-argon'
const FLASH = 'AIS PG-gemini-3.8-flash'
const BUILD = 'Build-gemini-3.8-flash'

let overview: PoolOverview
let poolCalls: number

function argonPool(overrides: Partial<AisModelPool> = {}): AisModelPool {
  return {
    provider: 'aistudio',
    model: 'gemini-4-argon',
    relay_model: ARGON,
    per_account: 50,
    take_max: 100,
    allowance: 100,
    calls: 25,
    failed_today: 1,
    given: 0,
    taken: 0,
    remaining: 75,
    take_left: 100,
    pool_balance: 40,
    pool_given: 40,
    pool_taken: 0,
    ...overrides,
  }
}

function flashPool(overrides: Partial<AisModelPool> = {}): AisModelPool {
  return argonPool({
    model: 'gemini-3.8-flash',
    relay_model: FLASH,
    per_account: 100,
    allowance: 200,
    calls: 20,
    failed_today: 0,
    remaining: 180,
    pool_balance: 5,
    pool_given: 8,
    pool_taken: 3,
    ...overrides,
  })
}

/** The BUILD pool's one model: 100 calls an account, a public pool of its own. */
function buildPool(overrides: Partial<AisModelPool> = {}): AisModelPool {
  return argonPool({
    provider: 'build',
    model: BUILD,
    relay_model: BUILD,
    per_account: 100,
    allowance: 100,
    calls: 10,
    failed_today: 0,
    remaining: 90,
    pool_balance: 12,
    pool_given: 20,
    pool_taken: 8,
    ...overrides,
  })
}

function aisStatus(overrides: Partial<AisPoolStatus> = {}): AisPoolStatus {
  return {
    provider: 'aistudio',
    day: '2026-10-19',
    accounts: 2,
    models: [argonPool(), flashPool()],
    ...overrides,
  }
}

function buildStatus(overrides: Partial<AisPoolStatus> = {}): AisPoolStatus {
  return {
    provider: 'build',
    day: '2026-10-19',
    accounts: 1,
    models: [buildPool()],
    ...overrides,
  }
}

function event(id: number, overrides: Partial<PoolEvent> = {}): PoolEvent {
  return {
    id,
    kind: 'ais_given',
    provider: 'aistudio',
    display_name: 'Mizuki',
    amount: 5,
    model: 'gemini-4-argon',
    created_at: Math.floor(Date.now() / 1000) - 60 * id,
    ...overrides,
  }
}

function wbLevel(overrides: Partial<WbPoolLevel> = {}): WbPoolLevel {
  return {
    fetched_at: Math.floor(Date.now() / 1000) - 60,
    accounts: 3,
    healthy: 2,
    cooling: 1,
    credits: 400,
    credits_total: 1000,
    ...overrides,
  }
}

function renderPage() {
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
    path: '/donations/',
    component: PoolPage,
  })
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren([page])]),
    history: createMemoryHistory({ initialEntries: ['/donations'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

/** A gauge, found by the name it announces. */
async function gaugeOf(name: string) {
  return screen.findByRole('region', { name })
}

async function openDialog(gauge: HTMLElement, name: string, title: string) {
  await userEvent.click(within(gauge).getByRole('button', { name }))
  await screen.findByText(title)
  return screen.getByLabelText('Number of calls')
}

async function submit(input: HTMLElement, raw: string, button: string) {
  await userEvent.clear(input)
  await userEvent.type(input, raw)
  await userEvent.click(screen.getByRole('button', { name: button }))
}

beforeEach(() => {
  poolCalls = 0
  overview = {
    antigravity: {
      fetched_at: Math.floor(Date.now() / 1000) - 120,
      accounts: 4,
      healthy: 3,
      buckets: [
        { name: 'Gemini 3 Pro', window: '5h', average_remaining_fraction: 0.8 },
        {
          name: 'Gemini 3 Pro',
          window: 'weekly',
          average_remaining_fraction: 0.6,
        },
      ],
    },
    aistudio: aisStatus(),
    build: buildStatus(),
    workbuddy: null,
    events: [
      event(3, {
        kind: 'ais_taken',
        display_name: '',
        amount: 3,
        model: 'gemini-3.8-flash',
      }),
      event(2, { kind: 'ais_account_donated', display_name: 'Ren', model: '' }),
      event(1),
    ],
  }
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/wt/pool') {
      poolCalls += 1
      return { data: { success: true, data: overview } }
    }
    if (url === '/api/wt/pool/events') {
      return { data: { success: true, data: { items: [] } } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
  vi.spyOn(api, 'post').mockImplementation(async (url, body) => {
    const input = body as { model: string; count: number }
    if (url === '/api/wt/pool/ais/give') {
      // The argon pool moves the way the server would move it.
      overview = {
        ...overview,
        aistudio: aisStatus({
          models: [
            argonPool({
              remaining: 75 - input.count,
              given: input.count,
              pool_balance: 40 + input.count,
            }),
            flashPool(),
          ],
        }),
      }
      return {
        data: {
          success: true,
          data: {
            model: input.model,
            remaining: 75 - input.count,
            pool_balance: 40 + input.count,
          },
        },
      }
    }
    if (url === '/api/wt/pool/ais/take') {
      return {
        data: {
          success: true,
          data: {
            model: input.model,
            remaining: 180 + input.count,
            pool_balance: 5 - input.count,
            take_left: 100 - input.count,
          },
        },
      }
    }
    throw new Error(`Unexpected POST ${url}`)
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the tanks', () => {
  test('show each pool at its level with its figures', async () => {
    renderPage()
    expect(
      await screen.findByText('3 / 4 accounts healthy')
    ).toBeInTheDocument()

    const antigravity = await gaugeOf('Antigravity')
    // The mean of 80% and 60%.
    expect(within(antigravity).getByText('70%')).toBeInTheDocument()
    expect(
      within(antigravity).getByRole('img', { name: '70% full' })
    ).toBeInTheDocument()
    expect(
      within(antigravity).getByText('Gemini 3 Pro · Every 5 hours · 80%')
    ).toBeInTheDocument()
    expect(
      within(antigravity).getByText('Gemini 3 Pro · Every week · 60%')
    ).toBeInTheDocument()
    expect(within(antigravity).getByText(/^Updated /)).toBeInTheDocument()

    expect(screen.getByText('Your active accounts: 2')).toBeInTheDocument()
    const argon = await gaugeOf(ARGON)
    expect(within(argon).getByText('75')).toBeInTheDocument()
    expect(within(argon).getByText('/ 100')).toBeInTheDocument()
    expect(
      within(argon).getByRole('img', { name: '75% full' })
    ).toBeInTheDocument()
    expect(within(argon).getByText('Public pool today: 40')).toBeInTheDocument()
    expect(within(argon).getByText('Used today: 25')).toBeInTheDocument()
    expect(
      within(argon).getByText('You can still take 100 today')
    ).toBeInTheDocument()
    expect(
      within(argon).getByRole('button', { name: 'Give calls' })
    ).toBeEnabled()
    expect(
      within(argon).getByRole('button', { name: 'Take calls' })
    ).toBeEnabled()

    const flash = await gaugeOf(FLASH)
    expect(within(flash).getByText('180')).toBeInTheDocument()
    expect(within(flash).getByText('/ 200')).toBeInTheDocument()
    expect(
      within(flash).getByRole('img', { name: '90% full' })
    ).toBeInTheDocument()
    expect(within(flash).getByText('Public pool today: 5')).toBeInTheDocument()
    expect(
      screen.queryByText('Donate an AI Studio account to unlock')
    ).not.toBeInTheDocument()

    // The BUILD tank stands beside, with its own pool's figures.
    expect(screen.getByText('Your active accounts: 1')).toBeInTheDocument()
    const build = await gaugeOf(BUILD)
    expect(within(build).getByText('90')).toBeInTheDocument()
    expect(within(build).getByText('/ 100')).toBeInTheDocument()
    expect(
      within(build).getByRole('img', { name: '90% full' })
    ).toBeInTheDocument()
    expect(within(build).getByText('Public pool today: 12')).toBeInTheDocument()
    expect(within(build).getByText('Used today: 10')).toBeInTheDocument()
    expect(
      screen.queryByText('Donate a BUILD account to unlock')
    ).not.toBeInTheDocument()
  })

  test('the shared pool on top adds up every public pool of the day', async () => {
    renderPage()
    const shared = await screen.findByRole('region', { name: 'Shared pool' })
    expect(
      within(shared).getByText('The tide everyone fills')
    ).toBeInTheDocument()
    // 40 + 5 + 12 in the pools; 40 + 8 + 20 given; 0 + 3 + 8 taken.
    expect(within(shared).getByText('57')).toBeInTheDocument()
    expect(within(shared).getByText('Given today')).toBeInTheDocument()
    expect(within(shared).getByText('68')).toBeInTheDocument()
    expect(within(shared).getByText('Taken today')).toBeInTheDocument()
    expect(within(shared).getByText('11')).toBeInTheDocument()
    expect(
      within(shared).getByText('84% of the gifts still in the pool')
    ).toBeInTheDocument()
    const lanterns = within(
      within(shared).getByRole('list', { name: 'Pools by model' })
    ).getAllByRole('listitem')
    expect(lanterns).toHaveLength(3)
    expect(lanterns[0]).toHaveTextContent(ARGON)
    expect(lanterns[0]).toHaveTextContent('Given 40 · taken 0 today')
    expect(lanterns[2]).toHaveTextContent(BUILD)
    expect(lanterns[2]).toHaveTextContent('Given 20 · taken 8 today')
    expect(
      within(shared).getByRole('progressbar', { name: `${BUILD} tide` })
    ).toHaveAttribute('aria-valuenow', '60')
  })

  test('before the first reading the Antigravity tank says so', async () => {
    overview = {
      ...overview,
      antigravity: { fetched_at: 0, accounts: 0, healthy: 0, buckets: [] },
    }
    renderPage()
    expect(
      await screen.findByText(
        'No data yet; the first reading arrives within five minutes'
      )
    ).toBeInTheDocument()
    expect(screen.queryByText(/accounts healthy/)).not.toBeInTheDocument()
  })

  test('a pool the site does not run has no tank, and no shared pool without a Cookie pool', async () => {
    overview = { ...overview, antigravity: null, aistudio: null, build: null }
    renderPage()
    expect(
      await screen.findByText('No pool is reporting right now')
    ).toBeInTheDocument()
    expect(screen.queryByText(/accounts healthy/)).not.toBeInTheDocument()
    expect(screen.queryByText(/Public pool today/)).not.toBeInTheDocument()
    expect(
      screen.queryByRole('region', { name: 'Shared pool' })
    ).not.toBeInTheDocument()
    // The boards left this page for /rankings.
    const urls = vi.mocked(api.get).mock.calls.map(([url]) => String(url))
    expect(urls).not.toContain('/api/wt/pool/ais/leaderboard')
    expect(urls).not.toContain('/api/wt/leaderboard')
  })

  test('with only the Antigravity pool there is a tank but no shared pool', async () => {
    overview = { ...overview, aistudio: null, build: null }
    renderPage()
    expect(
      await screen.findByText('3 / 4 accounts healthy')
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('region', { name: 'Shared pool' })
    ).not.toBeInTheDocument()
  })

  test('a member without an AI Studio account sees the panel locked, with the way in', async () => {
    overview = {
      ...overview,
      build: null,
      aistudio: aisStatus({
        accounts: 0,
        models: [
          argonPool({ allowance: 0, remaining: 0, calls: 0 }),
          flashPool({ allowance: 0, remaining: 0, calls: 0 }),
        ],
      }),
    }
    renderPage()
    const unlock = await screen.findByRole('link', {
      name: 'Donate an AI Studio account to unlock',
    })
    expect(unlock).toHaveAttribute('href', '/donations?tab=aistudio')
    expect(
      screen.queryByRole('button', { name: 'Give calls' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Take calls' })
    ).not.toBeInTheDocument()
    expect(screen.getAllByRole('img', { name: '0% full' })).toHaveLength(2)
  })

  test('a member without a BUILD account sees that panel locked, with its own way in', async () => {
    overview = {
      ...overview,
      aistudio: null,
      build: buildStatus({
        accounts: 0,
        models: [buildPool({ allowance: 0, remaining: 0, calls: 0 })],
      }),
    }
    renderPage()
    const unlock = await screen.findByRole('link', {
      name: 'Donate a BUILD account to unlock',
    })
    expect(unlock).toHaveAttribute('href', '/donations?tab=build')
    expect(
      screen.queryByRole('button', { name: 'Give calls' })
    ).not.toBeInTheDocument()
    expect(screen.getAllByRole('img', { name: '0% full' })).toHaveLength(1)
  })
})

describe('the WorkBuddy tank', () => {
  test('shows the credits left across the pool, its accounts and the way to donate', async () => {
    overview = { ...overview, workbuddy: wbLevel() }
    renderPage()
    const tank = await gaugeOf('WorkBuddy')
    expect(within(tank).getByText('40%')).toBeInTheDocument()
    expect(
      within(tank).getByRole('img', { name: '40% full' })
    ).toBeInTheDocument()
    expect(
      within(tank).getByText('Credits left across the pool')
    ).toBeInTheDocument()
    expect(within(tank).getByText('400 / 1,000 credits')).toBeInTheDocument()
    expect(within(tank).getByText('2 / 3 accounts healthy')).toBeInTheDocument()
    expect(within(tank).getByText('1 cooling down')).toBeInTheDocument()
    expect(within(tank).getByText(/^Updated /)).toBeInTheDocument()
    expect(
      within(tank).getByRole('link', { name: 'Donate a WorkBuddy account' })
    ).toHaveAttribute('href', '/donations?tab=workbuddy')
    // The pool has no gate, so nothing stands in front of the tank.
    expect(screen.queryByText(/to unlock$/)).not.toBeInTheDocument()
  })

  test('while the pool is not answering, the tank stands empty and says so', async () => {
    overview = {
      ...overview,
      workbuddy: wbLevel({
        fetched_at: 0,
        accounts: 0,
        healthy: 0,
        cooling: 0,
        credits: 0,
        credits_total: 0,
      }),
    }
    renderPage()
    const tank = await gaugeOf('WorkBuddy')
    expect(
      within(tank).getByText('The pool is not answering right now')
    ).toBeInTheDocument()
    expect(within(tank).getByText('—')).toBeInTheDocument()
    expect(within(tank).queryByText(/credits/)).not.toBeInTheDocument()
    expect(within(tank).queryByRole('link')).not.toBeInTheDocument()
  })

  test('with no account yet, the tank asks for the first one', async () => {
    overview = {
      ...overview,
      workbuddy: wbLevel({
        accounts: 0,
        healthy: 0,
        cooling: 0,
        credits: 0,
        credits_total: 0,
      }),
    }
    renderPage()
    const tank = await gaugeOf('WorkBuddy')
    expect(
      within(tank).getByText(
        'No account in the pool yet; be the first to donate one'
      )
    ).toBeInTheDocument()
    expect(within(tank).getByText('—')).toBeInTheDocument()
    expect(within(tank).queryByText(/accounts healthy/)).not.toBeInTheDocument()
    expect(
      within(tank).getByRole('link', { name: 'Donate a WorkBuddy account' })
    ).toBeInTheDocument()
  })

  test('without the WorkBuddy pool there is no such tank', async () => {
    renderPage()
    await screen.findByText('3 / 4 accounts healthy')
    expect(
      screen.queryByRole('region', { name: 'WorkBuddy' })
    ).not.toBeInTheDocument()
  })
})

describe('giving and taking calls', () => {
  test('Give checks the number here, then sends it for that model and refreshes the pool', async () => {
    renderPage()
    const argon = await gaugeOf(ARGON)
    const input = await openDialog(
      argon,
      'Give calls',
      'Give calls to the pool'
    )
    expect(
      screen.getByText(/You have 75 gemini-4-argon calls left today/)
    ).toBeInTheDocument()

    await submit(input, 'abc', 'Give')
    expect(
      await screen.findByText('Enter a whole number of calls, 1 or more')
    ).toBeInTheDocument()
    await submit(input, '80', 'Give')
    expect(
      await screen.findByText('You can only give calls you still have today')
    ).toBeInTheDocument()
    expect(api.post).not.toHaveBeenCalled()

    const before = poolCalls
    await submit(input, '5', 'Give')
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/wt/pool/ais/give', {
        model: 'gemini-4-argon',
        count: 5,
      })
    )
    await waitFor(() => expect(poolCalls).toBeGreaterThan(before))
    await waitFor(() =>
      expect(
        screen.queryByText('Give calls to the pool')
      ).not.toBeInTheDocument()
    )
    expect(
      await within(await gaugeOf(ARGON)).findByText('Public pool today: 45')
    ).toBeInTheDocument()
    expect(within(await gaugeOf(ARGON)).getByText('70')).toBeInTheDocument()
  })

  test('Take checks the daily limit and that pool, then sends for that model', async () => {
    renderPage()
    const flash = await gaugeOf(FLASH)
    const input = await openDialog(
      flash,
      'Take calls',
      'Take calls from the pool'
    )
    expect(
      screen.getByText(
        /The pool holds 5 gemini-3.8-flash calls today, and you can still take 100/
      )
    ).toBeInTheDocument()

    await submit(input, '150', 'Take')
    expect(
      await screen.findByText(
        'You can take at most 100 calls a day from the pool'
      )
    ).toBeInTheDocument()
    await submit(input, '10', 'Take')
    expect(
      await screen.findByText(
        'The pool does not hold that many calls right now'
      )
    ).toBeInTheDocument()
    expect(api.post).not.toHaveBeenCalled()

    await submit(input, '3', 'Take')
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/wt/pool/ais/take', {
        model: 'gemini-3.8-flash',
        count: 3,
      })
    )
    await waitFor(() =>
      expect(
        screen.queryByText('Take calls from the pool')
      ).not.toBeInTheDocument()
    )
  })

  test('a refusal from the server shows inside the dialog and keeps it open', async () => {
    vi.mocked(api.post).mockResolvedValueOnce({
      data: {
        success: false,
        message: 'The pool does not hold that many calls right now',
      },
    })
    renderPage()
    const argon = await gaugeOf(ARGON)
    const input = await openDialog(
      argon,
      'Take calls',
      'Take calls from the pool'
    )
    await submit(input, '10', 'Take')
    expect(
      await screen.findByText(
        'The pool does not hold that many calls right now'
      )
    ).toBeInTheDocument()
    expect(screen.getByText('Take calls from the pool')).toBeInTheDocument()
    expect(input).toHaveValue('10')
  })
})

describe('the announcements', () => {
  test('read as sentences, newest first, naming the member or a stand-in', async () => {
    renderPage()
    const title = await screen.findByText('Announcements')
    const ticker = title.closest('[data-slot="card"]') as HTMLElement | null
    if (!ticker) throw new Error('No announcements card')
    const rows = within(ticker).getAllByRole('listitem')
    expect(rows).toHaveLength(3)
    expect(rows[0]).toHaveTextContent(
      'Someone took 3 gemini-3.8-flash calls from the pool'
    )
    expect(rows[1]).toHaveTextContent('Ren donated an AI Studio account')
    expect(rows[2]).toHaveTextContent(
      'Mizuki gave 5 gemini-4-argon calls to the pool'
    )
    expect(rows[0]).toHaveTextContent(/ago/)
  })

  test('with nothing to tell, the feed says so', async () => {
    overview = { ...overview, events: [] }
    renderPage()
    expect(
      await screen.findByText('Nothing has happened yet')
    ).toBeInTheDocument()
  })
})
