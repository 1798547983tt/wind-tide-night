/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { FlaskConical, Hammer, type LucideIcon } from 'lucide-react'
import { useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { donationsQueryOptions } from '../../api'
import {
  COOKIE_POOL_TEXTS,
  invalidateDonationQueries,
} from '../../lib/donation'
import { invalidatePoolQueries } from '../../lib/pool'
import type { CookieProvider } from '../../types'
import { AisDonateDialog } from './ais-donate-dialog'
import { DonationList } from './donation-list'

/** A flask for the playground, a hammer for BUILD. */
const ICONS: Record<CookieProvider, LucideIcon> = {
  aistudio: FlaskConical,
  build: Hammer,
}

type AisDonationPageProps = {
  /** The pool the page is about. */
  provider: CookieProvider
  /** The strip that switches between the pools, when the site has several. */
  tabs?: ReactNode
}

/**
 * The member's accounts donated to one Cookie pool, the AIS playground or
 * BUILD (wind-tide/API.md §15). The owner's words at the top are the pool's
 * own; the Chinese value is the one members read.
 */
export function AisDonationPage(props: AisDonationPageProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const texts = COOKIE_POOL_TEXTS[props.provider]
  const Icon = ICONS[props.provider]
  const donations = useQuery(donationsQueryOptions(props.provider))
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
        icon={Icon}
        title={t(texts.emptyTitle)}
        description={t(texts.emptyDescription)}
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
            <Icon className='size-4' />
            {t(texts.donate)}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='space-y-4'>
            {props.tabs}
            <header className='space-y-1'>
              <h3 className='font-mincho text-lg font-semibold'>
                {t(texts.name)}
              </h3>
              <p className='text-muted-foreground text-sm'>{t(texts.slogan)}</p>
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
        <AisDonateDialog
          key={dialogKey}
          provider={props.provider}
          open
          onClose={closeDialog}
        />
      )}
    </>
  )
}
