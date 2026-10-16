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
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

import { artManifestQueryOptions } from '../../../api'
import { BRAND } from '../../../constants'
import type { ArtManifest } from '../../../lib/art'
import { AuthStage, type AuthPainting } from '../auth-stage'

const entry = { width: 2880, height: 4070, sizes: [720, 1080], depth: false }

/** A null manifest stands for a deployment whose /art/ cannot be fetched. */
function openStage(manifest: ArtManifest | null, painting?: AuthPainting) {
  const client = new QueryClient()
  if (manifest) {
    client.setQueryData(artManifestQueryOptions.queryKey, manifest)
  } else {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response('Not Found', { status: 404 })
    )
  }
  const root = createRootRoute({
    component: () => (
      <AuthStage painting={painting}>
        <p>page content</p>
      </AuthStage>
    ),
  })
  const router = createRouter({
    routeTree: root,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
  return client
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('sign-in stage', () => {
  test('shows the welcome painting beside the page by default', async () => {
    openStage({ 'auth-welcome': entry, 'hero-day': entry })

    const painting = await screen.findByRole('img', {
      name: 'Cecilia crouching with a flower on her blouse, smiling up at you',
    })
    expect(painting.getAttribute('src')).toContain('/art/auth-welcome-')
    expect(screen.getByText('page content')).toBeVisible()
  })

  test('a page can pick the tea painting instead', async () => {
    openStage({ 'auth-tea': entry, 'hero-day': entry }, 'tea')

    const painting = await screen.findByRole('img', {
      name: 'Cecilia holding a teacup and smiling up at the light',
    })
    expect(painting.getAttribute('src')).toContain('/art/auth-tea-')
  })

  test('a deployment without the sign-in paintings falls back to the landing painting', async () => {
    openStage({ 'hero-day': entry })

    const painting = await screen.findByRole('img', {
      name: 'A girl in a straw hat looking out over the summer sea',
    })
    expect(painting.getAttribute('src')).toContain('/art/hero-day-')
  })

  test('the calligraphy on the right of the welcome painting shows on every screen', async () => {
    openStage({ 'auth-welcome': entry })

    const title = await screen.findByLabelText(BRAND.name)
    expect(title.closest('[aria-hidden="true"]')).not.toHaveClass('hidden')
  })

  test('the calligraphy on the left of the tea painting waits for wide screens, clear of the home link', async () => {
    openStage({ 'auth-tea': entry }, 'tea')

    const title = await screen.findByLabelText(BRAND.name)
    expect(title.closest('[aria-hidden="true"]')).toHaveClass(
      'hidden',
      'lg:flex'
    )
  })

  test('without any artwork the stage keeps the page and draws no broken image', async () => {
    const client = openStage(null)

    await waitFor(() =>
      expect(
        client.getQueryState(artManifestQueryOptions.queryKey)?.status
      ).toBe('error')
    )
    expect(screen.getByText('page content')).toBeVisible()
    expect(screen.queryByRole('img')).toBeNull()
  })
})
