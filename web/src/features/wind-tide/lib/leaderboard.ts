/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import type { Leaderboard, LeaderboardBoard } from '../types'

/**
 * The boards counted over a period, today, the last 7 days or all time; the
 * streak and balance boards stand for the moment and have none.
 */
export function boardHasPeriod(board: LeaderboardBoard): boolean {
  return board === 'calls' || board === 'failed'
}

/** The boards whose value is the longest run, shown beside the run right now. */
export function boardIsStreak(board: LeaderboardBoard): boolean {
  return board === 'success_streak' || board === 'fail_streak'
}

/**
 * How much the requester lacks to draw level with the place right above
 * theirs, or undefined when there is nothing to chase: at the top, before
 * they have any calls or balance, for the unranked owner, or when that place
 * lies below the listed top 50 and its value is unknown.
 *
 * Ranks count the members ahead (1, 2, 2, 4), so a member ranked n has n − 1
 * members with more; all of them must be listed to know the closest one.
 */
export function gapToPlaceAbove(board: Leaderboard): number | undefined {
  const me = board.me
  if (!me || me.value <= 0 || me.rank <= 1) return undefined
  const ahead = board.items.filter((item) => item.value > me.value)
  if (ahead.length === 0 || ahead.length !== me.rank - 1) return undefined
  return Math.min(...ahead.map((item) => item.value)) - me.value
}
