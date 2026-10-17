/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import {
  Card,
  CardAction,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'
import { formatTimestampRelative } from '@/lib/format'

import { donationBadge, formatResetTime } from '../../lib/donation'
import type { Donation } from '../../types'
import { DonationCardActions } from './donation-card-actions'
import { DonationUsage } from './donation-usage'

type DonationCardProps = {
  donation: Donation
  onReauthorize: (donation: Donation) => void
}

/** One donated account: the pool's verdict, usage, the last test and actions. */
export function DonationCard(props: DonationCardProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const donation = props.donation
  const pool = donation.pool
  const badge = donationBadge(donation)
  const usageEnabled =
    donation.status === 'active' &&
    pool !== null &&
    (pool.state === 'ok' || pool.state === 'cooldown')
  const lastTest = donation.last_test

  return (
    <Card>
      <CardHeader>
        <CardTitle className='truncate text-base'>{donation.email}</CardTitle>
        <CardAction>
          <StatusBadge
            label={t(badge.label)}
            variant={badge.variant}
            copyable={false}
          />
        </CardAction>
      </CardHeader>
      <CardContent className='space-y-3 text-sm'>
        {pool && pool.state !== 'unknown' && (
          <p className='text-muted-foreground'>
            {t(
              '{{success}} succeeded · {{failed}} failed since the pool last started',
              { success: pool.success, failed: pool.failed }
            )}
          </p>
        )}
        {pool?.state === 'cooldown' && pool.retry_at > 0 && (
          <p>
            {t('Back at {{time}}', {
              time: formatResetTime(pool.retry_at, locale),
            })}
          </p>
        )}
        {pool && pool.message && pool.state !== 'ok' && (
          <p className='text-muted-foreground break-words'>{pool.message}</p>
        )}
        {donation.status === 'failed' && donation.fail_reason && (
          <p className='text-destructive'>{t(donation.fail_reason)}</p>
        )}
        <DonationUsage donationId={donation.id} enabled={usageEnabled} />
        {lastTest && lastTest.ok && (
          <p className='text-muted-foreground'>
            {t('Last test {{when}}: passed in {{latency}} ms', {
              when: formatTimestampRelative(lastTest.at, 'seconds', locale),
              latency: lastTest.latency_ms,
            })}
          </p>
        )}
        {lastTest && !lastTest.ok && (
          <p className='text-muted-foreground'>
            {t('Last test {{when}}: failed ({{message}})', {
              when: formatTimestampRelative(lastTest.at, 'seconds', locale),
              message: t(lastTest.message),
            })}
          </p>
        )}
      </CardContent>
      <CardFooter className='flex flex-wrap justify-end gap-2'>
        <DonationCardActions
          donation={donation}
          onReauthorize={props.onReauthorize}
        />
      </CardFooter>
    </Card>
  )
}
