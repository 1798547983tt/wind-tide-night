/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { describe, expect, test } from 'vitest'

import { getUserAvatarStyle } from '../avatar'

/** WCAG 2.1 contrast ratio of white text on a #rrggbb background. */
function contrastWithWhite(hex: string) {
  const [red, green, blue] = [1, 3, 5].map((start) => {
    const channel = Number.parseInt(hex.slice(start, start + 2), 16) / 255
    return channel <= 0.04045
      ? channel / 12.92
      : ((channel + 0.055) / 1.055) ** 2.4
  })
  const luminance = 0.2126 * red + 0.7152 * green + 0.0722 * blue
  return 1.05 / (luminance + 0.05)
}

describe('avatar fallback colours', () => {
  test('a name always gets the same colour', () => {
    expect(getUserAvatarStyle('Nagisa')).toEqual(getUserAvatarStyle('Nagisa'))
  })

  test('white initials stay readable (WCAG AA) on whatever colour a name gets', () => {
    const names = [
      ...'ABCDEFGHIJKLMNOPQRSTUVWXYZ',
      '潮汐守望者',
      '夜风',
      'ルナ',
      '海月くらげ',
      'Mizuki_0721',
    ]

    for (const name of names) {
      const style = getUserAvatarStyle(name)
      expect(style.color).toBe('white')
      expect(style.backgroundColor).toMatch(/^#[0-9a-f]{6}$/)
      expect(
        contrastWithWhite(String(style.backgroundColor)),
        name
      ).toBeGreaterThanOrEqual(4.5)
    }
  })
})
