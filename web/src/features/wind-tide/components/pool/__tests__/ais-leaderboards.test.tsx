/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import type {
  AisLeaderboard,
  AisLeaderboardEntry,
  AisModelLeaderboard,
} from '../../../types'
import { AisLeaderboards } from '../ais-leaderboards'

let board: AisLeaderboard

function entries(names: string[], first = 100): AisLeaderboardEntry[] {
  return names.map((display_name, index) => ({
    rank: index + 1,
    display_name,
    count: first - index * 10,
  }))
}

function argon(): AisModelLeaderboard {
  return {
    model: 'gemini-4-argon',
    relay_model: 'AIS PG-gemini-4-argon',
    today: {
      calls: entries(['Mizuki', 'Ren', 'Sora', 'Yuki']),
      failed: entries(['Ren'], 3),
    },
    total: {
      calls: entries(['Ren', 'Mizuki'], 900),
      failed: [],
    },
  }
}

function flash(): AisModelLeaderboard {
  return {
    model: 'gemini-3.8-flash',
    relay_model: 'AIS PG-gemini-3.8-flash',
    today: { calls: entries(['Hana'], 42), failed: [] },
    total: {
      calls: entries(['Hana', 'Ren'], 300),
      failed: entries(['Sora'], 9),
    },
  }
}

function renderBoards() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <AisLeaderboards />
    </QueryClientProvider>
  )
}

async function boardOf(title: string) {
  const card = (await screen.findByText(title)).closest('[data-slot="card"]')
  if (!card) throw new Error(`No board titled ${title}`)
  return card as HTMLElement
}

beforeEach(() => {
  board = { models: [argon(), flash()] }
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/wt/pool/ais/leaderboard') {
      return { data: { success: true, data: board } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the AIS PG leaderboards', () => {
  test('open on the first model and today, in rank order, with the counts', async () => {
    renderBoards()
    const calls = await boardOf('AIS PG calls')
    expect(await within(calls).findByText('Mizuki')).toBeInTheDocument()
    const rows = within(calls).getAllByRole('listitem')
    expect(rows).toHaveLength(4)
    expect(rows[0]).toHaveTextContent('No. 1')
    expect(rows[0]).toHaveTextContent('Mizuki')
    expect(rows[0]).toHaveTextContent('100')
    expect(rows[3]).toHaveTextContent('No. 4')
    expect(rows[3]).toHaveTextContent('Yuki')
    expect(rows[3]).toHaveTextContent('70')

    const failed = await boardOf('Failed calls')
    const failedRows = within(failed).getAllByRole('listitem')
    expect(failedRows).toHaveLength(1)
    expect(failedRows[0]).toHaveTextContent('Ren')
    expect(failedRows[0]).toHaveTextContent('3')
  })

  test('each board switches to the cumulative figures on its own', async () => {
    renderBoards()
    const calls = await boardOf('AIS PG calls')
    await within(calls).findByText('Mizuki')
    await userEvent.click(within(calls).getByText('Cumulative'))
    expect(await within(calls).findByText('900')).toBeInTheDocument()
    const rows = within(calls).getAllByRole('listitem')
    expect(rows).toHaveLength(2)
    expect(rows[0]).toHaveTextContent('Ren')
    // The other board keeps today's figures until its own toggle is used.
    const failed = await boardOf('Failed calls')
    expect(within(failed).getByText('Ren')).toBeInTheDocument()
    await userEvent.click(within(failed).getByText('Cumulative'))
    expect(await within(failed).findByText('Nobody yet')).toBeInTheDocument()
    expect(within(calls).getAllByRole('listitem')).toHaveLength(2)
  })

  test('each board switches model on its own, keeping its period', async () => {
    renderBoards()
    const calls = await boardOf('AIS PG calls')
    await within(calls).findByText('Mizuki')
    await userEvent.click(within(calls).getByText('gemini-3.8-flash'))
    expect(await within(calls).findByText('Hana')).toBeInTheDocument()
    expect(within(calls).getAllByRole('listitem')).toHaveLength(1)
    expect(within(calls).getByText('42')).toBeInTheDocument()
    expect(within(calls).queryByText('Mizuki')).not.toBeInTheDocument()

    const failed = await boardOf('Failed calls')
    expect(within(failed).getByText('Ren')).toBeInTheDocument()
    await userEvent.click(within(failed).getByText('Cumulative'))
    await userEvent.click(within(failed).getByText('gemini-3.8-flash'))
    expect(await within(failed).findByText('Sora')).toBeInTheDocument()
    expect(within(failed).getByText('9')).toBeInTheDocument()
  })

  test('with a single model there is no model switch', async () => {
    board = { models: [argon()] }
    renderBoards()
    const calls = await boardOf('AIS PG calls')
    await within(calls).findByText('Mizuki')
    expect(
      within(calls).queryByRole('group', { name: 'Model' })
    ).not.toBeInTheDocument()
    expect(
      within(calls).getByRole('group', { name: 'Period' })
    ).toBeInTheDocument()
  })

  test('shows at most twenty rows', async () => {
    const crowded = argon()
    crowded.today.calls = entries(
      Array.from({ length: 25 }, (_, index) => `member-${index + 1}`),
      500
    )
    board = { models: [crowded, flash()] }
    renderBoards()
    const calls = await boardOf('AIS PG calls')
    await within(calls).findByText('member-1')
    expect(within(calls).getAllByRole('listitem')).toHaveLength(20)
    expect(within(calls).queryByText('member-21')).not.toBeInTheDocument()
  })

  test('a server failure reads as a sentence, not a blank', async () => {
    vi.mocked(api.get).mockResolvedValueOnce({
      data: {
        success: false,
        message: 'The AIS playground pool is not configured',
      },
    })
    renderBoards()
    expect(
      await screen.findByText('The AIS playground pool is not configured')
    ).toBeInTheDocument()
  })
})
