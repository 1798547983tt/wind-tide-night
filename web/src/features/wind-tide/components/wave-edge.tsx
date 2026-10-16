/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { cn } from '@/lib/utils'

// Twelve 200-unit periods over 2400 units, so shifting by half is seamless.
// A horizontal edge fills below its waves; a vertical one fills to their right.
function wavePath(amplitude: number, baseline: number, vertical: boolean) {
  let d = vertical ? `M${baseline} 0` : `M0 ${baseline}`
  for (let at = 0; at < 2400; at += 200) {
    d += vertical
      ? ` Q${baseline - amplitude} ${at + 50} ${baseline} ${at + 100} T${baseline} ${at + 200}`
      : ` Q${at + 50} ${baseline - amplitude} ${at + 100} ${baseline} T${at + 200} ${baseline}`
  }
  return vertical ? `${d} H80 V0 Z` : `${d} V80 H0 Z`
}

const WAVES = {
  horizontal: { back: wavePath(9, 34, false), front: wavePath(7, 46, false) },
  vertical: { back: wavePath(9, 34, true), front: wavePath(7, 46, true) },
}

type WaveEdgeProps = {
  /** Vertical edges run down the right side of what they cover. */
  orientation?: 'horizontal' | 'vertical'
  className?: string
}

/**
 * Two drifting wave layers in the page background colour, laid over the
 * edge of a section so the next one rises out of it like the tide.
 */
export function WaveEdge(props: WaveEdgeProps) {
  if (props.orientation === 'vertical') {
    return (
      <div
        aria-hidden='true'
        className={cn(
          'pointer-events-none relative h-full w-16 overflow-hidden sm:w-20',
          props.className
        )}
      >
        <svg
          viewBox='0 0 80 2400'
          preserveAspectRatio='none'
          className='wt-wave-drift-y-reverse absolute inset-0 h-[200%] w-full opacity-50'
        >
          <path d={WAVES.vertical.back} fill='var(--background)' />
        </svg>
        <svg
          viewBox='0 0 80 2400'
          preserveAspectRatio='none'
          className='wt-wave-drift-y absolute inset-0 h-[200%] w-full'
        >
          <path d={WAVES.vertical.front} fill='var(--background)' />
        </svg>
      </div>
    )
  }

  return (
    <div
      aria-hidden='true'
      className={cn(
        'pointer-events-none relative h-16 w-full overflow-hidden sm:h-20',
        props.className
      )}
    >
      <svg
        viewBox='0 0 2400 80'
        preserveAspectRatio='none'
        className='wt-wave-drift-reverse absolute inset-0 h-full w-[200%] opacity-50'
      >
        <path d={WAVES.horizontal.back} fill='var(--background)' />
      </svg>
      <svg
        viewBox='0 0 2400 80'
        preserveAspectRatio='none'
        className='wt-wave-drift absolute inset-0 h-full w-[200%]'
      >
        <path d={WAVES.horizontal.front} fill='var(--background)' />
      </svg>
    </div>
  )
}
