/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useId } from 'react'

import { cn } from '@/lib/utils'

type BrandMarkProps = {
  className?: string
}

/** The site mark: a crescent moon over a breaking wave. */
export function BrandMark(props: BrandMarkProps) {
  const maskId = useId()

  return (
    <svg
      aria-hidden='true'
      viewBox='0 0 32 32'
      className={cn('size-7', props.className)}
    >
      <defs>
        <mask id={maskId}>
          <rect width='32' height='32' fill='#fff' />
          <circle cx='20.6' cy='12.2' r='9.6' fill='#000' />
        </mask>
      </defs>
      <circle
        cx='15.5'
        cy='15.5'
        r='12'
        fill='var(--wt-crimson)'
        mask={`url(#${maskId})`}
      />
      <path
        d='M4.5 22.5c3-2.4 5.6-2.6 8 0s5.4 2.4 8 0 5.2-2.6 7.5-.4'
        fill='none'
        stroke='var(--wt-tide)'
        strokeWidth='2.2'
        strokeLinecap='round'
      />
      <circle cx='24.5' cy='7' r='1.3' fill='var(--wt-sun)' />
    </svg>
  )
}
