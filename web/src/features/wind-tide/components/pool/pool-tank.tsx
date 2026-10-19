/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useReducedMotion } from 'motion/react'
import type { CSSProperties, ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent } from '@/components/ui/card'
import { cn } from '@/lib/utils'

import { WaveEdge } from '../wave-edge'

/** Tide blue for Antigravity, sun gold for the AI Studio pool. */
export type TankTone = 'tide' | 'sun'

// The bubbles are fixed, so a gauge looks the same on every render: each
// has its own place across the vessel and its own moment to set off.
const BUBBLES = [
  { left: '18%', size: 6, delay: 0, duration: 5.2 },
  { left: '44%', size: 4, delay: 1.3, duration: 4.1 },
  { left: '68%', size: 7, delay: 2.1, duration: 6 },
  { left: '30%', size: 3, delay: 3.4, duration: 3.6 },
  { left: '80%', size: 5, delay: 0.8, duration: 4.8 },
  { left: '56%', size: 4, delay: 2.9, duration: 5.5 },
] as const

type PoolTankProps = {
  tone: TankTone
  /** The member may only look: the tank fades and the overlay says why. */
  overlay?: ReactNode
  children: ReactNode
}

/**
 * The card a pool's gauges stand in (wind-tide/API.md §15). Its tone colours
 * every liquid inside it.
 */
export function PoolTank(props: PoolTankProps) {
  const dimmed = props.overlay !== undefined

  return (
    <Card
      data-tone={props.tone}
      className={cn(
        'wt-tank relative overflow-hidden',
        dimmed && 'wt-tank-dimmed'
      )}
    >
      <CardContent>{props.children}</CardContent>
      {props.overlay !== undefined && (
        <div className='bg-background/70 absolute inset-0 flex items-center justify-center p-6 backdrop-blur-[2px]'>
          {props.overlay}
        </div>
      )}
    </Card>
  )
}

type TankGaugeProps = {
  /** How full, 0 to 1. */
  level: number
  /** The gauge's name, set small above the figure; also names the region. */
  title: string
  /** The big figure beside the vessel. */
  figure: ReactNode
  /** The lines under the figure. */
  children?: ReactNode
  /** No reading yet: the vessel stands empty and still. */
  calm?: boolean
  /** A smaller vessel and figure, for several gauges in one tank. */
  compact?: boolean
}

/**
 * A glass vessel of liquid beside a big figure. The liquid rises to the
 * level with a wavy surface and bubbles drifting up; a member who prefers
 * reduced motion sees it still, at the level.
 */
export function TankGauge(props: TankGaugeProps) {
  const { t } = useTranslation()
  const reducedMotion = !!useReducedMotion()
  const percent = Math.round(Math.min(1, Math.max(0, props.level)) * 100)
  const still = props.calm || reducedMotion

  return (
    <section
      aria-label={props.title}
      className='flex items-stretch gap-5 sm:gap-7'
    >
      <div
        role='img'
        aria-label={t('{{percent}}% full', { percent })}
        className={cn(
          'wt-tank-vessel bg-muted/30 relative shrink-0 overflow-hidden rounded-full border',
          props.compact
            ? 'h-44 w-16 sm:h-52 sm:w-20'
            : 'h-56 w-20 sm:h-64 sm:w-24'
        )}
      >
        <div
          className='wt-tank-liquid absolute inset-x-0 bottom-0'
          style={{ height: `${percent}%` }}
        >
          {!props.calm && (
            <WaveEdge
              color='var(--wt-tank)'
              className='absolute inset-x-0 bottom-full h-5 sm:h-6'
            />
          )}
          <div className='absolute inset-0 bg-[var(--wt-tank)]' />
          {!still && (
            <div
              aria-hidden='true'
              className='absolute inset-0 overflow-hidden'
            >
              {BUBBLES.map((bubble) => (
                <span
                  key={bubble.left}
                  className='wt-tank-bubble'
                  style={
                    {
                      left: bubble.left,
                      width: bubble.size,
                      height: bubble.size,
                      '--wt-delay': `${bubble.delay}s`,
                      '--wt-duration': `${bubble.duration}s`,
                    } as CSSProperties
                  }
                />
              ))}
            </div>
          )}
        </div>
        <div
          aria-hidden='true'
          className='wt-tank-glass pointer-events-none absolute inset-0 rounded-full'
        />
      </div>
      <div className='flex min-w-0 flex-1 flex-col justify-center gap-2.5'>
        <p className='text-muted-foreground truncate text-xs font-medium tracking-[0.2em] uppercase'>
          {props.title}
        </p>
        <div
          className={cn(
            'wt-tank-figure font-display leading-none font-semibold italic [font-variant-numeric:lining-nums_tabular-nums]',
            props.compact ? 'text-4xl sm:text-5xl' : 'text-5xl sm:text-6xl'
          )}
        >
          {props.figure}
        </div>
        {props.children}
      </div>
    </section>
  )
}
