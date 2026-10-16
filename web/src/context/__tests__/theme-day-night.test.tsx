/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { act, cleanup, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { ThemeProvider } from '@/context/theme-provider'

const MODE_KEY = 'newapi:theme:v1:mode'

function renderAt(iso: string) {
  vi.useFakeTimers({ now: new Date(iso) })
  render(
    <ThemeProvider>
      <div />
    </ThemeProvider>
  )
}

beforeEach(() => {
  localStorage.clear()
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  localStorage.clear()
  document.documentElement.classList.remove('light', 'dark')
})

describe('automatic day and night by Beijing time', () => {
  test('the automatic mode at 20:00 in Beijing applies the night theme', () => {
    renderAt('2026-10-15T12:00:00Z')

    expect(document.documentElement).toHaveClass('dark')
  })

  test('the automatic mode at 10:00 in Beijing applies the day theme', () => {
    renderAt('2026-10-15T02:00:00Z')

    expect(document.documentElement).toHaveClass('light')
  })

  test('an open page turns to night once 18:00 passes in Beijing', () => {
    renderAt('2026-10-15T09:59:30Z')
    expect(document.documentElement).toHaveClass('light')

    act(() => {
      vi.advanceTimersByTime(32_000)
    })

    expect(document.documentElement).toHaveClass('dark')
  })

  test('a theme chosen by hand stays as chosen at night', () => {
    localStorage.setItem(MODE_KEY, 'light')

    renderAt('2026-10-15T14:00:00Z')

    expect(document.documentElement).toHaveClass('light')
  })
})

describe('browser bar color', () => {
  let meta: HTMLMetaElement

  beforeEach(() => {
    meta = document.createElement('meta')
    meta.name = 'theme-color'
    meta.content = '#ffffff'
    document.head.append(meta)
  })

  afterEach(() => {
    meta.remove()
  })

  test('the night theme gives the browser bar the night sky color', () => {
    renderAt('2026-10-15T12:00:00Z')

    expect(meta.content).toBe('#0a0f24')
  })

  test('the day theme gives the browser bar the paper color', () => {
    renderAt('2026-10-15T02:00:00Z')

    expect(meta.content).toBe('#fbf7f1')
  })
})
