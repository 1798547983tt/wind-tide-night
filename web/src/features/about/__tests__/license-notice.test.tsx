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
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'

import { About } from '../index'

function openAbout(content: string | Promise<string>, entry = '/about') {
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/about') {
      return { data: { success: true, data: await content } }
    }
    if (url === '/api/status') return { data: { success: true, data: {} } }
    // The layout's notice and anything else the page asks for is empty.
    return { data: { success: true, data: '' } }
  })
  const root = createRootRoute()
  const about = createRoute({
    getParentRoute: () => root,
    path: 'about',
    component: About,
  })
  const router = createRouter({
    routeTree: root.addChildren([about]),
    history: createMemoryHistory({ initialEntries: [entry] }),
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
  return router
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('license notice on the about page', () => {
  test('a site without about content still states the license and both sources', async () => {
    openAbout('')

    const notice = await screen.findByRole('region', {
      name: 'Open-source license',
    })
    expect(notice).toHaveTextContent(
      'Frontend design and development by New API contributors.'
    )
    expect(
      screen.getByRole('link', { name: 'github.com/QuantumNous/new-api' })
    ).toHaveAttribute('href', 'https://github.com/QuantumNous/new-api')
    expect(
      screen.getByRole('link', {
        name: 'github.com/1798547983tt/wind-tide-night/tree/wind-tide',
      })
    ).toHaveAttribute(
      'href',
      'https://github.com/1798547983tt/wind-tide-night/tree/wind-tide'
    )
    expect(notice).toHaveTextContent(/runs a modified version of new-api/)
  })

  test("the owner's about content comes first, with the notice after it", async () => {
    openAbout('# Welcome to the night')

    const intro = await screen.findByRole('heading', {
      name: 'Welcome to the night',
    })
    const notice = screen.getByRole('region', { name: 'Open-source license' })
    expect(
      intro.compareDocumentPosition(notice) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
  })

  test('arriving from the footer link brings the notice into view once loaded', async () => {
    const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')
    let answer: (content: string) => void = () => undefined
    const content = new Promise<string>((resolve) => {
      answer = resolve
    })

    const router = openAbout(content, '/about#license')
    // The router has finished with the anchor while the page still loads.
    await waitFor(() => expect(router.state.status).toBe('idle'))
    await waitFor(() =>
      expect(document.querySelector('[data-slot="skeleton"]')).not.toBeNull()
    )
    answer('# Welcome to the night')

    const notice = await screen.findByRole('region', {
      name: 'Open-source license',
    })
    await waitFor(() => expect(scrollIntoView.mock.contexts).toContain(notice))
  })
})
