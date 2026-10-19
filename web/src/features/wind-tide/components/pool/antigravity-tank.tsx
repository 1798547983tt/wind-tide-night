/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestampRelative } from '@/lib/format'

import { formatRemaining, windowLabel } from '../../lib/donation'
import { antigravityLevel } from '../../lib/pool'
import type { AntigravityPoolLevel } from '../../types'
import { PoolTank, TankGauge } from './pool-tank'

type AntigravityTankProps = {
  level: AntigravityPoolLevel
}

/**
 * How much quota the Antigravity pool has left on average, across every
 * account in it, with a chip per quota bucket (wind-tide/API.md §15).
 */
export function AntigravityTank(props: AntigravityTankProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const level = props.level
  // Before the refresher's first round there is nothing to show.
  const waiting = level.fetched_at === 0
  const fraction = antigravityLevel(level)

  let details = (
    <p className='text-muted-foreground text-sm'>
      {t('No data yet; the first reading arrives within five minutes')}
    </p>
  )
  if (!waiting) {
    details = (
      <>
        <p className='text-muted-foreground text-sm'>
          {t('Average remaining quota across the pool')}
        </p>
        <p className='text-sm'>
          {t('{{healthy}} / {{accounts}} accounts healthy', {
            healthy: level.healthy,
            accounts: level.accounts,
          })}
        </p>
        {level.buckets.length > 0 && (
          <ul
            className='flex flex-wrap gap-1.5'
            aria-label={t('Quota buckets')}
          >
            {level.buckets.map((bucket) => (
              <li key={`${bucket.name}/${bucket.window}`}>
                <Badge variant='outline' className='font-normal'>
                  {bucket.name} · {t(windowLabel(bucket.window))} ·{' '}
                  {formatRemaining(bucket.average_remaining_fraction)}
                </Badge>
              </li>
            ))}
          </ul>
        )}
        <p className='text-muted-foreground text-xs'>
          {t('Updated {{when}}', {
            when: formatTimestampRelative(level.fetched_at, 'seconds', locale),
          })}
        </p>
      </>
    )
  }

  return (
    <PoolTank tone='tide'>
      <TankGauge
        level={waiting ? 0 : fraction}
        calm={waiting}
        title={t('Antigravity')}
        figure={waiting ? '—' : formatRemaining(fraction)}
      >
        {details}
      </TankGauge>
    </PoolTank>
  )
}
