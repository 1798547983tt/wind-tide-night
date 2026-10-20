/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { leaderboardQueryOptions } from '../../api'
import { boardHasPeriod, boardIsStreak } from '../../lib/leaderboard'
import type {
  LeaderboardBoard,
  LeaderboardItem,
  LeaderboardPeriod,
} from '../../types'
import { MascotFigure } from '../mascot/mascot-figure'
import { MyPlace } from './my-place'
import { Podium } from './podium'
import { RankingList } from './ranking-list'

type MemberBoardProps = {
  board: LeaderboardBoard
  /** The period of the calls and failed boards; the others have none. */
  range: LeaderboardPeriod
}

// What a board with nobody on it says, as i18n keys: the title and the
// line under it.
const EMPTY_COPY: Record<
  LeaderboardBoard,
  { title: string; description: string }
> = {
  calls: {
    title: 'No calls yet',
    description: 'First place is waiting for you~',
  },
  failed: {
    title: 'No failed calls yet',
    description: 'Long may it last~',
  },
  success_streak: {
    title: 'No streaks yet',
    description: 'One successful call starts a streak~',
  },
  fail_streak: {
    title: 'No failed calls yet',
    description: 'Long may it last~',
  },
  balance: {
    title: 'Nobody has a balance yet',
    description: 'Check in to get on the board~',
  },
}

/**
 * A members' leaderboard (wind-tide/DESIGN.md §5): the top three on a
 * podium, everyone else down to rank 50 below it, and the member's own place
 * pinned at the bottom. The server rebuilds it once a minute.
 */
export function MemberBoard(props: MemberBoardProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const user = useAuthStore((state) => state.auth.user)
  const leaderboard = useQuery({
    ...leaderboardQueryOptions(props.board, props.range),
    enabled: user != null,
    // Another period keeps the board on show until its own figures land;
    // another board's would be read in the wrong unit, so not those.
    placeholderData: (previous) =>
      previous?.board === props.board ? previous : undefined,
  })

  // Leaderboards are for members only; the server turns visitors away too.
  if (!user) {
    return (
      <Empty className='bg-card rounded-2xl border py-12'>
        <EmptyHeader>
          <MascotFigure mood='wave' sizes='112px' imageClassName='size-28' />
          <EmptyTitle className='font-mincho text-base'>
            {t('Please sign in to view {{module}}.', {
              module: t('Rankings'),
            })}
          </EmptyTitle>
        </EmptyHeader>
        <EmptyContent>
          <Button
            render={<Link to='/sign-in' search={{ redirect: '/rankings' }} />}
          >
            {t('Sign in')}
          </Button>
        </EmptyContent>
      </Empty>
    )
  }

  if (leaderboard.isLoading) {
    return (
      <div className='space-y-8'>
        <div className='grid grid-cols-3 items-end gap-2 sm:gap-6'>
          <Skeleton className='h-52 rounded-t-2xl sm:h-64' />
          <Skeleton className='h-64 rounded-t-2xl sm:h-80' />
          <Skeleton className='h-48 rounded-t-2xl sm:h-56' />
        </div>
        <Skeleton className='h-64 w-full rounded-2xl' />
      </div>
    )
  }

  if (!leaderboard.data) {
    return (
      <ErrorState
        className='bg-card rounded-2xl border'
        title={t('Unable to load rankings')}
        onRetry={() => void leaderboard.refetch()}
      />
    )
  }

  const data = leaderboard.data
  const formatValue = (value: number) => {
    if (props.board === 'calls') {
      return t('{{calls}} calls', { calls: formatNumber(value, locale) })
    }
    if (props.board === 'failed') {
      return t('{{count}} failed', { count: formatNumber(value, locale) })
    }
    if (boardIsStreak(props.board)) {
      return t('{{count}} in a row', { count: formatNumber(value, locale) })
    }
    return formatQuotaWithCurrency(value, { digitsLarge: 0 })
  }
  // On a streak board the run right now stands beside the longest; the
  // server leaves it out when it is nought.
  const formatDetail = boardIsStreak(props.board)
    ? (item: LeaderboardItem) =>
        t('Now {{count}} in a row', {
          count: formatNumber(item.current ?? 0, locale),
        })
    : undefined
  // Days are counted in Beijing time, which the all-time board needs no
  // telling of.
  const countedByDay = boardHasPeriod(props.board) && props.range !== 'all'
  const empty = EMPTY_COPY[props.board]
  const updatedAt = new Intl.DateTimeFormat(locale, {
    hour: '2-digit',
    minute: '2-digit',
  }).format(data.updated_at * 1000)

  return (
    <div
      aria-busy={leaderboard.isPlaceholderData}
      className={cn(
        'space-y-8 transition-opacity',
        leaderboard.isPlaceholderData && 'opacity-60'
      )}
    >
      <p className='text-muted-foreground text-right text-xs'>
        {t('Updated at {{time}}', { time: updatedAt })}
        {countedByDay && ` · ${t('Counted by Beijing time')}`}
      </p>

      {data.items.length === 0 ? (
        <Empty className='bg-card rounded-2xl border py-12'>
          <EmptyHeader>
            {/* Nobody yet, so she dozes off. */}
            <MascotFigure mood='sleep' sizes='112px' imageClassName='size-28' />
            <EmptyTitle className='font-mincho text-base'>
              {t(empty.title)}
            </EmptyTitle>
            <EmptyDescription>{t(empty.description)}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <>
          <Podium
            items={data.items.slice(0, 3)}
            myId={user.id}
            formatValue={formatValue}
            formatDetail={formatDetail}
          />
          {data.items.length > 3 && (
            <RankingList
              items={data.items.slice(3)}
              topValue={data.items[0].value}
              myId={user.id}
              formatValue={formatValue}
              formatDetail={formatDetail}
            />
          )}
          <MyPlace board={data} formatValue={formatValue} />
        </>
      )}
    </div>
  )
}
