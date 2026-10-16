/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatNumber } from '@/lib/format'

import { checkinQueryOptions } from '../../api'
import { useCheckIn } from '../../hooks/use-check-in'
import { shiftMonth } from '../../lib/checkin-calendar'
import { MascotFigure } from '../mascot/mascot-figure'
import { CheckinCalendar } from './checkin-calendar'
import { StampSeal } from './stamp-seal'

/**
 * The wallet's daily check-in (wind-tide/DESIGN.md §4): a seal to stamp for
 * the fixed reward, the streak, and a month calendar of the days stamped, all
 * in Beijing time. Below desktop width the mascot sits beside the seal; on
 * desktops the console's own mascot in the corner keeps her company instead
 * (§8).
 */
export function CheckinCard() {
  const { t, i18n } = useTranslation()
  const titleId = useId()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  // Months back from the current one, for leafing through the calendar.
  const [monthsBack, setMonthsBack] = useState(0)
  // The day stamped from this card, so its print lands with coins.
  const [freshDay, setFreshDay] = useState<string | null>(null)
  const checkIn = useCheckIn()

  const current = useQuery(checkinQueryOptions())
  const currentMonth = current.data?.today.slice(0, 7)
  const shownMonth =
    currentMonth && monthsBack > 0
      ? shiftMonth(currentMonth, -monthsBack)
      : undefined
  // The current month shares its cache entry with `current`.
  const shown = useQuery({
    ...checkinQueryOptions(shownMonth),
    placeholderData: keepPreviousData,
  })

  if (current.isLoading) {
    return (
      <div className='bg-card grid gap-6 rounded-2xl border p-6 lg:grid-cols-2'>
        <div className='space-y-4'>
          <Skeleton className='h-7 w-32' />
          <Skeleton className='h-4 w-56' />
          <Skeleton className='size-36 rounded-2xl' />
        </div>
        <Skeleton className='h-72 w-full rounded-xl' />
      </div>
    )
  }

  if (!current.data) {
    return (
      <ErrorState
        className='bg-card min-h-0 rounded-2xl border py-10'
        title={t('Failed to fetch checkin status')}
        onRetry={() => void current.refetch()}
      />
    )
  }

  const today = current.data.today
  const fresh = freshDay === today
  const stamped = current.data.checked_in_today || fresh
  // A fresh stamp's own count arrives before the month is fetched again.
  const streak = Math.max(
    current.data.streak,
    fresh ? (checkIn.data?.streak ?? 0) : 0
  )
  const reward = formatQuotaWithCurrency(current.data.reward_quota, {
    digitsLarge: 0,
  })
  const month = shownMonth ?? today.slice(0, 7)
  const stampedDays = new Set(
    (shown.data?.stats.records ?? []).map((record) => record.checkin_date)
  )
  if (fresh) stampedDays.add(today)

  const stamp = () => {
    checkIn.mutate(undefined, {
      onSuccess: (result) => setFreshDay(result.checkin_date),
    })
  }

  return (
    <section
      aria-labelledby={titleId}
      className='bg-card relative isolate overflow-hidden rounded-2xl border'
    >
      {/* A large brush 签 soaked into the corner of the sheet. */}
      <span
        aria-hidden='true'
        className='font-brush text-primary/[0.06] pointer-events-none absolute -top-8 -right-3 -z-10 text-[10rem] leading-none select-none'
      >
        签
      </span>

      {/* Side by side only from lg: with the console's sidebar open, a
          narrower card cannot fit the mascot and the seal in half of it. */}
      <div className='grid gap-8 p-5 sm:p-7 lg:grid-cols-[minmax(0,0.95fr)_minmax(0,1.05fr)] lg:gap-12 lg:p-9'>
        <div className='flex flex-col gap-6'>
          <header className='space-y-1.5'>
            <h2
              id={titleId}
              className='font-mincho text-xl font-semibold sm:text-2xl'
            >
              {t('Daily Check-in')}
            </h2>
            <p className='text-muted-foreground text-sm'>
              {t('Stamp a seal every day to receive {{amount}}.', {
                amount: reward,
              })}
            </p>
          </header>

          {/* The seal is the centre of the card: it takes the room the
              calendar leaves beside it and sits in the middle of it. */}
          <div className='flex flex-1 flex-col justify-center gap-3'>
            <div className='flex items-end justify-center gap-6 sm:gap-14'>
              {/* Below lg she sits beside the seal and says her line. */}
              <MascotFigure
                mood={stamped ? 'rich' : 'wave'}
                line={
                  stamped
                    ? t('All stamped! See you tomorrow~')
                    : t("Today's seal is waiting for you~")
                }
                sizes='112px'
                className='shrink-0 lg:hidden'
                imageClassName='size-24 sm:size-28'
              />

              {stamped ? (
                <div className='flex flex-col items-center gap-2'>
                  <StampSeal
                    stamped
                    fresh={fresh}
                    reward={reward}
                    className='lg:size-44'
                  />
                  <p className='font-mincho text-sm font-semibold lg:text-base'>
                    {t('Stamped for today')}
                  </p>
                </div>
              ) : (
                <button
                  type='button'
                  disabled={checkIn.isPending}
                  onClick={stamp}
                  className='group focus-visible:ring-ring/50 flex flex-col items-center gap-2 rounded-2xl outline-none focus-visible:ring-3 disabled:cursor-progress'
                >
                  <StampSeal
                    stamped={false}
                    pressing={checkIn.isPending}
                    reward={reward}
                    className='transition-transform group-hover:-translate-y-1 lg:size-44'
                  />
                  <span className='font-mincho text-primary text-base font-semibold lg:text-lg'>
                    {t('Stamp to check in')}
                  </span>
                  <span className='text-muted-foreground text-xs'>
                    {t('Receive {{amount}}', { amount: reward })}
                  </span>
                </button>
              )}
            </div>

            <p role='status' className='text-center text-sm font-medium'>
              {fresh &&
                t('Checked in. {{amount}} has been added to your balance.', {
                  amount: reward,
                })}
            </p>
          </div>

          {/* The streak heads the row; the other three share one line on
              phones and in the half-width column, so none is left alone. */}
          <dl className='grid grid-cols-3 gap-x-4 gap-y-4 border-t pt-5 sm:grid-cols-4 lg:grid-cols-3 xl:grid-cols-4'>
            <div className='col-span-3 sm:col-span-1 lg:col-span-3 xl:col-span-1'>
              <dt className='text-muted-foreground text-xs'>
                {t('Check-in streak')}
              </dt>
              <dd className='font-display text-primary text-5xl leading-none font-semibold italic [font-variant-numeric:lining-nums_tabular-nums]'>
                {formatNumber(streak, locale)}
              </dd>
            </div>
            <StatItem
              label={t('Check-ins this month')}
              value={t('{{days}} days', {
                days: formatNumber(
                  shown.data?.stats.checkin_count ?? 0,
                  locale
                ),
              })}
            />
            <StatItem
              label={t('Total check-ins')}
              value={t('{{days}} days', {
                days: formatNumber(current.data.stats.total_checkins, locale),
              })}
            />
            <StatItem
              label={t('Total earned')}
              value={formatQuotaWithCurrency(current.data.stats.total_quota, {
                digitsLarge: 0,
              })}
            />
          </dl>
        </div>

        <CheckinCalendar
          month={month}
          today={today}
          stampedDays={stampedDays}
          isCurrentMonth={monthsBack === 0}
          loading={shown.isFetching}
          onShift={(offset) =>
            setMonthsBack((back) => Math.max(0, back - offset))
          }
          className='lg:border-l lg:pl-12'
        />
      </div>
    </section>
  )
}

function StatItem(props: { label: string; value: string }) {
  return (
    <div className='min-w-0'>
      <dt className='text-muted-foreground text-xs'>{props.label}</dt>
      <dd className='mt-1 truncate text-base font-semibold tabular-nums'>
        {props.value}
      </dd>
    </div>
  )
}
