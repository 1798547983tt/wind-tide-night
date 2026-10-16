/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { WalletStatsCard } from '@/features/wallet/components/wallet-stats-card'
import { useStatus } from '@/hooks/use-status'

import { walletQueryOptions } from '../../api'
import { CheckinCard } from '../checkin/checkin-card'

/**
 * The wallet of a free relay: balance, usage and the daily check-in, with no
 * top-up, plans, referrals or codes (wind-tide/DESIGN.md §6).
 */
export function WalletPage() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const wallet = useQuery(walletQueryOptions)

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Wallet')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 sm:gap-5'>
          <WalletStatsCard
            user={wallet.data ?? null}
            loading={wallet.isLoading}
          />
          {status?.checkin_enabled === true && <CheckinCard />}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
