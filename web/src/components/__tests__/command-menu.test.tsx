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
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  afterAll,
  afterEach,
  beforeAll,
  describe,
  expect,
  test,
  vi,
} from 'vitest'

import { SearchProvider } from '@/context/search-provider'
import { api } from '@/lib/api'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { useOwnerViewStore, type OwnerView } from '@/stores/owner-view-store'

const ALLOWLIST = 'Allowlist'

// jsdom has no Web Animations API; the menu's scroll area asks for it.
const getAnimationsDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'getAnimations'
)

beforeAll(() => {
  Object.defineProperty(HTMLElement.prototype, 'getAnimations', {
    configurable: true,
    value: () => [],
  })
})

afterAll(() => {
  if (getAnimationsDescriptor) {
    Object.defineProperty(
      HTMLElement.prototype,
      'getAnimations',
      getAnimationsDescriptor
    )
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, 'getAnimations')
  }
})

/** Opens the console's command menu with Ctrl+K and returns its dialog. */
async function openCommandMenu(role: number, view: OwnerView) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  client.setQueryData(['status'], { SidebarModulesAdmin: '' })
  useAuthStore.getState().auth.setUser({ id: 1, username: 'someone', role })
  useOwnerViewStore.setState({ view })
  // The owner's pending entry count; nothing else is asked for.
  vi.spyOn(api, 'get').mockResolvedValue({
    data: { success: true, data: { count: 0 } },
  })
  const router = createRouter({
    routeTree: createRootRoute({
      component: () => (
        <SearchProvider>
          <span>console ready</span>
        </SearchProvider>
      ),
    }),
    history: createMemoryHistory({ initialEntries: ['/dashboard/overview'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  await screen.findByText('console ready')
  await userEvent.keyboard('{Control>}k{/Control}')
  return screen.findByRole('dialog')
}

afterEach(() => {
  vi.restoreAllMocks()
  useAuthStore.getState().auth.reset()
  useOwnerViewStore.setState({ view: 'admin' })
})

describe('command menu', () => {
  test.each<[string, number, OwnerView]>([
    ['a member', ROLE.USER, 'admin'],
    ['the owner in member view', ROLE.SUPER_ADMIN, 'member'],
  ])(
    '%s searching the console finds no administration pages',
    async (_who, role, view) => {
      const dialog = await openCommandMenu(role, view)

      expect(
        within(dialog).getByRole('option', { name: 'Playground' })
      ).toBeInTheDocument()
      expect(within(dialog).queryByRole('option', { name: 'Users' })).toBeNull()
      expect(
        within(dialog).queryByRole('option', { name: ALLOWLIST })
      ).toBeNull()
    }
  )

  test('the owner searching the console finds the allowlist page', async () => {
    const dialog = await openCommandMenu(ROLE.SUPER_ADMIN, 'admin')

    expect(
      within(dialog).getByRole('option', { name: ALLOWLIST })
    ).toBeInTheDocument()
  })
})
