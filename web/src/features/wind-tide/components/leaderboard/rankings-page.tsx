/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useNavigate, useSearch } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { PageTransition } from '@/components/page-transition'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import type { RankingPeriod } from '@/features/rankings/types'

import { boardHasPeriod } from '../../lib/leaderboard'
import type { LeaderboardBoard, LeaderboardPeriod } from '../../types'
import { MemberBoard } from './member-board'
import { ModelBoards } from './model-boards'

type Board = LeaderboardBoard | 'models'

// The tabs in the order they are shown, with the i18n key of each name.
const TABS: { board: Board; label: string }[] = [
  { board: 'calls', label: 'Call board' },
  { board: 'failed', label: 'Failed board' },
  { board: 'success_streak', label: 'Success streak board' },
  { board: 'fail_streak', label: 'Fail streak board' },
  { board: 'balance', label: 'Balance board' },
  { board: 'models', label: 'Model board' },
]

function isBoard(value: string): value is Board {
  return TABS.some((tab) => tab.board === value)
}

// A wider underline in the brand colour, and tabs only as wide as their name,
// which never breaks across lines.
const TAB_CLASS =
  'font-mincho h-auto flex-none px-0 pt-1 pb-3 text-base whitespace-nowrap sm:text-lg group-data-horizontal/tabs:after:-bottom-px group-data-horizontal/tabs:after:h-[3px] after:rounded-full after:bg-primary'

/**
 * The rankings page (wind-tide/DESIGN.md §5): the members' call, failed,
 * streak and balance boards, and the upstream model board as the last tab.
 * The board and its period live in the address, so a link opens the same
 * view.
 */
export function RankingsPage() {
  const { t } = useTranslation()
  const search = useSearch({ from: '/rankings/' })
  const navigate = useNavigate()
  const board = search.board ?? 'calls'
  const range = search.range ?? 'week'
  const period = search.period ?? 'week'
  // The model board has periods of its own; of the members' boards, only
  // those counted over time offer one.
  const hasRange = board !== 'models' && boardHasPeriod(board)

  const show = (next: {
    board?: Board
    range?: LeaderboardPeriod
    period?: RankingPeriod
  }) => {
    void navigate({
      to: '/rankings',
      search: (prev) => ({ ...prev, ...next }),
    })
  }

  return (
    <PublicLayout showMainContainer={false}>
      <main className='relative isolate'>
        <div
          aria-hidden='true'
          className='wt-rankings-sky pointer-events-none absolute inset-x-0 top-0 -z-10 h-[560px]'
        />
        {/* The top padding clears the fixed 4rem header. */}
        <PageTransition className='mx-auto w-full max-w-[1120px] px-4 pt-20 pb-16 sm:px-6 sm:pt-24 lg:px-8'>
          <header className='relative'>
            {/* A large brush 榜 soaked into the corner of the page. */}
            <span
              aria-hidden='true'
              className='font-brush text-primary/[0.07] pointer-events-none absolute -top-10 right-0 -z-10 text-[9rem] leading-none select-none sm:-top-16 sm:text-[14rem]'
            >
              榜
            </span>
            <h1 className='font-mincho text-[clamp(2.25rem,6vw,3.75rem)] leading-tight font-semibold'>
              {t('Rankings')}
            </h1>
            <p className='text-muted-foreground mt-3 max-w-xl text-sm sm:text-base'>
              {t(
                'Who calls the most, who fails the most, the longest streaks and who has saved the most: the boards refresh every minute.'
              )}
            </p>
          </header>

          <Tabs
            value={board}
            onValueChange={(value) => {
              if (isBoard(value)) {
                show({ board: value })
              }
            }}
            className='mt-10 gap-0'
          >
            <div className='flex flex-col gap-3 border-b sm:flex-row sm:items-end sm:justify-between'>
              {/* Six tabs outgrow a phone, so the strip scrolls sideways on
                  its own rather than wrapping or widening the page. The
                  pixel of padding keeps the active underline, which hangs
                  below the tab onto the border, inside the scrolling box. */}
              <div className='-mb-px min-w-0 overflow-x-auto pb-px'>
                <TabsList
                  variant='line'
                  className='h-auto flex-nowrap gap-6 p-0 sm:gap-8'
                >
                  {TABS.map((tab) => (
                    <TabsTrigger
                      key={tab.board}
                      value={tab.board}
                      className={TAB_CLASS}
                    >
                      {t(tab.label)}
                    </TabsTrigger>
                  ))}
                </TabsList>
              </div>

              <div className='pb-3'>
                {hasRange && (
                  <ToggleGroup
                    value={[range]}
                    onValueChange={(values) => {
                      const next = values[0]
                      if (
                        next === 'today' ||
                        next === 'week' ||
                        next === 'all'
                      ) {
                        show({ range: next })
                      }
                    }}
                    variant='outline'
                    size='sm'
                    aria-label={t('Period')}
                  >
                    <ToggleGroupItem value='today'>
                      {t('Today')}
                    </ToggleGroupItem>
                    <ToggleGroupItem value='week'>
                      {t('Last 7 days')}
                    </ToggleGroupItem>
                    <ToggleGroupItem value='all'>
                      {t('All time')}
                    </ToggleGroupItem>
                  </ToggleGroup>
                )}
                {board === 'models' && (
                  <ToggleGroup
                    value={[period]}
                    onValueChange={(values) => {
                      const next = values[0]
                      if (
                        next === 'today' ||
                        next === 'week' ||
                        next === 'month' ||
                        next === 'year'
                      ) {
                        show({ period: next })
                      }
                    }}
                    variant='outline'
                    size='sm'
                    aria-label={t('Period')}
                  >
                    <ToggleGroupItem value='today'>
                      {t('Today')}
                    </ToggleGroupItem>
                    <ToggleGroupItem value='week'>{t('Week')}</ToggleGroupItem>
                    <ToggleGroupItem value='month'>
                      {t('Month')}
                    </ToggleGroupItem>
                    <ToggleGroupItem value='year'>{t('Year')}</ToggleGroupItem>
                  </ToggleGroup>
                )}
              </div>
            </div>

            <TabsContent value={board} className='pt-6 sm:pt-8'>
              {board === 'models' ? (
                <ModelBoards period={period} />
              ) : (
                <MemberBoard board={board} range={range} />
              )}
            </TabsContent>
          </Tabs>
        </PageTransition>
      </main>
    </PublicLayout>
  )
}
