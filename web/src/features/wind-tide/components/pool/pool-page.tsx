/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { getServerErrorMessage } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import { poolQueryOptions } from '../../api'
import { sharedPools } from '../../lib/pool'
import { AisTank } from './ais-tank'
import { AntigravityTank } from './antigravity-tank'
import { PoolTicker } from './pool-ticker'
import { SharedPool } from './shared-pool'

type PoolPageProps = {
  /** The strip that switches between the pools, when the site has several. */
  tabs?: ReactNode
}

/**
 * The compute pool tab (wind-tide/API.md §15): the shared tide pool on top,
 * a tank per pool under it and the announcement feed. A pool the site does
 * not run has no tank.
 */
export function PoolPage(props: PoolPageProps) {
  const { t } = useTranslation()
  const pool = useQuery(poolQueryOptions)

  let content: ReactNode = <LoadingState />
  if (pool.isError) {
    content = (
      <ErrorState
        title={t('Could not load the pool')}
        description={getServerErrorMessage(pool.error)}
        onRetry={() => void pool.refetch()}
      />
    )
  } else if (pool.data) {
    const overview = pool.data
    const shared = sharedPools([overview.aistudio, overview.build])
    const tanks: ReactNode[] = []
    if (overview.antigravity) {
      tanks.push(
        <AntigravityTank key='antigravity' level={overview.antigravity} />
      )
    }
    if (overview.aistudio) {
      tanks.push(<AisTank key='aistudio' status={overview.aistudio} />)
    }
    if (overview.build) {
      tanks.push(<AisTank key='build' status={overview.build} />)
    }
    content = (
      <>
        {shared.length > 0 && <SharedPool pools={shared} />}
        {tanks.length === 0 ? (
          <p className='text-muted-foreground text-sm'>
            {t('No pool is reporting right now')}
          </p>
        ) : (
          <div
            className={cn(
              'grid gap-4',
              tanks.length > 1 && 'xl:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]'
            )}
          >
            {tanks}
          </div>
        )}
        <PoolTicker seed={overview.events} />
      </>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Donated accounts')}</SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          size='sm'
          variant='outline'
          disabled={pool.isFetching}
          onClick={() => void pool.refetch()}
        >
          <RefreshCw className='size-4' />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <div className='space-y-4'>
          {props.tabs}
          <header className='space-y-1'>
            <h3 className='font-mincho text-lg font-semibold'>
              {t('Compute pool')}
            </h3>
            <p className='text-muted-foreground text-sm'>
              {t(
                'What the donated accounts hold right now, and who is sharing.'
              )}
            </p>
          </header>
          {content}
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
