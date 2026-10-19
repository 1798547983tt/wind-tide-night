/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { FlaskConical } from 'lucide-react'
import { useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { donationsQueryOptions } from '../../api'
import { invalidateDonationQueries } from '../../lib/donation'
import { invalidatePoolQueries } from '../../lib/pool'
import { AisDonateDialog } from './ais-donate-dialog'
import { DonationList } from './donation-list'

// The owner's words; the Chinese value is the one members read.
const SLOGAN =
  'Spare AI Studio accounts become shared AIS PG calls: every account you donate gives you 50 calls a day, and the pool shares the load.'

type AisDonationPageProps = {
  /** The strip that switches between the pools, when the site has several. */
  tabs?: ReactNode
}

/** The member's donated AI Studio accounts (wind-tide/API.md §15). */
export function AisDonationPage(props: AisDonationPageProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const donations = useQuery(donationsQueryOptions('aistudio'))
  // A key per opening, so every run of the dialog starts from step 1.
  const [dialogKey, setDialogKey] = useState<number | null>(null)
  const openings = useRef(0)
  const items = donations.data?.items ?? []
  const max = donations.data?.max ?? 10
  // Failed rows keep their card until removed but hold no slot.
  const used = items.filter((donation) => donation.status !== 'failed').length
  const full = used >= max

  const openDialog = () => {
    openings.current += 1
    setDialogKey(openings.current)
  }
  const closeDialog = () => {
    setDialogKey(null)
    void invalidateDonationQueries(queryClient)
    void invalidatePoolQueries(queryClient)
  }

  let content = <DonationList items={items} onReauthorize={openDialog} />
  if (donations.isPending) {
    content = <LoadingState />
  } else if (donations.isError) {
    content = (
      <ErrorState
        title={t('Could not load your donations')}
        description={getServerErrorMessage(donations.error)}
        onRetry={() => void donations.refetch()}
      />
    )
  } else if (items.length === 0) {
    content = (
      <EmptyState
        icon={FlaskConical}
        title={t('No AI Studio accounts yet')}
        description={t(
          'Donate a spare AI Studio account: every active account gives you AIS PG calls each day, and the pool shares the load.'
        )}
      />
    )
  }

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          {t('Donated accounts')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <span className='text-muted-foreground text-sm'>
            {t('Donated {{count}} / {{max}}', { count: used, max })}
          </span>
          <Button
            size='sm'
            disabled={donations.isPending || full}
            onClick={openDialog}
          >
            <FlaskConical className='size-4' />
            {t('Donate an AI Studio account')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='space-y-4'>
            {props.tabs}
            <header className='space-y-1'>
              <h3 className='font-mincho text-lg font-semibold'>
                {t('AIS playground')}
              </h3>
              <p className='text-muted-foreground text-sm'>{t(SLOGAN)}</p>
            </header>
            {full && (
              <p className='text-warning text-sm'>
                {t('You have reached the limit of {{max}} donated accounts', {
                  max,
                })}
              </p>
            )}
            {content}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      {dialogKey !== null && (
        <AisDonateDialog key={dialogKey} open onClose={closeDialog} />
      )}
    </>
  )
}
