/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestampRelative } from '@/lib/format'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { donationUsageQueryOptions } from '../../api'
import {
  formatRemaining,
  formatResetTime,
  tierLabel,
  windowLabel,
} from '../../lib/donation'

type DonationUsageProps = {
  donationId: number
  /** Only an account the pool can use has usage worth asking Google for. */
  enabled: boolean
}

/** What Google says the donated account has left, per model group and window. */
export function DonationUsage(props: DonationUsageProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const usage = useQuery({
    ...donationUsageQueryOptions(props.donationId),
    enabled: props.enabled,
  })

  if (!props.enabled) return null
  if (usage.isPending) return <Skeleton className='h-16 w-full' />
  if (usage.isError) {
    return (
      <p className='text-muted-foreground text-sm'>
        {getServerErrorMessage(
          usage.error,
          t('Could not fetch usage for this account')
        )}
      </p>
    )
  }

  const buckets = usage.data.groups.flatMap((group) =>
    group.buckets.map((bucket) => ({ group: group.name, ...bucket }))
  )
  return (
    <div className='space-y-2'>
      <div className='flex items-center justify-between gap-2 text-sm'>
        <span className='text-muted-foreground'>
          {t('{{tier}} tier · fetched {{when}}', {
            tier: t(tierLabel(usage.data.tier)),
            when: formatTimestampRelative(
              usage.data.fetched_at,
              'seconds',
              locale
            ),
          })}
        </span>
        <Button
          type='button'
          variant='ghost'
          size='sm'
          disabled={usage.isFetching}
          onClick={() => void usage.refetch()}
        >
          {t('Refresh')}
        </Button>
      </div>
      {buckets.length === 0 && (
        <p className='text-muted-foreground text-sm'>
          {t('Google reported no usage figures for this account')}
        </p>
      )}
      {buckets.map((bucket) => (
        <div
          key={`${bucket.group}/${bucket.name}/${bucket.window}`}
          className='space-y-1'
        >
          <div className='flex justify-between gap-2 text-sm'>
            <span className='truncate'>
              {bucket.name} · {t(windowLabel(bucket.window))}
            </span>
            <span className='text-muted-foreground shrink-0'>
              {bucket.reset_at
                ? t('{{remaining}} left · resets {{time}}', {
                    remaining: formatRemaining(bucket.remaining_fraction),
                    time: formatResetTime(bucket.reset_at, locale),
                  })
                : t('{{remaining}} left', {
                    remaining: formatRemaining(bucket.remaining_fraction),
                  })}
            </span>
          </div>
          <Progress value={Math.round(bucket.remaining_fraction * 100)} />
        </div>
      ))}
    </div>
  )
}
