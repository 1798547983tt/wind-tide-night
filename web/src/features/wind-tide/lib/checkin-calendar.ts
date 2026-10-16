/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/

// Check-in days are Beijing-time dates that the server names as `YYYY-MM-DD`
// (wind-tide/API.md §6). The calendar works on those names with UTC
// arithmetic, so the browser's own time zone never moves a day.

/** The month `offset` months after a `YYYY-MM` month; negative goes back. */
export function shiftMonth(month: string, offset: number) {
  const [year, monthNumber] = month.split('-').map(Number)
  const first = new Date(Date.UTC(year, monthNumber - 1 + offset, 1))
  const shifted = String(first.getUTCMonth() + 1).padStart(2, '0')
  return `${first.getUTCFullYear()}-${shifted}`
}

/**
 * The days of a `YYYY-MM` month in weeks that start on Monday, as Chinese
 * calendars do, with null for the cells before the 1st.
 */
export function monthGrid(month: string): Array<string | null> {
  const [year, monthNumber] = month.split('-').map(Number)
  const weekday = new Date(Date.UTC(year, monthNumber - 1, 1)).getUTCDay()
  const length = new Date(Date.UTC(year, monthNumber, 0)).getUTCDate()
  const blanks: null[] = Array.from({ length: (weekday + 6) % 7 }, () => null)
  const days = Array.from(
    { length },
    (_, index) => `${month}-${String(index + 1).padStart(2, '0')}`
  )
  return [...blanks, ...days]
}
