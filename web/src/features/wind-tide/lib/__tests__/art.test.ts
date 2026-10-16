/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { afterEach, describe, expect, test, vi } from 'vitest'

import { coverWidth, pickArtUrl, type ArtEntry } from '../art'

const HERO: ArtEntry = {
  width: 2880,
  height: 1620,
  sizes: [720, 1080, 1600, 2160],
  depth: true,
}

function setScreen(width: number, height: number, dpr: number) {
  vi.stubGlobal('innerWidth', width)
  vi.stubGlobal('innerHeight', height)
  vi.stubGlobal('devicePixelRatio', dpr)
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('coverWidth', () => {
  test('a landscape painting on a tall phone needs enough pixels to fill the screen height', () => {
    setScreen(390, 844, 1)

    expect(coverWidth(HERO)).toBeCloseTo(844 * (2880 / 1620))
  })

  test('a landscape painting on a wider screen needs the screen width', () => {
    setScreen(1920, 900, 1)

    expect(coverWidth(HERO)).toBe(1920)
  })

  test('screen density counts only up to the painting canvas cap of 1.75', () => {
    setScreen(1920, 900, 3)

    expect(coverWidth(HERO)).toBe(1920 * 1.75)
  })
})

describe('pickArtUrl', () => {
  test('a tall high-density phone gets the full copy instead of a blurry small one', () => {
    setScreen(390, 844, 2)

    expect(pickArtUrl('hero-day', HERO, coverWidth(HERO))).toBe(
      '/art/hero-day-full.webp'
    )
  })

  test('the smallest listed width that covers the need is chosen', () => {
    expect(pickArtUrl('hero-day', HERO, 1000)).toBe('/art/hero-day-1080.webp')
  })

  test('without a manifest entry the full copy is used', () => {
    expect(pickArtUrl('hero-day', undefined, 1000)).toBe(
      '/art/hero-day-full.webp'
    )
  })
})
