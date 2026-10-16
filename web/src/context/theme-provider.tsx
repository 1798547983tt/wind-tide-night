/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react'

import { isBeijingNight, msUntilDayNightSwitch } from '@/lib/beijing-time'
import {
  readThemePreference,
  THEME_STORAGE_KEYS,
  writeThemePreference,
} from '@/lib/theme-storage'

type Theme = 'dark' | 'light' | 'system'
type ResolvedTheme = Exclude<Theme, 'system'>

const DEFAULT_THEME = 'system'
const THEMES = new Set<Theme>(['dark', 'light', 'system'])

type ThemeProviderProps = {
  children: React.ReactNode
  defaultTheme?: Theme
  storageKey?: string
}

type ThemeProviderState = {
  defaultTheme: Theme
  resolvedTheme: ResolvedTheme
  theme: Theme
  setTheme: (theme: Theme) => void
  resetTheme: () => void
}

const initialState: ThemeProviderState = {
  defaultTheme: DEFAULT_THEME,
  resolvedTheme: 'light',
  theme: DEFAULT_THEME,
  setTheme: () => null,
  resetTheme: () => null,
}

const ThemeContext = createContext<ThemeProviderState>(initialState)

// wind-tide: "system" is the automatic day-night mode. Night (the dark theme)
// runs from 18:00 to 06:00 Beijing time, whatever the device's own settings.
function getSystemTheme(): ResolvedTheme {
  return isBeijingNight() ? 'dark' : 'light'
}

// wind-tide: the browser bar takes the page background of each theme.
const BROWSER_BAR_COLORS: Record<ResolvedTheme, string> = {
  light: '#fbf7f1',
  dark: '#0a0f24',
}

function resolveTheme(theme: Theme): ResolvedTheme {
  return theme === 'system' ? getSystemTheme() : theme
}

export function ThemeProvider({
  children,
  defaultTheme = DEFAULT_THEME,
  storageKey = THEME_STORAGE_KEYS.mode,
  ...props
}: ThemeProviderProps) {
  const [theme, _setTheme] = useState<Theme>(() =>
    readThemePreference(storageKey, THEMES, defaultTheme)
  )
  const [resolvedTheme, setResolvedTheme] = useState<ResolvedTheme>(() =>
    resolveTheme(theme)
  )

  useEffect(() => {
    const root = window.document.documentElement
    let switchTimer: number | undefined

    const applyTheme = () => {
      const nextResolvedTheme = theme === 'system' ? getSystemTheme() : theme
      root.classList.remove('light', 'dark')
      root.classList.add(nextResolvedTheme)
      setResolvedTheme(nextResolvedTheme)
      document
        .querySelector("meta[name='theme-color']")
        ?.setAttribute('content', BROWSER_BAR_COLORS[nextResolvedTheme])

      // wind-tide: re-evaluate right after the next 06:00 / 18:00 boundary.
      window.clearTimeout(switchTimer)
      if (theme === 'system') {
        switchTimer = window.setTimeout(
          applyTheme,
          msUntilDayNightSwitch() + 1000
        )
      }
    }

    // Timers can stall while a device sleeps; check again on return.
    const onVisible = () => {
      if (document.visibilityState === 'visible') applyTheme()
    }

    applyTheme()
    document.addEventListener('visibilitychange', onVisible)

    return () => {
      window.clearTimeout(switchTimer)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [theme])

  const setTheme = useCallback(
    (theme: Theme) => {
      writeThemePreference(storageKey, theme)
      _setTheme(theme)
    },
    [storageKey]
  )

  const resetTheme = useCallback(() => {
    writeThemePreference(storageKey, null)
    _setTheme(defaultTheme)
  }, [defaultTheme, storageKey])

  const contextValue = useMemo(
    () => ({
      defaultTheme,
      resolvedTheme,
      resetTheme,
      theme,
      setTheme,
    }),
    [defaultTheme, resolvedTheme, resetTheme, theme, setTheme]
  )

  return (
    <ThemeContext value={contextValue} {...props}>
      {children}
    </ThemeContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useTheme = () => {
  const context = useContext(ThemeContext)

  if (!context) throw new Error('useTheme must be used within a ThemeProvider')

  return context
}
