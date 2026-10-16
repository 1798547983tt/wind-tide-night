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

import { ThemeProvider } from '@/context/theme-provider'

import { artManifestQueryOptions } from '../../../api'
import { LandingHero } from '../landing-hero'
import { ThreeActs } from '../three-acts'

// How much a check-in brings is a surprise kept for members, so the home
// page promises balance without naming the amount.
const AMOUNT = /9[,. ]?999/

function renderHome(page: ReactNode) {
  const client = new QueryClient()
  client.setQueryData(artManifestQueryOptions.queryKey, {})
  const root = createRootRoute({ component: () => page })
  const router = createRouter({
    routeTree: root,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <ThemeProvider>
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </ThemeProvider>
  )
}

beforeEach(() => {
  // jsdom has no IntersectionObserver; the acts never scroll into view.
  vi.stubGlobal(
    'IntersectionObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
})

afterEach(() => {
  localStorage.clear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('check-in promise on the home page', () => {
  test('the hero promises balance for checking in without naming the amount', async () => {
    renderHome(<LandingHero isAuthenticated={false} />)

    expect(
      await screen.findByText(
        'A free AI API relay open only to invited members. Check in every day for bonus balance.'
      )
    ).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent(AMOUNT)
  })

  test('the check-in act promises balance without naming the amount', async () => {
    renderHome(<ThreeActs />)

    expect(
      await screen.findByText(
        'Check in once every check-in day for bonus balance. Your balance has no upper limit.'
      )
    ).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent(AMOUNT)
  })
})
