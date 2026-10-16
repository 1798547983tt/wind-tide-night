/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { ThemeQuickSwitcher } from '@/components/theme-quick-switcher'
import { ThemeSwitch } from '@/components/theme-switch'
import { ThemeProvider } from '@/context/theme-provider'

beforeEach(() => {
  localStorage.clear()
  // Midday in Beijing, when the automatic mode shows the day theme.
  vi.useFakeTimers({ toFake: ['Date'], now: new Date('2026-10-15T04:00:00Z') })
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  localStorage.clear()
  document.documentElement.classList.remove('light', 'dark')
})

describe('theme names (wind-tide/DESIGN.md §8)', () => {
  test('the header menu offers the day and night themes by name and applies the one chosen', async () => {
    const user = userEvent.setup()
    render(
      <ThemeProvider>
        <ThemeSwitch />
      </ThemeProvider>
    )

    await user.click(screen.getByRole('button', { name: 'Toggle theme' }))
    expect(
      screen.getAllByRole('menuitem').map((item) => item.textContent)
    ).toEqual([
      'Wind (day)',
      'Night of Tide (night)',
      'Day and night (Beijing time)',
    ])
    await user.click(
      screen.getByRole('menuitem', { name: 'Night of Tide (night)' })
    )

    expect(document.documentElement).toHaveClass('dark')
    expect(localStorage.getItem('newapi:theme:v1:mode')).toBe('dark')
  })

  test('the quick switcher names the same three choices, the automatic one chosen until a member picks', () => {
    render(
      <ThemeProvider>
        <ThemeQuickSwitcher />
      </ThemeProvider>
    )

    expect(
      screen.getByRole('radio', { name: 'Day and night (Beijing time)' })
    ).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByRole('radio', { name: 'Wind (day)' })).toHaveAttribute(
      'aria-checked',
      'false'
    )
    expect(
      screen.getByRole('radio', { name: 'Night of Tide (night)' })
    ).toHaveAttribute('aria-checked', 'false')
  })
})
