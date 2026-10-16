/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useId } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { cn } from '@/lib/utils'

import { monthGrid } from '../../lib/checkin-calendar'

type CheckinCalendarProps = {
  /** The `YYYY-MM` month on show. */
  month: string
  /** Today in Beijing time, `YYYY-MM-DD`. */
  today: string
  /** Days of the month already stamped. */
  stampedDays: ReadonlySet<string>
  /** No later month has begun, so there is nothing to move on to. */
  isCurrentMonth: boolean
  onShift: (offset: -1 | 1) => void
  /** The month's check-ins are still on their way. */
  loading?: boolean
  className?: string
}

/**
 * A month of check-ins in Beijing time, weeks starting on Monday. Each day
 * checked in carries a small vermilion seal; today waits as an empty slot
 * until it is stamped.
 */
export function CheckinCalendar(props: CheckinCalendarProps) {
  const { t, i18n } = useTranslation()
  const headingId = useId()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const monthLabel = new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: 'long',
    timeZone: 'UTC',
  }).format(new Date(`${props.month}-01T00:00:00Z`))
  const weekdayFormat = new Intl.DateTimeFormat(locale, {
    weekday: 'narrow',
    timeZone: 'UTC',
  })
  // 1 January 2024 was a Monday.
  const weekdays = Array.from({ length: 7 }, (_, index) =>
    weekdayFormat.format(Date.UTC(2024, 0, 1 + index))
  )

  return (
    <section
      aria-labelledby={headingId}
      className={cn('space-y-3', props.className)}
    >
      <header className='flex items-center justify-between gap-2'>
        <h3 id={headingId} className='font-mincho text-base font-semibold'>
          {monthLabel}
        </h3>
        <div className='flex items-center gap-1'>
          <Button
            type='button'
            variant='ghost'
            size='icon'
            className='size-8 rounded-full'
            aria-label={t('Previous month')}
            onClick={() => props.onShift(-1)}
          >
            <ChevronLeft className='size-4' aria-hidden='true' />
          </Button>
          <Button
            type='button'
            variant='ghost'
            size='icon'
            className='size-8 rounded-full'
            aria-label={t('Next month')}
            disabled={props.isCurrentMonth}
            onClick={() => props.onShift(1)}
          >
            <ChevronRight className='size-4' aria-hidden='true' />
          </Button>
        </div>
      </header>

      <div
        aria-hidden='true'
        className='text-muted-foreground grid grid-cols-7 text-center text-xs'
      >
        {weekdays.map((weekday, index) => (
          // Narrow names repeat (T, S), so the column is the identity.
          // eslint-disable-next-line react/no-array-index-key
          <span key={index}>{weekday}</span>
        ))}
      </div>

      <ol
        aria-busy={props.loading}
        className='grid grid-cols-7 gap-1 sm:gap-1.5'
      >
        {monthGrid(props.month).map((day, index) => {
          if (day === null) {
            // Blank cells before the 1st have only their place.
            // eslint-disable-next-line react/no-array-index-key
            return <li key={`blank-${index}`} aria-hidden='true' />
          }
          const stamped = props.stampedDays.has(day)
          const isToday = day === props.today
          return (
            <li
              key={day}
              className='flex aspect-square items-center justify-center'
            >
              <span
                // Lining figures in one property: the `tabular-nums` class
                // alone would win, as index.css sets it outside the layers.
                className={cn(
                  'font-display relative flex size-full max-h-11 max-w-11 items-center justify-center rounded-lg text-base [font-variant-numeric:lining-nums_tabular-nums] sm:text-lg',
                  stamped &&
                    'bg-wt-crimson rotate-[-6deg] font-semibold text-[#fff6e6] shadow-[inset_0_0_0_2px_rgb(255_246_230/0.35)]',
                  stamped &&
                    isToday &&
                    'ring-primary/45 ring-offset-card ring-2 ring-offset-2',
                  !stamped &&
                    isToday &&
                    'wt-await border-primary text-primary border-2 border-dashed font-semibold',
                  !stamped && !isToday && day > props.today && 'opacity-35',
                  !stamped &&
                    !isToday &&
                    day < props.today &&
                    'text-muted-foreground'
                )}
              >
                {Number(day.slice(8))}
                {stamped && <span className='sr-only'>{t('Checked in')}</span>}
                {isToday && <span className='sr-only'>{t('Today')}</span>}
              </span>
            </li>
          )
        })}
      </ol>
    </section>
  )
}
