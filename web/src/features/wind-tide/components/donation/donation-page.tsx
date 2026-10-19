/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { HeartHandshake } from 'lucide-react'
import { useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Button } from '@/components/ui/button'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { donationsQueryOptions } from '../../api'
import type { DonateTarget } from '../../hooks/use-donate-flow'
import { invalidateDonationQueries } from '../../lib/donation'
import type { Donation } from '../../types'
import { DonateDialog } from './donate-dialog'
import { DonationList } from './donation-list'

// The owner's words; the Chinese value is the one members read.
const SLOGAN =
  'If you have spare Antigravity accounts you no longer use, consider donating them for shared use. After all, all for one and one for all...'

type DialogState = { key: number; target: DonateTarget } | null

type DonationPageProps = {
  /** The strip that switches between the pools, when the site has several. */
  tabs?: ReactNode
}

/** The member's donated Antigravity accounts (wind-tide/DESIGN.md §14). */
export function DonationPage(props: DonationPageProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const donations = useQuery(donationsQueryOptions('antigravity'))
  const [dialog, setDialog] = useState<DialogState>(null)
  const openings = useRef(0)
  const items = donations.data?.items ?? []
  const max = donations.data?.max ?? 10
  // Failed rows keep their card until removed but hold no slot (API.md §14).
  const used = items.filter((donation) => donation.status !== 'failed').length

  const openDialog = (target: DonateTarget) => {
    openings.current += 1
    setDialog({ key: openings.current, target })
  }
  const closeDialog = () => {
    setDialog(null)
    void invalidateDonationQueries(queryClient)
  }
  const reauthorize = (donation: Donation) =>
    openDialog({ id: donation.id, email: donation.email })

  let content = <DonationList items={items} onReauthorize={reauthorize} />
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
        icon={HeartHandshake}
        title={t('No donated accounts yet')}
        description={t(
          'Donate a spare Antigravity account and everyone shares its allowance.'
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
            {t('{{used}} / {{max}} donated', { used, max })}
          </span>
          <Button
            size='sm'
            disabled={donations.isPending || used >= max}
            onClick={() => openDialog(null)}
          >
            <HeartHandshake className='size-4' />
            {t('Donate an account')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='space-y-4'>
            {props.tabs}
            <p className='text-muted-foreground text-sm'>{t(SLOGAN)}</p>
            {content}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      {dialog && (
        <DonateDialog
          key={dialog.key}
          open
          onClose={closeDialog}
          target={dialog.target}
        />
      )}
    </>
  )
}
