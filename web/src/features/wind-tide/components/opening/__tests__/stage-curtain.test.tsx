/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { artManifestQueryOptions } from '../../../api'
import type { ArtEntry, ArtManifest } from '../../../lib/art'
import { StageCurtain } from '../stage-curtain'

const half: ArtEntry = {
  width: 2880,
  height: 2788,
  sizes: [480, 960, 1440, 1920],
  depth: false,
}
const pelmet: ArtEntry = {
  width: 2880,
  height: 212,
  sizes: [1080, 1600, 2160],
  depth: false,
}
const paintedSet: ArtManifest = {
  'curtain-left': half,
  'curtain-right': half,
  'curtain-valance': pelmet,
}

function renderCurtain(manifest: ArtManifest) {
  const client = new QueryClient()
  client.setQueryData(artManifestQueryOptions.queryKey, manifest)
  const view = render(
    <QueryClientProvider client={client}>
      <StageCurtain open={false} lift={false} />
    </QueryClientProvider>
  )
  return {
    panel: (side: 'left' | 'right') =>
      view.container.querySelector(`.wt-curtain-panel[data-side='${side}']`),
    drawnPelmet: () => view.container.querySelector('.wt-valance'),
    images: () => [...view.container.querySelectorAll('img')],
  }
}

describe('stage curtain', () => {
  test('without the painted curtain in the art set the drawn curtain stays', () => {
    const curtain = renderCurtain({})

    expect(curtain.images()).toHaveLength(0)
    expect(curtain.panel('left')).not.toHaveAttribute('data-painted')
    expect(curtain.drawnPelmet()).not.toHaveAttribute('data-painted')
  })

  test('a painted curtain missing a part is not used, so no half-painted stage shows', () => {
    const curtain = renderCurtain({
      'curtain-left': half,
      'curtain-right': half,
    })

    expect(curtain.images()).toHaveLength(0)
  })

  test('the painted halves and pelmet load over the drawn curtain', () => {
    const curtain = renderCurtain(paintedSet)

    expect(curtain.images().map((image) => image.getAttribute('src'))).toEqual([
      expect.stringContaining('/art/curtain-left-'),
      expect.stringContaining('/art/curtain-right-'),
      expect.stringContaining('/art/curtain-valance-'),
    ])
    expect(curtain.panel('left')).not.toHaveAttribute('data-painted')
  })

  test('each painted part replaces its drawn part once it has loaded', () => {
    const curtain = renderCurtain(paintedSet)
    const [left, , valance] = curtain.images()

    fireEvent.load(left)
    fireEvent.load(valance)

    expect(curtain.panel('left')).toHaveAttribute('data-painted', 'true')
    expect(curtain.panel('right')).not.toHaveAttribute('data-painted')
    expect(curtain.drawnPelmet()).toHaveAttribute('data-painted', 'true')
    expect(valance).toHaveAttribute('data-shown', 'true')
  })
})
