/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useId } from 'react'

// Same curve as the .wt-tide-mask edge in wind-tide.css, moved from y=150 of
// 300 to y=10 of 100 so the crest rides exactly on the edge of the mask.
const WAVE =
  'M1200 10C1150 16 1125 16 1075 10S1000 4 950 10 875 16 825 10 750 4 700 10 625 16 575 10 500 4 450 10 375 16 325 10 250 4 200 10 125 16 75 10 25 4 0 10'

type TideFrontProps = {
  active: boolean
}

/** The foam crest and wash of water that sweep the opening away. */
export function TideFront(props: TideFrontProps) {
  const gradientId = useId()

  return (
    <div
      aria-hidden='true'
      className='wt-tide-front pointer-events-none absolute top-0 left-0 h-dvh w-[200%]'
      data-tide={props.active ? 'out' : undefined}
    >
      <svg
        viewBox='0 0 1200 100'
        preserveAspectRatio='none'
        className='size-full overflow-visible'
      >
        <defs>
          <linearGradient id={gradientId} x1='0' y1='0' x2='0' y2='1'>
            <stop offset='0' stopColor='var(--wt-tide)' stopOpacity='0.6' />
            <stop offset='0.12' stopColor='var(--wt-tide)' stopOpacity='0.22' />
            <stop offset='0.45' stopColor='var(--wt-tide)' stopOpacity='0' />
          </linearGradient>
        </defs>
        <path d={`${WAVE}V100H1200Z`} fill={`url(#${gradientId})`} />
        <path
          d={WAVE}
          fill='none'
          stroke='#ffffff'
          strokeWidth={3}
          vectorEffect='non-scaling-stroke'
          className='drop-shadow-[0_0_10px_rgba(255,255,255,0.9)]'
        />
      </svg>
    </div>
  )
}
