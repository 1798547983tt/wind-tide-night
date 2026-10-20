/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { render, screen, within } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import type { AisModelPool } from '../../../types'
import { SharedPool } from '../shared-pool'

const ARGON = 'AIS PG-gemini-4-argon'
const BUILD = 'Build-gemini-3.8-flash'

function pool(overrides: Partial<AisModelPool> = {}): AisModelPool {
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
    pool_balance: 30,
    pool_given: 40,
    pool_taken: 10,
    ...overrides,
  }
}

const buildPool = pool({
  provider: 'build',
  model: BUILD,
  relay_model: BUILD,
  pool_balance: 5,
  pool_given: 20,
  pool_taken: 15,
})

function sharedPool() {
  return screen.getByRole('region', { name: 'Shared pool' })
}

describe('the shared tide pool', () => {
  test('adds up the pools of the day and shows each model with its tide', () => {
    render(<SharedPool pools={[pool(), buildPool]} />)
    const shared = sharedPool()
    expect(
      within(shared).getByText('The tide everyone fills')
    ).toBeInTheDocument()
    // 30 + 5 in the pools, of 40 + 20 given; 10 + 15 taken.
    expect(within(shared).getByText('35')).toBeInTheDocument()
    expect(
      within(shared).getByText('calls in the pool right now')
    ).toBeInTheDocument()
    expect(within(shared).getByText('Given today')).toBeInTheDocument()
    expect(within(shared).getByText('60')).toBeInTheDocument()
    expect(within(shared).getByText('Taken today')).toBeInTheDocument()
    expect(within(shared).getByText('25')).toBeInTheDocument()
    expect(
      within(shared).getByText('58% of the gifts still in the pool')
    ).toBeInTheDocument()

    const rows = within(
      within(shared).getByRole('list', { name: 'Pools by model' })
    ).getAllByRole('listitem')
    expect(rows).toHaveLength(2)
    expect(rows[0]).toHaveTextContent(ARGON)
    expect(rows[0]).toHaveTextContent('30')
    expect(rows[0]).toHaveTextContent('Given 40 · taken 10 today')
    expect(
      within(rows[0]).getByRole('progressbar', { name: `${ARGON} tide` })
    ).toHaveAttribute('aria-valuenow', '75')
    expect(rows[1]).toHaveTextContent(BUILD)
    expect(rows[1]).toHaveTextContent('Given 20 · taken 15 today')
    expect(
      within(rows[1]).getByRole('progressbar', { name: `${BUILD} tide` })
    ).toHaveAttribute('aria-valuenow', '25')
  })

  test('with nothing given yet the tide stands at zero', () => {
    render(
      <SharedPool
        pools={[pool({ pool_balance: 0, pool_given: 0, pool_taken: 0 })]}
      />
    )
    const shared = sharedPool()
    expect(
      within(shared).getByText('0% of the gifts still in the pool')
    ).toBeInTheDocument()
    expect(
      within(shared).getByRole('progressbar', { name: `${ARGON} tide` })
    ).toHaveAttribute('aria-valuenow', '0')
    // The total, the day's gifts and takes, and the one model's balance.
    expect(within(shared).getAllByText('0')).toHaveLength(4)
  })

  test('a member who prefers reduced motion gets the figures at once, without the lanterns', () => {
    // The test setup answers prefers-reduced-motion: reduce.
    render(<SharedPool pools={[pool()]} />)
    expect(document.querySelectorAll('.wt-shared-pool-lantern')).toHaveLength(0)
    // The total and the one model's balance, both 30, with no count-up.
    expect(within(sharedPool()).getAllByText('30')).toHaveLength(2)
  })
})
