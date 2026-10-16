/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import { artManifestQueryOptions } from '../../../api'
import type { ArtEntry, ArtManifest } from '../../../lib/art'
import { MascotFigure, type MascotMood } from '../mascot-figure'

const picture: ArtEntry = {
  width: 760,
  height: 788,
  sizes: [160, 320],
  depth: false,
}

function renderMascot(manifest: ArtManifest, mood?: MascotMood) {
  const client = new QueryClient()
  client.setQueryData(artManifestQueryOptions.queryKey, manifest)
  return render(
    <QueryClientProvider client={client}>
      <MascotFigure mood={mood} line='Hello~' sizes='112px' />
    </QueryClientProvider>
  )
}

describe('mascot figure', () => {
  test('an expression the deployment has not uploaded falls back to the teddy-bear hug', () => {
    const { container } = renderMascot({ 'mascot-base': picture }, 'think')

    expect(container.querySelector('img')).toHaveAttribute(
      'src',
      expect.stringContaining('/mascot-base-')
    )
  })

  test('an uploaded expression shows in place of the hug', () => {
    const { container } = renderMascot(
      { 'mascot-base': picture, 'mascot-think': picture },
      'think'
    )

    expect(container.querySelector('img')).toHaveAttribute(
      'src',
      expect.stringContaining('/mascot-think-')
    )
  })

  test('the speech bubble’s tail is outlined in the bubble’s border colour, not the text colour', () => {
    renderMascot({ 'mascot-base': picture })

    expect(screen.getByText('Hello~')).toHaveClass(
      'border',
      'after:border-r',
      'after:border-border'
    )
  })
})
