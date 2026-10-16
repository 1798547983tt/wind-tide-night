/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import type { CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

import type { LeaderboardItem } from '../../types'
import { MemberAvatar } from '../member-avatar'
import { WaveEdge } from '../wave-edge'

// The column each of the first three listed stands in: the first in the
// middle, the second on its left and the third on its right. The list keeps
// rank order for screen readers.
const SPOTS = ['col-start-2', 'col-start-1', 'col-start-3'] as const

// They rise in reverse, so first place comes up last.
const RISE_DELAYS = [380, 190, 0] as const

// Heights and colours go by rank, so members tied for a place stand level.
const TIERS = {
  1: {
    ring: 'ring-wt-sun',
    avatar: 'size-20 sm:size-28',
    block: 'h-40 sm:h-52',
    band: 'from-wt-sun/40',
    numeral: 'text-wt-sun',
  },
  2: {
    ring: 'ring-wt-silver',
    avatar: 'size-16 sm:size-22',
    block: 'h-32 sm:h-40',
    band: 'from-wt-silver/40',
    numeral: 'text-wt-silver',
  },
  3: {
    ring: 'ring-wt-bronze',
    avatar: 'size-16 sm:size-22',
    block: 'h-28 sm:h-32',
    band: 'from-wt-bronze/40',
    numeral: 'text-wt-bronze',
  },
} as const

type PodiumProps = {
  /** The first three listed, in rank order. */
  items: LeaderboardItem[]
  /** The signed-in member, marked where they stand. */
  myId: number
  formatValue: (value: number) => string
}

/**
 * The top three of a leaderboard on a podium that rises out of the tide:
 * avatars ringed in sun gold, silver and bronze, a crescent moon crowning
 * first place, and a seal with each rank.
 */
export function Podium(props: PodiumProps) {
  const { t } = useTranslation()

  return (
    <div className='relative'>
      <ol
        aria-label={t('Top three')}
        className='grid grid-cols-3 items-end gap-2 sm:gap-6'
      >
        {props.items.map((item, index) => {
          // The first three listed never rank below third.
          const tier = TIERS[Math.min(item.rank, 3) as 1 | 2 | 3]
          return (
            <li
              key={item.user_id}
              className={cn('row-start-1 min-w-0', SPOTS[index])}
            >
              <div
                className='wt-podium-rise flex flex-col items-center'
                style={
                  { '--wt-delay': `${RISE_DELAYS[index]}ms` } as CSSProperties
                }
              >
                <span className='sr-only'>
                  {t('No. {{rank}}', { rank: item.rank })}
                </span>
                <div className='relative'>
                  {item.rank === 1 && (
                    <span
                      aria-hidden='true'
                      className='absolute -top-8 left-1/2 size-7 -translate-x-1/2 rotate-[-28deg] rounded-full shadow-[inset_7px_-3px_0_0_var(--wt-sun)] drop-shadow-[0_0_6px_rgb(242_196_107/0.7)] sm:-top-11 sm:size-9 sm:shadow-[inset_9px_-4px_0_0_var(--wt-sun)]'
                    />
                  )}
                  <MemberAvatar
                    name={item.display_name}
                    avatarUrl={item.avatar_url}
                    className={cn(
                      'ring-offset-background ring-4 ring-offset-2',
                      tier.ring,
                      tier.avatar
                    )}
                    fallbackClassName='text-xl sm:text-3xl'
                  />
                  {/* The ring in the page colour keeps the seal apart from
                      an avatar of the same red. */}
                  <span
                    aria-hidden='true'
                    className='bg-wt-crimson font-display ring-background absolute -right-1.5 -bottom-1.5 flex size-7 rotate-[-8deg] items-center justify-center rounded-md text-base font-semibold text-[#fff6e6] [font-variant-numeric:lining-nums] shadow-[inset_0_0_0_1.5px_rgb(255_246_230/0.4)] ring-2 sm:size-8 sm:text-lg'
                  >
                    {item.rank}
                  </span>
                </div>

                <p className='mt-4 flex max-w-full min-w-0 items-center gap-1.5 px-1'>
                  <span className='font-mincho truncate text-sm font-semibold sm:text-base'>
                    {item.display_name}
                  </span>
                  {item.user_id === props.myId && (
                    <Badge className='px-1.5 text-[10px]'>{t('Me')}</Badge>
                  )}
                </p>
                <p className='font-display text-lg leading-tight font-semibold whitespace-nowrap italic [font-variant-numeric:lining-nums_tabular-nums] sm:text-2xl'>
                  {props.formatValue(item.value)}
                </p>

                <div
                  aria-hidden='true'
                  className={cn(
                    'relative mt-4 w-full overflow-hidden rounded-t-2xl border border-b-0 bg-linear-to-b to-transparent',
                    tier.band,
                    tier.block
                  )}
                >
                  <span
                    className={cn(
                      'font-display absolute inset-x-0 top-2 text-center text-6xl leading-none font-semibold italic opacity-70 [font-variant-numeric:lining-nums] sm:text-8xl',
                      tier.numeral
                    )}
                  >
                    {item.rank}
                  </span>
                </div>
              </div>
            </li>
          )
        })}
      </ol>
      <WaveEdge className='absolute inset-x-0 bottom-0' />
    </div>
  )
}
