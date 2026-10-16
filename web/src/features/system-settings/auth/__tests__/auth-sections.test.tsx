/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import { createInstance, type i18n } from 'i18next'
import { useState, type ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { AuthSettings } from '../index'
import { getAuthSectionNavItems } from '../section-registry'

let testI18n: i18n

function Fixture(props: { children: ReactNode }) {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  const [client] = useState(
    () => new QueryClient({ defaultOptions: { queries: { retry: false } } })
  )
  return (
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={client}>
        <div ref={setContainer} />
        <SettingsPageProvider actionsContainer={container}>
          {props.children}
        </SettingsPageProvider>
      </QueryClientProvider>
    </I18nextProvider>
  )
}

function renderAuthSettingsAt(path: string) {
  const root = createRootRoute()
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
  })
  const route = createRoute({
    getParentRoute: () => authenticated,
    path: 'system-settings/auth/$section',
    component: AuthSettings,
  })
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren([route])]),
    history: createMemoryHistory({ initialEntries: [path] }),
  })
  render(
    <Fixture>
      <RouterProvider router={router} />
    </Fixture>
  )
}

beforeEach(async () => {
  testI18n = createInstance()
  await testI18n.init({
    lng: 'en',
    fallbackLng: 'en',
    resources: { en: { translation: {} } },
    interpolation: { escapeValue: false },
  })
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  vi.spyOn(api, 'get').mockImplementation(async (url) => ({
    data: {
      success: true,
      data:
        url === '/api/option/'
          ? [{ key: 'discord.client_id', value: 'discord-client' }]
          : {},
    },
  }))
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('authentication settings sections', () => {
  it('lists Discord as the only sign-in settings section', () => {
    expect(getAuthSectionNavItems(testI18n.t)).toEqual([
      { title: 'Discord', url: '/system-settings/auth/discord' },
    ])
  })

  it('opens the Discord settings from an old link to another sign-in section', async () => {
    renderAuthSettingsAt('/system-settings/auth/passkey')

    expect(
      await screen.findByRole('textbox', { name: 'Client ID' })
    ).toHaveValue('discord-client')
    expect(
      screen.queryByRole('textbox', { name: 'Primary Passkey domain' })
    ).toBeNull()
  })
})
