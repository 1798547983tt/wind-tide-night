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
import { describe, expect, test } from 'vitest'

import { artManifestQueryOptions } from '@/features/wind-tide/api'
import type { ArtManifest } from '@/features/wind-tide/lib/art'

import { GeneralError } from '../general-error'
import { MaintenanceError } from '../maintenance-error'
import { NotFoundError } from '../not-found-error'

const withMascot: ArtManifest = {
  'mascot-base': { width: 760, height: 788, sizes: [160, 320], depth: false },
}

function openErrorPage(page: () => ReactNode, manifest = withMascot) {
  const client = new QueryClient()
  client.setQueryData(artManifestQueryOptions.queryKey, manifest)
  const router = createRouter({
    routeTree: createRootRoute({ component: page }),
    history: createMemoryHistory({ initialEntries: ['/missing'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

describe('the mascot on the error pages (wind-tide/DESIGN.md §8)', () => {
  test('a missing page has her say it drifted away, above the plain explanation', async () => {
    openErrorPage(() => <NotFoundError />)

    const line = await screen.findByText('This page drifted away on the tide…')
    const heading = screen.getByRole('heading', { name: '404' })
    expect(
      line.compareDocumentPosition(heading) & Node.DOCUMENT_POSITION_FOLLOWING
    ).toBeTruthy()
    expect(screen.getByText('Oops! Page Not Found!')).toBeVisible()
  })

  test('too many requests gets a line of its own', async () => {
    openErrorPage(() => <GeneralError error={{ response: { status: 429 } }} />)

    expect(
      await screen.findByText('Too fast! Let the wind catch its breath~')
    ).toBeVisible()
    expect(screen.getByText('Too many requests')).toBeVisible()
  })

  test('maintenance has her asleep with a line about the tide', async () => {
    openErrorPage(() => <MaintenanceError />)

    expect(
      await screen.findByText(
        'The tide is out for maintenance. Come back a little later~'
      )
    ).toBeVisible()
  })

  test('without mascot art the error page explains itself without her', async () => {
    openErrorPage(() => <NotFoundError />, {})

    expect(
      await screen.findByRole('heading', { name: '404' })
    ).toBeInTheDocument()
    expect(screen.queryByText('This page drifted away on the tide…')).toBeNull()
  })
})
