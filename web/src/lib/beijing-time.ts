/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/

/**
 * Beijing time (UTC+8, no daylight saving) helpers.
 *
 * 风与潮之夜 counts check-in days and switches between the day theme and the
 * night theme by Beijing time, whatever the visitor's own time zone is.
 */

const BEIJING_OFFSET_MS = 8 * 60 * 60 * 1000
const HOUR_MS = 60 * 60 * 1000

/** Night runs from 18:00 to 06:00 Beijing time. */
export const NIGHT_START_HOUR = 18
export const NIGHT_END_HOUR = 6

/** Hour of the day, 0–23, in Beijing time. */
export function getBeijingHour(now: Date = new Date()): number {
  return new Date(now.getTime() + BEIJING_OFFSET_MS).getUTCHours()
}

export function isBeijingNight(now: Date = new Date()): boolean {
  const hour = getBeijingHour(now)
  return hour >= NIGHT_START_HOUR || hour < NIGHT_END_HOUR
}

/** Milliseconds until the next 06:00 or 18:00 in Beijing time. */
export function msUntilDayNightSwitch(now: Date = new Date()): number {
  const shifted = new Date(now.getTime() + BEIJING_OFFSET_MS)
  const hour = shifted.getUTCHours()
  const elapsedInHour =
    shifted.getUTCMinutes() * 60_000 +
    shifted.getUTCSeconds() * 1000 +
    shifted.getUTCMilliseconds()

  let nextHour = NIGHT_START_HOUR
  if (hour >= NIGHT_START_HOUR) nextHour = 24 + NIGHT_END_HOUR
  else if (hour < NIGHT_END_HOUR) nextHour = NIGHT_END_HOUR

  return (nextHour - hour) * HOUR_MS - elapsedInHour
}

/** The check-in day, `YYYY-MM-DD`, in Beijing time. */
export function getBeijingDay(now: Date = new Date()): string {
  return new Date(now.getTime() + BEIJING_OFFSET_MS).toISOString().slice(0, 10)
}
