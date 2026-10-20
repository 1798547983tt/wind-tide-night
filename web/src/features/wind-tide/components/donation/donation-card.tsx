/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardAction,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestampRelative } from '@/lib/format'

import {
  donationBadge,
  formatResetTime,
  maskEmail,
  WB_REALM_LABELS,
} from '../../lib/donation'
import type {
  AisDonation,
  AntigravityDonation,
  BuildDonation,
  Donation,
  WbDonation,
} from '../../types'
import { DonationCardActions } from './donation-card-actions'
import { DonationUsage } from './donation-usage'

type DonationCardProps = {
  donation: Donation
  /**
   * Antigravity: authorize again, or retry a failed record. AI Studio: donate
   * again after a failure.
   */
  onReauthorize: (donation: Donation) => void
}

/** One donated account: the pool's verdict, usage, the last test and actions. */
export function DonationCard(props: DonationCardProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const donation = props.donation
  const badge = donationBadge(donation)
  const lastTest = donation.last_test
  // Only a working Antigravity account has usage worth asking Google for;
  // the AI Studio pool reports none.
  const usageEnabled =
    donation.provider === 'antigravity' &&
    donation.status === 'active' &&
    donation.pool !== null &&
    (donation.pool.state === 'ok' || donation.pool.state === 'cooldown')
  let title = donation.email
  let details: ReactNode = null
  if (donation.provider === 'antigravity') {
    details = <AntigravityPoolDetails donation={donation} locale={locale} />
  } else if (donation.provider === 'aistudio') {
    title = donation.email ? maskEmail(donation.email) : t('Google account')
    details = <AisPoolDetails donation={donation} locale={locale} />
  } else if (donation.provider === 'build') {
    // The BUILD pool never reads the email; the account goes by its slot.
    title = donation.pool
      ? t('BUILD account #{{index}}', { index: donation.pool.index })
      : t('BUILD account')
    details = <BuildPoolDetails donation={donation} />
  } else {
    // The WorkBuddy pool reads no email either; the account goes by the
    // nickname Tencent gave it.
    title = donation.nickname || t('WorkBuddy account')
    details = <WbPoolDetails donation={donation} locale={locale} />
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className='truncate text-base'>{title}</CardTitle>
        <CardAction>
          <StatusBadge
            label={t(badge.label)}
            variant={badge.variant}
            copyable={false}
          />
        </CardAction>
      </CardHeader>
      <CardContent className='space-y-3 text-sm'>
        {details}
        {donation.status === 'failed' && donation.fail_reason && (
          <p className='text-destructive'>{t(donation.fail_reason)}</p>
        )}
        {donation.provider === 'antigravity' && (
          <DonationUsage donationId={donation.id} enabled={usageEnabled} />
        )}
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

type PoolDetailsProps<T> = {
  donation: T
  locale?: string
}

/** CPA's counters, the end of a cooldown and its own status message. */
function AntigravityPoolDetails(props: PoolDetailsProps<AntigravityDonation>) {
  const { t } = useTranslation()
  const pool = props.donation.pool
  if (!pool) return null
  return (
    <>
      {pool.state !== 'unknown' && (
        <p className='text-muted-foreground'>
          {t(
            '{{success}} succeeded · {{failed}} failed since the pool last started',
            { success: pool.success, failed: pool.failed }
          )}
        </p>
      )}
      {pool.state === 'cooldown' && pool.retry_at > 0 && (
        <p>
          {t('Back at {{time}}', {
            time: formatResetTime(pool.retry_at, props.locale),
          })}
        </p>
      )}
      {pool.message && pool.state !== 'ok' && (
        <p className='text-muted-foreground break-words'>{pool.message}</p>
      )}
    </>
  )
}

/**
 * The AI Studio pool's figures: what the account carried upstream today, its
 * counters, and for a paused or exhausted account when it comes back and why.
 */
function AisPoolDetails(props: PoolDetailsProps<AisDonation>) {
  const { t } = useTranslation()
  const pool = props.donation.pool
  if (!pool) return null
  return (
    <>
      <p>
        {t('Used upstream today: {{count}}', { count: pool.requests_today })}
      </p>
      {pool.cooling.length > 0 && (
        <ul className='flex flex-wrap gap-1.5'>
          {pool.cooling.map((entry) => (
            <li key={entry.model}>
              <Badge variant='warning'>
                {t('{{model}} quota used up until {{time}}', {
                  model: entry.model,
                  time: formatResetTime(entry.retry_at, props.locale),
                })}
              </Badge>
            </li>
          ))}
        </ul>
      )}
      <p className='text-muted-foreground'>
        {t(
          '{{success}} succeeded · {{failed}} failed since the pool last started',
          { success: pool.success, failed: pool.failed }
        )}
      </p>
      {pool.state !== 'available' && pool.retry_at > 0 && (
        <p>
          {t('Back at {{time}}', {
            time: formatResetTime(pool.retry_at, props.locale),
          })}
        </p>
      )}
      {pool.state !== 'available' && pool.message && (
        <p className='text-muted-foreground break-words'>{t(pool.message)}</p>
      )}
    </>
  )
}

/**
 * The BUILD pool's figures: the account's slot, whether the pool is serving
 * with it right now, and for an account it can no longer use, why.
 */
function BuildPoolDetails(props: PoolDetailsProps<BuildDonation>) {
  const { t } = useTranslation()
  const pool = props.donation.pool
  if (!pool) return null
  let turn: string | null = null
  if (pool.state === 'available') {
    turn = pool.current
      ? t('Serving requests right now')
      : t('Waiting its turn in the rotation')
  }
  return (
    <>
      <p>{t('Slot {{index}} in the pool', { index: pool.index })}</p>
      {turn && <p className='text-muted-foreground'>{turn}</p>}
      {pool.state !== 'available' && pool.message && (
        <p className='text-muted-foreground break-words'>{t(pool.message)}</p>
      )}
    </>
  )
}

/**
 * The WorkBuddy pool's figures: which sign-in the account came from, the
 * credits it has left, its counters, and for a resting account when it is
 * back and why.
 */
function WbPoolDetails(props: PoolDetailsProps<WbDonation>) {
  const { t } = useTranslation()
  const pool = props.donation.pool
  if (!pool) return null
  let realm = pool.realm
  if (pool.realm === 'cn' || pool.realm === 'global') {
    realm = t(WB_REALM_LABELS[pool.realm])
  }
  return (
    <>
      {pool.state !== 'missing' && (
        <>
          <p>
            {t('{{credits}} / {{total}} credits left', {
              credits: formatNumber(pool.credits, props.locale),
              total: formatNumber(pool.credits_total, props.locale),
            })}
          </p>
          <p className='text-muted-foreground'>
            {realm}
            {pool.checked_in && ` · ${t('Checked in today')}`}
          </p>
          <p className='text-muted-foreground'>
            {t(
              '{{success}} succeeded · {{failed}} failed since the pool last started',
              { success: pool.success, failed: pool.failed }
            )}
          </p>
        </>
      )}
      {pool.state === 'cooldown' && pool.retry_at > 0 && (
        <p>
          {t('Back at {{time}}', {
            time: formatResetTime(pool.retry_at, props.locale),
          })}
        </p>
      )}
      {pool.state !== 'available' && pool.message && (
        <p className='text-muted-foreground break-words'>{t(pool.message)}</p>
      )}
    </>
  )
}
