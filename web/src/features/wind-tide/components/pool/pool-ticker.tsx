/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { Megaphone } from 'lucide-react'
import { useReducedMotion } from 'motion/react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestampRelative } from '@/lib/format'

import { usePoolEventsFeed } from '../../hooks/use-pool-events-feed'
import { poolEventSentence, rotatePoolEvents } from '../../lib/pool'
import type { PoolEvent } from '../../types'

/** Rows on show at once. */
const WINDOW = 5
const ADVANCE_MS = 4000

type PoolTickerProps = {
  /** The announcements the pool overview came with. */
  seed: PoolEvent[]
}

/**
 * The announcement feed (wind-tide/API.md §15): the newest on top, the rows
 * advancing one by one while there are more than fit, holding still under
 * the pointer and for a member who prefers reduced motion.
 */
export function PoolTicker(props: PoolTickerProps) {
  const { t } = useTranslation()
  const events = usePoolEventsFeed(props.seed)
  const reducedMotion = !!useReducedMotion()
  const [paused, setPaused] = useState(false)
  const rolling = events.length > WINDOW && !paused && !reducedMotion
  // A fresh announcement starts the rows again from the top.
  const newestId = events[0]?.id ?? 0

  return (
    <Card
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
    >
      <CardHeader>
        <CardTitle className='font-mincho flex items-center gap-2 text-base'>
          <Megaphone className='size-4' aria-hidden='true' />
          {t('Announcements')}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {events.length === 0 ? (
          <p className='text-muted-foreground text-sm'>
            {t('Nothing has happened yet')}
          </p>
        ) : (
          <TickerRows key={newestId} events={events} rolling={rolling} />
        )}
      </CardContent>
    </Card>
  )
}

type TickerRowsProps = {
  events: PoolEvent[]
  rolling: boolean
}

function TickerRows(props: TickerRowsProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [offset, setOffset] = useState(0)
  const rolling = props.rolling

  useEffect(() => {
    if (!rolling) return
    const timer = window.setInterval(
      () => setOffset((current) => current + 1),
      ADVANCE_MS
    )
    return () => window.clearInterval(timer)
  }, [rolling])

  const shown = rotatePoolEvents(props.events, offset, WINDOW)
  const anonymous = t('Someone')

  return (
    <ol className='space-y-2'>
      {shown.map((event) => {
        const sentence = poolEventSentence(event, anonymous)
        if (!sentence) return null
        return (
          <li
            key={event.id}
            className='wt-pool-event-in flex items-baseline justify-between gap-3 text-sm'
          >
            <span className='min-w-0 truncate'>
              {t(sentence.key, sentence.params)}
            </span>
            <span className='text-muted-foreground shrink-0 text-xs'>
              {formatTimestampRelative(event.created_at, 'seconds', locale)}
            </span>
          </li>
        )
      })}
    </ol>
  )
}
