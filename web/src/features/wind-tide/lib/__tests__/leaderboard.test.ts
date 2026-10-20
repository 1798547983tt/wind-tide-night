/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { describe, expect, test } from 'vitest'

import type { Leaderboard } from '../../types'
import { boardHasPeriod, boardIsStreak, gapToPlaceAbove } from '../leaderboard'

/** A calls board listing the given [rank, value] pairs. */
function board(ranked: [number, number][], me: Leaderboard['me']): Leaderboard {
  return {
    board: 'calls',
    period: 'today',
    items: ranked.map(([rank, value], index) => ({
      rank,
      user_id: index + 1,
      display_name: `member ${index + 1}`,
      avatar_url: '',
      value,
    })),
    me,
    updated_at: 1_760_000_000,
  }
}

describe('boards with a period', () => {
  test('the calls and failed boards are counted over a period', () => {
    expect(boardHasPeriod('calls')).toBe(true)
    expect(boardHasPeriod('failed')).toBe(true)
  })

  test('the streak and balance boards stand for the moment and have none', () => {
    expect(boardHasPeriod('success_streak')).toBe(false)
    expect(boardHasPeriod('fail_streak')).toBe(false)
    expect(boardHasPeriod('balance')).toBe(false)
  })
})

describe('streak boards', () => {
  test('the success and fail streak boards count runs in a row', () => {
    expect(boardIsStreak('success_streak')).toBe(true)
    expect(boardIsStreak('fail_streak')).toBe(true)
  })

  test('the calls, failed and balance boards count totals, not runs', () => {
    expect(boardIsStreak('calls')).toBe(false)
    expect(boardIsStreak('failed')).toBe(false)
    expect(boardIsStreak('balance')).toBe(false)
  })
})

describe('gap to the place above', () => {
  test('a member in the list lacks the difference to the closest value ahead', () => {
    const top = board(
      [
        [1, 40],
        [2, 25],
        [3, 12],
      ],
      { rank: 3, value: 12 }
    )

    expect(gapToPlaceAbove(top)).toBe(13)
  })

  test('behind a tie, the gap is to the tied value and not past it', () => {
    const tied = board(
      [
        [1, 40],
        [2, 25],
        [2, 25],
        [4, 9],
      ],
      { rank: 4, value: 9 }
    )

    expect(gapToPlaceAbove(tied)).toBe(16)
  })

  test('the member at the top has nobody to chase', () => {
    expect(
      gapToPlaceAbove(board([[1, 40]], { rank: 1, value: 40 }))
    ).toBeUndefined()
  })

  test('sharing first place leaves nobody to chase', () => {
    const shared = board(
      [
        [1, 40],
        [1, 40],
      ],
      { rank: 1, value: 40 }
    )

    expect(gapToPlaceAbove(shared)).toBeUndefined()
  })

  test('a member without any calls is not on the board yet', () => {
    // Everyone at nought shares rank 1, which is no place to report.
    expect(gapToPlaceAbove(board([], { rank: 1, value: 0 }))).toBeUndefined()
    expect(
      gapToPlaceAbove(board([[1, 40]], { rank: 2, value: 0 }))
    ).toBeUndefined()
  })

  test('the owner, who is not ranked, has no gap', () => {
    expect(gapToPlaceAbove(board([[1, 40]], null))).toBeUndefined()
  })

  test('below the listed top 50 the place above is unknown, so no gap is made up', () => {
    // Only two of the members ahead are listed; the closest one is not.
    const cut = board(
      [
        [1, 40],
        [2, 25],
      ],
      { rank: 60, value: 3 }
    )

    expect(gapToPlaceAbove(cut)).toBeUndefined()
  })
})
