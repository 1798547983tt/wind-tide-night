/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import { aisLeaderboardQueryOptions } from '../../api'
import type { AisLeaderboardEntry, AisModelLeaderboard } from '../../types'

type BoardKind = 'calls' | 'failed'
type BoardRange = 'today' | 'total'

/** Titles are i18n keys. */
const BOARD_TITLES: Record<BoardKind, string> = {
  calls: 'AIS PG calls',
  failed: 'Failed calls',
}

const TOP = 20

// Sun gold, silver and bronze seals for the first three, as on the podium.
const MEDALS = new Map<number, string>([
  [1, 'bg-wt-sun text-wt-ink'],
  [2, 'bg-wt-silver text-wt-ink'],
  [3, 'bg-wt-bronze text-wt-moon'],
])

/** The two AIS PG boards (wind-tide/API.md §15): most calls, most failed. */
export function AisLeaderboards() {
  const { t } = useTranslation()
  const board = useQuery(aisLeaderboardQueryOptions)

  if (board.isPending) {
    return (
      <div className='grid gap-4 md:grid-cols-2'>
        <Skeleton className='h-64 w-full' />
        <Skeleton className='h-64 w-full' />
      </div>
    )
  }
  if (board.isError) {
    return (
      <p className='text-muted-foreground text-sm'>
        {getServerErrorMessage(
          board.error,
          t('Could not load the leaderboards')
        )}
      </p>
    )
  }
  return (
    <div className='grid gap-4 md:grid-cols-2'>
      <AisBoard kind='calls' models={board.data.models} />
      <AisBoard kind='failed' models={board.data.models} />
    </div>
  )
}

type AisBoardProps = {
  kind: BoardKind
  /** Every model's boards, in the site's order. */
  models: AisModelLeaderboard[]
}

/** One board, with its own model and period switches. */
function AisBoard(props: AisBoardProps) {
  const { t } = useTranslation()
  const [range, setRange] = useState<BoardRange>('today')
  const [model, setModel] = useState(props.models[0]?.model ?? '')
  const current =
    props.models.find((entry) => entry.model === model) ?? props.models[0]
  const entries = current ? current[range][props.kind].slice(0, TOP) : []
  const top = entries[0]?.count ?? 0

  return (
    <Card>
      <CardHeader className='flex flex-wrap items-center justify-between gap-2'>
        <CardTitle className='font-mincho text-base'>
          {t(BOARD_TITLES[props.kind])}
        </CardTitle>
        <div className='flex flex-wrap items-center gap-2'>
          {props.models.length > 1 && (
            <ToggleGroup
              value={[current?.model ?? '']}
              onValueChange={(values) => {
                const next = values[0]
                if (typeof next === 'string' && next) setModel(next)
              }}
              variant='outline'
              size='sm'
              aria-label={t('Model')}
            >
              {props.models.map((entry) => (
                <ToggleGroupItem key={entry.model} value={entry.model}>
                  {entry.model}
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
          )}
          <ToggleGroup
            value={[range]}
            onValueChange={(values) => {
              const next = values[0]
              if (next === 'today' || next === 'total') setRange(next)
            }}
            variant='outline'
            size='sm'
            aria-label={t('Period')}
          >
            <ToggleGroupItem value='today'>{t('Today')}</ToggleGroupItem>
            <ToggleGroupItem value='total'>{t('Cumulative')}</ToggleGroupItem>
          </ToggleGroup>
        </div>
      </CardHeader>
      <CardContent>
        {entries.length === 0 ? (
          <p className='text-muted-foreground text-sm'>{t('Nobody yet')}</p>
        ) : (
          <ol className='divide-y'>
            {entries.map((entry) => (
              <BoardRow key={entry.rank} entry={entry} top={top} />
            ))}
          </ol>
        )}
      </CardContent>
    </Card>
  )
}

type BoardRowProps = {
  entry: AisLeaderboardEntry
  /** First place's count, which a full tide line stands for. */
  top: number
}

function BoardRow(props: BoardRowProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const entry = props.entry
  const share = props.top > 0 ? (entry.count / props.top) * 100 : 0

  return (
    <li className='relative flex items-center gap-3 py-2'>
      <span className='sr-only'>{t('No. {{rank}}', { rank: entry.rank })}</span>
      <span
        aria-hidden='true'
        className={cn(
          'font-display flex size-7 shrink-0 items-center justify-center rounded-full text-sm font-semibold [font-variant-numeric:lining-nums_tabular-nums]',
          MEDALS.get(entry.rank) ?? 'text-muted-foreground italic'
        )}
      >
        {entry.rank}
      </span>
      <span className='min-w-0 flex-1 truncate text-sm font-medium'>
        {entry.display_name}
      </span>
      <span className='font-display shrink-0 text-lg font-semibold [font-variant-numeric:lining-nums_tabular-nums]'>
        {formatNumber(entry.count, locale)}
      </span>
      <span
        aria-hidden='true'
        className='bg-primary/30 absolute bottom-0 left-0 h-0.5 rounded-r-full'
        style={{ width: `${share}%` }}
      />
    </li>
  )
}
