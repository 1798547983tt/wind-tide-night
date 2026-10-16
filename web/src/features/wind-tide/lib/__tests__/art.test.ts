/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { afterEach, describe, expect, test, vi } from 'vitest'

import {
  coverWidth,
  pickArtUrl,
  stageArtName,
  type ArtEntry,
  type ArtManifest,
} from '../art'

const HERO: ArtEntry = {
  width: 2880,
  height: 1620,
  sizes: [720, 1080, 1600, 2160],
  depth: true,
}

function manifestOf(...names: string[]): ArtManifest {
  return Object.fromEntries(names.map((name) => [name, HERO]))
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

  test('a painting filling half the screen width needs only that share when it is wider than the height allows', () => {
    setScreen(3840, 900, 1)

    expect(coverWidth(HERO, 0.5)).toBe(1920)
  })
})

describe('stageArtName', () => {
  const full = manifestOf(
    'stage',
    'stage-wide',
    'stage-night',
    'stage-night-wide'
  )

  test.each([
    [false, true, 'stage-wide'],
    [false, false, 'stage'],
    [true, true, 'stage-night-wide'],
    [true, false, 'stage-night'],
  ])(
    'with the whole set, night %s and a wide screen %s open on %s',
    (night, wide, expected) => {
      expect(stageArtName(full, night, wide)).toBe(expected)
    }
  )

  test('a night without a painting for the screen shape keeps the moonlit painting of the other shape', () => {
    expect(
      stageArtName(manifestOf('stage', 'stage-wide', 'stage-night'), true, true)
    ).toBe('stage-night')
  })

  test('a night without moonlit paintings falls back to the day painting for the screen shape', () => {
    expect(stageArtName(manifestOf('stage', 'stage-wide'), true, true)).toBe(
      'stage-wide'
    )
  })

  test('without a manifest the opening asks for the upright day painting', () => {
    expect(stageArtName(undefined, true, true)).toBe('stage')
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
