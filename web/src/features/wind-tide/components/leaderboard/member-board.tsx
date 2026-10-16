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
import type { LeaderboardBoard, LeaderboardPeriod } from '../../types'
import { MascotFigure } from '../mascot/mascot-figure'
import { MyPlace } from './my-place'
import { Podium } from './podium'
import { RankingList } from './ranking-list'

type MemberBoardProps = {
  board: LeaderboardBoard
  /** The calls board's period; the balance board has none. */
  range: LeaderboardPeriod
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
  const calls = props.board === 'calls'
  const formatValue = (value: number) =>
    calls
      ? t('{{calls}} calls', { calls: formatNumber(value, locale) })
      : formatQuotaWithCurrency(value, { digitsLarge: 0 })
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
        {calls && props.range !== 'all' && ` · ${t('Counted by Beijing time')}`}
      </p>

      {data.items.length === 0 ? (
        <Empty className='bg-card rounded-2xl border py-12'>
          <EmptyHeader>
            {/* Nobody yet, so she dozes off. */}
            <MascotFigure mood='sleep' sizes='112px' imageClassName='size-28' />
            <EmptyTitle className='font-mincho text-base'>
              {calls ? t('No calls yet') : t('Nobody has a balance yet')}
            </EmptyTitle>
            <EmptyDescription>
              {calls
                ? t('First place is waiting for you~')
                : t('Check in to get on the board~')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <>
          <Podium
            items={data.items.slice(0, 3)}
            myId={user.id}
            formatValue={formatValue}
          />
          {data.items.length > 3 && (
            <RankingList
              items={data.items.slice(3)}
              topValue={data.items[0].value}
              myId={user.id}
              formatValue={formatValue}
            />
          )}
          <MyPlace board={data} formatValue={formatValue} />
        </>
      )}
    </div>
  )
}
