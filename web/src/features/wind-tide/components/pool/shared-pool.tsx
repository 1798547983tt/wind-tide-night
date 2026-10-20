/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState, type CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'

import { Progress } from '@/components/ui/progress'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { useCountUp } from '../../hooks/use-count-up'
import { sharedPoolLevel } from '../../lib/pool'
import type { AisModelPool } from '../../types'
import { WaveEdge } from '../wave-edge'

type SharedPoolProps = {
  /** The public pool of every model the site shares, in the site's order. */
  pools: AisModelPool[]
}

// Lanterns drifting up through the water, each at its own place and pace,
// so the scene is the same on every render.
const LANTERNS = [
  { left: '6%', size: 10, delay: 0, duration: 14 },
  { left: '17%', size: 6, delay: 3.2, duration: 11 },
  { left: '29%', size: 8, delay: 6.5, duration: 16 },
  { left: '41%', size: 5, delay: 1.4, duration: 12 },
  { left: '53%', size: 9, delay: 8.1, duration: 15 },
  { left: '64%', size: 6, delay: 4.6, duration: 13 },
  { left: '76%', size: 11, delay: 2.3, duration: 17 },
  { left: '88%', size: 7, delay: 9.7, duration: 12 },
  { left: '95%', size: 5, delay: 5.9, duration: 14 },
] as const

/** How long the pool glows after a gift or dips after a take. */
const PULSE_MS = 1600

// The water never drains out of sight: at least this much of the panel's
// height, and more as the day's gifts stay in the pool.
const WATER_MIN_PERCENT = 16
const WATER_RANGE_PERCENT = 56

type Pulse = 'surge' | 'ebb' | null

/**
 * The shared tide pool (wind-tide/API.md §15): everything members gave to
 * the public pools today and nobody has taken yet, in one scene at the top
 * of the compute pool tab. The water stands as high as the share of the
 * day's gifts still in the pool, the big figure counts to every change, a
 * gift makes the whole pool glow and a take sends a ripple through it. A
 * member who prefers reduced motion sees the same figures, still.
 */
export function SharedPool(props: SharedPoolProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const reducedMotion = !!useReducedMotion()
  const pools = props.pools
  const balance = pools.reduce((sum, pool) => sum + pool.pool_balance, 0)
  const given = pools.reduce((sum, pool) => sum + pool.pool_given, 0)
  const taken = pools.reduce((sum, pool) => sum + pool.pool_taken, 0)
  const shown = useCountUp(balance, { disabled: reducedMotion })
  const tide = given > 0 ? Math.min(1, Math.max(0, balance / given)) : 0
  const percent = Math.round(tide * 100)

  // A gift makes the pool surge and a take makes it ebb, for a moment.
  const [pulse, setPulse] = useState<Pulse>(null)
  const lastBalance = useRef(balance)
  useEffect(() => {
    const before = lastBalance.current
    lastBalance.current = balance
    if (reducedMotion || before === balance) return
    setPulse(balance > before ? 'surge' : 'ebb')
    const timer = window.setTimeout(() => setPulse(null), PULSE_MS)
    return () => window.clearTimeout(timer)
  }, [balance, reducedMotion])

  return (
    <section
      aria-label={t('Shared pool')}
      data-pulse={pulse ?? undefined}
      className='wt-shared-pool text-wt-moon relative isolate overflow-hidden rounded-3xl px-5 py-6 sm:px-8 sm:py-8'
    >
      <div
        aria-hidden='true'
        className='wt-shared-pool-sky absolute inset-0 -z-20'
      />
      <div
        aria-hidden='true'
        className='wt-shared-pool-moon absolute -top-12 right-4 -z-10 size-44 rounded-full sm:-top-16 sm:right-10 sm:size-60'
      />
      {!reducedMotion && (
        <div
          aria-hidden='true'
          className='absolute inset-0 -z-10 overflow-hidden'
        >
          {LANTERNS.map((lantern) => (
            <span
              key={lantern.left}
              className='wt-shared-pool-lantern'
              style={
                {
                  left: lantern.left,
                  width: lantern.size,
                  height: lantern.size,
                  '--wt-delay': `${lantern.delay}s`,
                  '--wt-duration': `${lantern.duration}s`,
                } as CSSProperties
              }
            />
          ))}
        </div>
      )}
      <div
        aria-hidden='true'
        className='wt-shared-pool-water absolute inset-x-0 bottom-0 -z-10'
        style={{
          height: `${WATER_MIN_PERCENT + tide * WATER_RANGE_PERCENT}%`,
        }}
      >
        <WaveEdge
          color='var(--wt-shared-water)'
          className='absolute inset-x-0 bottom-full h-10 sm:h-14'
        />
      </div>

      <div className='grid gap-8 lg:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)] lg:items-end'>
        <div>
          <p className='text-xs font-medium tracking-[0.3em] uppercase opacity-80'>
            {t('Shared pool')}
          </p>
          <h4 className='font-mincho mt-2 text-2xl font-semibold sm:text-3xl'>
            {t('The tide everyone fills')}
          </h4>
          <p className='mt-2 max-w-md text-sm opacity-80'>
            {t(
              'Calls members give today, for anyone to take; the pool empties at midnight, Beijing time.'
            )}
          </p>
          <p className='wt-shared-pool-figure font-display mt-6 text-6xl leading-none font-semibold italic [font-variant-numeric:lining-nums_tabular-nums] sm:text-7xl'>
            {formatNumber(shown, locale)}
          </p>
          <p className='mt-2 text-sm opacity-80'>
            {t('calls in the pool right now')}
          </p>
          <dl className='mt-4 flex flex-wrap gap-x-6 gap-y-1 text-sm'>
            <div className='flex gap-1.5'>
              <dt className='opacity-70'>{t('Given today')}</dt>
              <dd className='font-semibold [font-variant-numeric:tabular-nums]'>
                {formatNumber(given, locale)}
              </dd>
            </div>
            <div className='flex gap-1.5'>
              <dt className='opacity-70'>{t('Taken today')}</dt>
              <dd className='font-semibold [font-variant-numeric:tabular-nums]'>
                {formatNumber(taken, locale)}
              </dd>
            </div>
            <div className='flex gap-1.5'>
              <dt className='opacity-70'>{t('Tide')}</dt>
              <dd className='font-semibold [font-variant-numeric:tabular-nums]'>
                {t('{{percent}}% of the gifts still in the pool', { percent })}
              </dd>
            </div>
          </dl>
        </div>
        <ul
          aria-label={t('Pools by model')}
          className='grid gap-3 sm:grid-cols-2 lg:grid-cols-1 xl:grid-cols-2'
        >
          {pools.map((pool) => (
            <PoolLantern
              key={pool.relay_model}
              pool={pool}
              locale={locale}
              reducedMotion={reducedMotion}
            />
          ))}
        </ul>
      </div>
    </section>
  )
}

type PoolLanternProps = {
  pool: AisModelPool
  locale?: string
  reducedMotion: boolean
}

/** One model's public pool: its balance, the day's gifts and takes, its tide. */
function PoolLantern(props: PoolLanternProps) {
  const { t } = useTranslation()
  const pool = props.pool
  const balance = useCountUp(pool.pool_balance, {
    disabled: props.reducedMotion,
  })
  const percent = Math.round(sharedPoolLevel(pool) * 100)

  return (
    <li className='wt-shared-pool-glass rounded-2xl p-4'>
      <p className='truncate text-xs font-medium tracking-[0.18em] uppercase opacity-80'>
        {pool.relay_model}
      </p>
      <p className='font-display mt-1 text-3xl leading-none font-semibold italic [font-variant-numeric:lining-nums_tabular-nums] sm:text-4xl'>
        {formatNumber(balance, props.locale)}
      </p>
      <p className='mt-1.5 text-xs opacity-80'>
        {t('Given {{given}} · taken {{taken}} today', {
          given: formatNumber(pool.pool_given, props.locale),
          taken: formatNumber(pool.pool_taken, props.locale),
        })}
      </p>
      <Progress
        value={percent}
        aria-label={t('{{model}} tide', { model: pool.relay_model })}
        className='mt-3'
      />
    </li>
  )
}
