/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { Skeleton } from '@/components/ui/skeleton'
import {
  MarketShareSection,
  ModelsSection,
  PulseSection,
} from '@/features/rankings/components'
import { useRankings } from '@/features/rankings/hooks/use-rankings'
import type { RankingPeriod } from '@/features/rankings/types'

/**
 * The model board: the upstream model, vendor and movers rankings as they
 * are (wind-tide/DESIGN.md §5), under the wind-tide page.
 */
export function ModelBoards(props: { period: RankingPeriod }) {
  const { t } = useTranslation()
  const rankings = useRankings(props.period)
  const snapshot = rankings.data?.data

  if (rankings.isLoading) {
    return (
      <div className='space-y-6'>
        <Skeleton className='h-[420px] w-full rounded-xl' />
        <Skeleton className='h-[360px] w-full rounded-xl' />
        <Skeleton className='h-[180px] w-full rounded-xl' />
      </div>
    )
  }

  if (!snapshot) {
    return (
      <ErrorState
        className='bg-card rounded-2xl border'
        title={t('Unable to load rankings')}
        description={
          rankings.error instanceof Error
            ? rankings.error.message
            : t('Unable to load rankings data')
        }
        onRetry={() => void rankings.refetch()}
      />
    )
  }

  return (
    <div className='space-y-8'>
      <ModelsSection
        history={snapshot.models_history}
        rows={snapshot.models}
        period={props.period}
      />
      <MarketShareSection
        history={snapshot.vendor_share_history}
        rows={snapshot.vendors}
        period={props.period}
      />
      <PulseSection
        movers={snapshot.top_movers}
        droppers={snapshot.top_droppers}
      />
    </div>
  )
}
