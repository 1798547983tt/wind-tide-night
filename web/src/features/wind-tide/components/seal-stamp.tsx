/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useId } from 'react'

import { cn } from '@/lib/utils'

type SealStampProps = {
  /** Two characters, read top to bottom. */
  text?: string
  /**
   * - crimson: a fresh vermilion stamp (the default).
   * - ink: a faded grey stamp, for something closed.
   * - slot: a dashed outline still waiting for its stamp.
   */
  tone?: 'crimson' | 'ink' | 'slot'
  className?: string
}

const TONES = {
  crimson: { fill: 'var(--wt-crimson)', ink: '#fff6e6', frame: 0.85 },
  ink: {
    fill: 'var(--muted-foreground)',
    ink: 'var(--background)',
    frame: 0.7,
  },
  slot: { fill: 'none', ink: 'currentColor', frame: 0 },
} as const

/** A name seal (落款印), slightly uneven like a real stamp. */
export function SealStamp(props: SealStampProps) {
  const filterId = useId()
  const [first = '风', second = '潮'] = [...(props.text ?? '风潮')]
  const tone = props.tone ?? 'crimson'
  const colors = TONES[tone]
  const slot = tone === 'slot'

  return (
    <svg
      aria-hidden='true'
      viewBox='0 0 64 64'
      className={cn('size-12 rotate-[-4deg]', props.className)}
    >
      <defs>
        <filter id={filterId} x='-10%' y='-10%' width='120%' height='120%'>
          <feTurbulence
            type='fractalNoise'
            baseFrequency='0.9'
            numOctaves='2'
            seed='7'
            result='noise'
          />
          <feDisplacementMap in='SourceGraphic' in2='noise' scale='2.4' />
        </filter>
      </defs>
      <g filter={`url(#${filterId})`}>
        <rect
          x='4'
          y='4'
          width='56'
          height='56'
          rx='7'
          fill={colors.fill}
          stroke={slot ? 'currentColor' : 'none'}
          strokeWidth={slot ? 2 : 0}
          strokeDasharray={slot ? '5 4' : undefined}
        />
        {!slot && (
          <rect
            x='8.5'
            y='8.5'
            width='47'
            height='47'
            rx='3.5'
            fill='none'
            stroke={colors.ink}
            strokeWidth='1.4'
            opacity={colors.frame}
          />
        )}
        <g opacity={slot ? 0.45 : 1}>
          <text
            x='32'
            y='30.5'
            textAnchor='middle'
            fontFamily='Zhi Mang Xing, serif'
            fontSize='23'
            fill={colors.ink}
          >
            {first}
          </text>
          <text
            x='32'
            y='52'
            textAnchor='middle'
            fontFamily='Zhi Mang Xing, serif'
            fontSize='23'
            fill={colors.ink}
          >
            {second}
          </text>
        </g>
      </g>
    </svg>
  )
}
