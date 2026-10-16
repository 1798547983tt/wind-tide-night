/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { describe, expect, test } from 'vitest'

import {
  getBeijingDay,
  isBeijingNight,
  msUntilDayNightSwitch,
} from '../beijing-time'

const HOUR = 60 * 60 * 1000

describe('getBeijingDay', () => {
  test('a UTC evening that is already past midnight in Beijing counts as the next day', () => {
    expect(getBeijingDay(new Date('2026-10-15T16:30:00Z'))).toBe('2026-10-16')
  })

  test('the last second before Beijing midnight still counts as the same day', () => {
    expect(getBeijingDay(new Date('2026-10-15T15:59:59Z'))).toBe('2026-10-15')
  })
})

describe('isBeijingNight', () => {
  test.each([
    ['17:59:59 in Beijing is day', '2026-10-15T09:59:59Z', false],
    ['18:00 in Beijing is night', '2026-10-15T10:00:00Z', true],
    ['05:59:59 in Beijing is night', '2026-10-15T21:59:59Z', true],
    ['06:00 in Beijing is day', '2026-10-15T22:00:00Z', false],
  ])('%s', (_name, iso, night) => {
    expect(isBeijingNight(new Date(iso))).toBe(night)
  })
})

describe('msUntilDayNightSwitch', () => {
  test.each([
    ['17:00 waits one hour for nightfall', '2026-10-15T09:00:00Z', HOUR],
    ['18:00 waits twelve hours for morning', '2026-10-15T10:00:00Z', 12 * HOUR],
    [
      '04:30 waits an hour and a half for morning',
      '2026-10-15T20:30:00Z',
      1.5 * HOUR,
    ],
    [
      '06:00 waits twelve hours for nightfall',
      '2026-10-15T22:00:00Z',
      12 * HOUR,
    ],
  ])('%s', (_name, iso, wait) => {
    expect(msUntilDayNightSwitch(new Date(iso))).toBe(wait)
  })
})
