/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Bot } from 'lucide-react'
import { useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { donationsQueryOptions } from '../../api'
import { invalidateDonationQueries, WB_TEXTS } from '../../lib/donation'
import { invalidatePoolQueries } from '../../lib/pool'
import { DonationList } from './donation-list'
import { WbDonateDialog } from './wb-donate-dialog'

type WbDonationPageProps = {
  /** The strip that switches between the pools, when the site has several. */
  tabs?: ReactNode
}

/**
 * The member's accounts donated to the WorkBuddy pool (wind-tide/API.md
 * §16). The pool has no call count: the page says so at the top, and the
 * cards show what each account has left for everyone.
 */
export function WbDonationPage(props: WbDonationPageProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const donations = useQuery(donationsQueryOptions('workbuddy'))
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
        icon={Bot}
        title={t(WB_TEXTS.emptyTitle)}
        description={t(WB_TEXTS.emptyDescription)}
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
            <Bot className='size-4' />
            {t(WB_TEXTS.donate)}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='space-y-4'>
            {props.tabs}
            <header className='space-y-1'>
              <h3 className='font-mincho text-lg font-semibold'>
                {t(WB_TEXTS.name)}
              </h3>
              <p className='text-muted-foreground text-sm'>
                {t(WB_TEXTS.slogan)}
              </p>
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
        <WbDonateDialog key={dialogKey} open onClose={closeDialog} />
      )}
    </>
  )
}
