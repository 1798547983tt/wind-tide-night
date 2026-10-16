/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import type { CSSProperties } from 'react'

import { cn } from '@/lib/utils'

import { SealStamp } from '../seal-stamp'

// Calligraphy on the seal and on the print it leaves, read as a picture: the
// controls around it say the same in words, so these are not translation keys.
const SEAL_FACE = '签到'
const PRINT_FACE = '已签'

// Cash coins thrown up from a fresh print: offsets in rem, spin in degrees
// and a start delay in milliseconds, uneven on purpose.
const COINS = [
  { dx: -4.6, dy: -3.6, spin: -260, delay: 0 },
  { dx: -3.1, dy: -5.4, spin: 200, delay: 40 },
  { dx: -1.1, dy: -6.2, spin: -180, delay: 90 },
  { dx: 1.2, dy: -6, spin: 240, delay: 20 },
  { dx: 3.2, dy: -5.1, spin: -220, delay: 70 },
  { dx: 4.8, dy: -3.3, spin: 300, delay: 110 },
  { dx: -4, dy: -1.4, spin: 160, delay: 140 },
  { dx: 4.1, dy: -1.2, spin: -140, delay: 160 },
] as const

type StampSealProps = {
  /** Today's check-in is done, so the print lies where the seal hovered. */
  stamped: boolean
  /** The print was made just now: it lands with spreading ink and coins. */
  fresh?: boolean
  /** The seal is down on the paper while the check-in is on its way. */
  pressing?: boolean
  /** The reward the coins bring, already formatted, such as "$9,999". */
  reward: string
  className?: string
}

/**
 * A name seal hovering over a slot of paper. Checking in presses it down and
 * leaves a vermilion print; a fresh print throws up a handful of cash coins.
 * Pure decoration: whatever holds it names the action and announces the
 * result, and reduced motion keeps only the still print.
 */
export function StampSeal(props: StampSealProps) {
  return (
    <div
      aria-hidden='true'
      className={cn('relative size-32 sm:size-36', props.className)}
    >
      <div className='border-primary/35 absolute inset-[13%] rounded-2xl border-2 border-dashed' />

      {props.stamped ? (
        <>
          {props.fresh && (
            <span className='wt-ink-ring border-wt-crimson/50 absolute inset-[15%] rounded-2xl border-4' />
          )}
          <div className='absolute inset-[13%]'>
            <SealStamp
              text={PRINT_FACE}
              className={cn(
                'size-full rotate-[-7deg]',
                props.fresh && 'wt-imprint'
              )}
            />
          </div>
          {props.fresh &&
            COINS.map((coin) => (
              <svg
                key={`${coin.dx},${coin.dy}`}
                viewBox='0 0 20 20'
                className='wt-coin absolute top-1/2 left-1/2 -mt-2.5 -ml-2.5 size-5'
                style={
                  {
                    '--wt-dx': `${coin.dx}rem`,
                    '--wt-dy': `${coin.dy}rem`,
                    '--wt-spin': `${coin.spin}deg`,
                    '--wt-delay': `${coin.delay}ms`,
                  } as CSSProperties
                }
              >
                {/* A round coin with a square hole, gold with a darker rim. */}
                <path
                  fillRule='evenodd'
                  d='M10 0a10 10 0 1 1 0 20a10 10 0 1 1 0-20ZM7.7 7.7v4.6h4.6V7.7Z'
                  fill='var(--wt-sun)'
                />
                <path
                  fillRule='evenodd'
                  d='M10 2.4a7.6 7.6 0 1 1 0 15.2a7.6 7.6 0 1 1 0-15.2ZM6.7 6.7v6.6h6.6V6.7Z'
                  fill='none'
                  stroke='#a8741c'
                  strokeWidth='0.8'
                  opacity='0.75'
                />
              </svg>
            ))}
          {props.fresh && (
            <span className='wt-reward-rise font-display text-wt-crimson absolute inset-x-0 -top-3 text-center text-2xl font-semibold whitespace-nowrap italic'>
              +{props.reward}
            </span>
          )}
        </>
      ) : (
        <>
          {/* The seal's shadow on the paper, smaller as it comes down. */}
          <span
            className={cn(
              'absolute inset-x-[24%] bottom-[9%] h-3 rounded-[50%] bg-black/25 blur-md transition-[scale,opacity] duration-300 dark:bg-black/60',
              props.pressing && 'scale-75 opacity-60'
            )}
          />
          <div
            className={cn(
              'absolute inset-[13%] transition-[translate,scale] duration-300 ease-out',
              props.pressing ? 'scale-95' : '-translate-y-3'
            )}
          >
            <SealStamp
              text={SEAL_FACE}
              className={cn(
                'size-full drop-shadow-[0_6px_10px_rgb(120_20_40/0.25)]',
                !props.pressing && 'wt-breathe'
              )}
            />
          </div>
        </>
      )}
    </div>
  )
}
