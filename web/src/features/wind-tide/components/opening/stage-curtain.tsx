/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { useState, type CSSProperties } from 'react'

import { cn } from '@/lib/utils'

import { artManifestQueryOptions } from '../../api'
import { coverWidth, pickArtUrl } from '../../lib/art'

type StageCurtainProps = {
  open: boolean
  /** Lifts the pelmet out of frame, for the camera push. */
  lift: boolean
  className?: string
}

type CurtainPart = 'left' | 'right' | 'valance'

// A panel's share of the screen's width, as .wt-curtain-panel sets it.
const PANEL_SHARE = 0.506

/**
 * Crimson velvet curtains that gather to the sides when opened. The painted
 * curtain from the art set fades in over the drawn one as each part loads;
 * without it the drawn curtain stays.
 */
export function StageCurtain(props: StageCurtainProps) {
  const manifest = useQuery(artManifestQueryOptions)
  const [loaded, setLoaded] = useState<Partial<Record<CurtainPart, boolean>>>(
    {}
  )
  const left = manifest.data?.['curtain-left']
  const right = manifest.data?.['curtain-right']
  const valance = manifest.data?.['curtain-valance']
  const painted = !!(left && right && valance)
  // A half fills its panel as `object-fit: cover` does; the pelmet spans the
  // painting both halves were cut from, so it is shown twice as wide.
  const halfWidth = coverWidth(left, PANEL_SHARE)
  const show = (part: CurtainPart) => () =>
    setLoaded((state) => ({ ...state, [part]: true }))

  return (
    <div
      aria-hidden='true'
      className={cn(
        'wt-curtain pointer-events-none absolute inset-0 overflow-hidden',
        props.className
      )}
      style={
        left
          ? ({
              '--wt-curtain-aspect': left.width / left.height,
            } as CSSProperties)
          : undefined
      }
    >
      <div
        className='wt-curtain-panel'
        data-side='left'
        data-open={props.open}
        data-painted={loaded.left || undefined}
      >
        {painted && (
          <img
            src={pickArtUrl('curtain-left', left, halfWidth)}
            alt=''
            decoding='async'
            className='wt-curtain-art'
            onLoad={show('left')}
          />
        )}
      </div>
      <div
        className='wt-curtain-panel'
        data-side='right'
        data-open={props.open}
        data-painted={loaded.right || undefined}
      >
        {painted && (
          <img
            src={pickArtUrl('curtain-right', right, halfWidth)}
            alt=''
            decoding='async'
            className='wt-curtain-art'
            onLoad={show('right')}
          />
        )}
      </div>
      <div
        className='wt-valance'
        data-lift={props.lift}
        data-painted={loaded.valance || undefined}
      />
      {painted && (
        <img
          src={pickArtUrl('curtain-valance', valance, halfWidth * 2)}
          alt=''
          decoding='async'
          className='wt-valance-art'
          data-lift={props.lift}
          data-shown={loaded.valance || undefined}
          onLoad={show('valance')}
        />
      )}
    </div>
  )
}
