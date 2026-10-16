/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

import type { LeaderboardItem } from '../../types'
import { MemberAvatar } from '../member-avatar'

type RankingListProps = {
  /** Everyone listed below the podium, in rank order. */
  items: LeaderboardItem[]
  /** First place's value, which a full tide line stands for. */
  topValue: number
  /** The signed-in member, whose row is lit. */
  myId: number
  formatValue: (value: number) => string
}

/**
 * The members below the podium, one row each. A thin tide line under every
 * row shows how its value measures against first place.
 */
export function RankingList(props: RankingListProps) {
  const { t } = useTranslation()

  return (
    <ol
      aria-label={t('Other members')}
      className='bg-card divide-y overflow-hidden rounded-2xl border'
    >
      {props.items.map((item) => {
        const isMe = item.user_id === props.myId
        return (
          <li
            key={item.user_id}
            className={cn(
              'relative flex items-center gap-3 px-4 py-3 sm:gap-4 sm:px-6',
              isMe && 'bg-primary/[0.07]'
            )}
          >
            <span className='sr-only'>
              {t('No. {{rank}}', { rank: item.rank })}
            </span>
            <span
              aria-hidden='true'
              className='font-display text-muted-foreground w-7 shrink-0 text-right text-xl italic [font-variant-numeric:lining-nums_tabular-nums] sm:w-9 sm:text-2xl'
            >
              {item.rank}
            </span>
            <MemberAvatar
              name={item.display_name}
              avatarUrl={item.avatar_url}
              className='size-9'
            />
            <span className='min-w-0 flex-1 truncate text-sm font-medium sm:text-base'>
              {item.display_name}
            </span>
            {isMe && <Badge className='px-1.5 text-[10px]'>{t('Me')}</Badge>}
            <span className='font-display shrink-0 text-lg font-semibold whitespace-nowrap [font-variant-numeric:lining-nums_tabular-nums] sm:text-xl'>
              {props.formatValue(item.value)}
            </span>
            <span
              aria-hidden='true'
              className='bg-primary/30 absolute bottom-0 left-0 h-0.5 rounded-r-full'
              style={{ width: `${(item.value / props.topValue) * 100}%` }}
            />
          </li>
        )
      })}
    </ol>
  )
}
