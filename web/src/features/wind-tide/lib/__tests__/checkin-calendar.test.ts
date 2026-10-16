/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { describe, expect, test } from 'vitest'

import { monthGrid, shiftMonth } from '../checkin-calendar'

describe('check-in month grid', () => {
  test('a month that starts on a Thursday opens with three blank cells', () => {
    const grid = monthGrid('2026-10')

    expect(grid.slice(0, 4)).toEqual([null, null, null, '2026-10-01'])
    expect(grid.at(-1)).toBe('2026-10-31')
    expect(grid).toHaveLength(3 + 31)
  })

  test('a month that starts on a Monday has no blank cells', () => {
    expect(monthGrid('2026-06')[0]).toBe('2026-06-01')
  })

  test('a month that starts on a Sunday fills the first week with six blanks', () => {
    const grid = monthGrid('2026-02')

    expect(grid.indexOf('2026-02-01')).toBe(6)
    expect(grid.at(-1)).toBe('2026-02-28')
  })

  test('February of a leap year runs to the 29th', () => {
    expect(monthGrid('2028-02').at(-1)).toBe('2028-02-29')
  })
})

describe('moving between check-in months', () => {
  test('going back from January lands in December of the year before', () => {
    expect(shiftMonth('2026-01', -1)).toBe('2025-12')
  })

  test('going on from December lands in January of the next year', () => {
    expect(shiftMonth('2026-12', 1)).toBe('2027-01')
  })
})
