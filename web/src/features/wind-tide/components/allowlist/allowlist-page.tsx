/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { pendingEntryCountQueryOptions } from '../../api'
import { pendingCountBadge } from '../../lib/allowlist'
import { AllowlistDialogs } from './allowlist-dialogs'
import { AllowlistProvider, useAllowlist } from './allowlist-provider'
import { AllowlistTable } from './allowlist-table'
import { EntryRequestsTable } from './entry-requests-table'

const route = getRouteApi('/_authenticated/allowlist/')

/**
 * The owner's admission desk (wind-tide/DESIGN.md §3): entry requests to
 * approve or reject, and the allowlist itself.
 */
export function AllowlistPage() {
  return (
    <AllowlistProvider>
      <AllowlistPageContent />
    </AllowlistProvider>
  )
}

function AllowlistPageContent() {
  const { t } = useTranslation()
  const navigate = route.useNavigate()
  const tab = route.useSearch({ select: (search) => search.tab }) ?? 'requests'
  const { setOpen } = useAllowlist()
  const { data: pendingCount = 0 } = useQuery(pendingEntryCountQueryOptions)

  return (
    <>
      <SectionPageLayout fixedContent>
        <SectionPageLayout.Title>
          {t('Allowlist & entry requests')}
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button size='sm' onClick={() => setOpen('add')}>
            <Plus className='size-4' />
            {t('Add entry')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='flex h-full min-h-0 flex-col gap-4'>
            <Tabs
              value={tab}
              onValueChange={(value) => {
                if (value === tab) return
                // Both tabs read paging and filters from the same URL, so a
                // switch starts the other tab from its defaults.
                void navigate({
                  search: {
                    tab: value === 'allowlist' ? 'allowlist' : undefined,
                  },
                })
              }}
            >
              <TabsList>
                <TabsTrigger value='requests'>
                  {t('Entry requests')}
                  {pendingCount > 0 && (
                    <Badge className='px-1.5'>
                      {pendingCountBadge(pendingCount)}
                    </Badge>
                  )}
                </TabsTrigger>
                <TabsTrigger value='allowlist'>{t('Allowlist')}</TabsTrigger>
              </TabsList>
            </Tabs>
            <div className='min-h-0 flex-1'>
              {tab === 'requests' ? <EntryRequestsTable /> : <AllowlistTable />}
            </div>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <AllowlistDialogs />
    </>
  )
}
