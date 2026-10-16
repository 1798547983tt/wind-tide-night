/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import { SidebarProvider } from '@/components/ui/sidebar'
import { DirectionProvider } from '@/context/direction-provider'
import { LayoutProvider } from '@/context/layout-provider'
import { SearchProvider } from '@/context/search-provider'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { AppHeader } from '../components/app-header'
import { PublicHeader } from '../components/public-header'

const fetchMock = vi.fn<typeof fetch>()

function renderHeader(header: ReactNode) {
  const root = createRootRoute({ component: () => header })
  const router = createRouter({
    routeTree: root,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  vi.stubGlobal('fetch', fetchMock)
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/status') return { data: { success: true, data: {} } }
    // Notices, notifications and the rest have nothing to say.
    return { data: { success: true, data: '' } }
  })
  useAuthStore
    .getState()
    .auth.setUser({ id: 1, username: 'owner', role: ROLE.SUPER_ADMIN })
})

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  fetchMock.mockReset()
  useAuthStore.getState().auth.reset()
})

describe('headers of a fork that ships its own image', () => {
  test.each([
    [
      'console header',
      <DirectionProvider key='console'>
        <LayoutProvider>
          <SearchProvider>
            <SidebarProvider>
              <AppHeader />
            </SidebarProvider>
          </SearchProvider>
        </LayoutProvider>
      </DirectionProvider>,
    ],
    ['public header', <PublicHeader key='public' />],
  ])(
    'the owner sees no upstream update check in the %s',
    async (_name, header) => {
      renderHeader(header)

      expect(await screen.findByRole('banner')).toBeInTheDocument()
      expect(
        screen.queryByRole('button', {
          name: /System updates|Check for updates|Update available/,
        })
      ).toBeNull()
      expect(fetchMock).not.toHaveBeenCalledWith(
        expect.stringContaining('api.github.com'),
        expect.anything()
      )
    }
  )
})
