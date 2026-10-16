/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { animate, useInView, useReducedMotion } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { toIntlLocale } from '@/i18n/languages'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatNumber } from '@/lib/format'

import { publicStatsQueryOptions } from '../../api'
import { WaveEdge } from '../wave-edge'

type CountUpProps = {
  value: number
  format: (value: number) => string
}

/**
 * Counts up from zero the first time it scrolls into view, then glides from
 * the figure on screen to each refreshed value.
 */
function CountUp(props: CountUpProps) {
  const ref = useRef<HTMLSpanElement>(null)
  const inView = useInView(ref, { once: true, margin: '0px 0px -15% 0px' })
  const reducedMotion = !!useReducedMotion()
  const [current, setCurrent] = useState(0)
  const shown = useRef(0)

  useEffect(() => {
    if (!inView) return
    if (reducedMotion) {
      shown.current = props.value
      setCurrent(props.value)
      return
    }
    const controls = animate(shown.current, props.value, {
      duration: 1.8,
      ease: [0.16, 1, 0.3, 1],
      onUpdate: (value) => {
        shown.current = value
        setCurrent(value)
      },
    })
    return () => controls.stop()
  }, [inView, props.value, reducedMotion])

  return (
    <span ref={ref}>
      <span aria-hidden='true'>{props.format(current)}</span>
      <span className='sr-only'>{props.format(props.value)}</span>
    </span>
  )
}

/** 潮汐表 — the site-wide figures every visitor may see. */
export function TideTable() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const stats = useQuery(publicStatsQueryOptions)
  // A failed refresh keeps the last figures on screen; only an empty table
  // says the tide is out.
  const unavailable = stats.isError && !stats.data

  const figures = [
    {
      key: 'calls',
      label: t('Calls today'),
      value: stats.data?.calls_today,
      format: (value: number) => formatNumber(Math.round(value), locale),
    },
    {
      key: 'rewarded',
      label: t('Balance handed out'),
      value: stats.data?.rewarded_quota_total,
      format: (value: number) => formatQuotaWithCurrency(Math.round(value)),
    },
    {
      key: 'members',
      label: t('Members'),
      value: stats.data?.members,
      format: (value: number) => formatNumber(Math.round(value), locale),
    },
  ]

  return (
    <section className='bg-background relative'>
      <WaveEdge className='absolute inset-x-0 top-0 -translate-y-[calc(100%-1px)]' />
      <div className='mx-auto max-w-6xl px-5 pt-14 pb-20 sm:px-8 lg:pt-20 lg:pb-28'>
        <div className='flex flex-wrap items-end justify-between gap-x-8 gap-y-3 border-b pb-6'>
          <h2 className='font-brush text-5xl leading-none sm:text-6xl'>
            {t('Tide table')}
          </h2>
          <p className='font-display text-muted-foreground text-base italic sm:text-lg'>
            {unavailable
              ? t('The tide has not come in yet.')
              : t('Live figures for the whole site, refreshed every minute.')}
          </p>
        </div>
        <dl className='grid gap-x-10 gap-y-12 pt-12 sm:grid-cols-3'>
          {figures.map((figure, index) => (
            <div key={figure.key} className='relative'>
              <dt className='font-mincho text-muted-foreground flex items-center gap-3 text-sm tracking-[0.3em]'>
                <span className='font-display text-wt-crimson text-base tracking-normal italic'>
                  {String(index + 1).padStart(2, '0')}
                </span>
                {figure.label}
              </dt>
              {/* Lining figures in one property: with the `tabular-nums`
                  class beside it, index.css (outside the layers) would win. */}
              <dd className='font-display mt-3 text-[clamp(2.75rem,6.5vw,5rem)] leading-none font-medium [font-variant-numeric:lining-nums_tabular-nums]'>
                {figure.value == null ? (
                  <span className='text-muted-foreground/60'>∿</span>
                ) : (
                  <CountUp value={figure.value} format={figure.format} />
                )}
              </dd>
            </div>
          ))}
        </dl>
      </div>
    </section>
  )
}
