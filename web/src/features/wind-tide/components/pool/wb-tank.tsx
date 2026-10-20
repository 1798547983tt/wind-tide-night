/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { Link } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { buttonVariants } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestampRelative } from '@/lib/format'
import { cn } from '@/lib/utils'

import { formatRemaining, WB_TEXTS } from '../../lib/donation'
import { wbLevel } from '../../lib/pool'
import type { WbPoolLevel } from '../../types'
import { PoolTank, TankGauge } from './pool-tank'

type WbTankProps = {
  level: WbPoolLevel
}

/**
 * How many credits the WorkBuddy pool has left across every account in it
 * (wind-tide/API.md §16). The pool has no gate, so the tank stands open to
 * everyone, with the way to donate under the figures.
 */
export function WbTank(props: WbTankProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const level = props.level
  // No answer from the pool yet, or nothing in it to measure.
  const waiting = level.fetched_at === 0
  const empty = !waiting && level.accounts === 0
  const fraction = wbLevel(level)

  let details: ReactNode = (
    <p className='text-muted-foreground text-sm'>
      {t('The pool is not answering right now')}
    </p>
  )
  if (!waiting) {
    details = (
      <>
        <p className='text-muted-foreground text-sm'>
          {empty
            ? t('No account in the pool yet; be the first to donate one')
            : t('Credits left across the pool')}
        </p>
        {!empty && (
          <>
            <p className='text-sm'>
              {t('{{credits}} / {{total}} credits', {
                credits: formatNumber(level.credits, locale),
                total: formatNumber(level.credits_total, locale),
              })}
            </p>
            <p className='text-sm'>
              {t('{{healthy}} / {{accounts}} accounts healthy', {
                healthy: level.healthy,
                accounts: level.accounts,
              })}
            </p>
            {level.cooling > 0 && (
              <p className='text-muted-foreground text-xs'>
                {t('{{count}} cooling down', {
                  count: level.cooling,
                })}
              </p>
            )}
          </>
        )}
        <p className='text-muted-foreground text-xs'>
          {t('Updated {{when}}', {
            when: formatTimestampRelative(level.fetched_at, 'seconds', locale),
          })}
        </p>
        <Link
          to='/donations'
          search={{ tab: 'workbuddy' }}
          className={cn(
            buttonVariants({ variant: 'outline', size: 'sm' }),
            'w-fit'
          )}
        >
          {t(WB_TEXTS.donate)}
        </Link>
      </>
    )
  }

  return (
    <PoolTank tone='plum'>
      <TankGauge
        level={waiting || empty ? 0 : fraction}
        calm={waiting || empty}
        title={t(WB_TEXTS.name)}
        figure={waiting || empty ? '—' : formatRemaining(fraction)}
      >
        {details}
      </TankGauge>
    </PoolTank>
  )
}
