/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { createServerError } from '@/lib/server-error-message'

import { getEntryRequests } from '../../api'
import {
  ENTRY_REQUEST_STATUSES,
  entryRequestsQueryKey,
} from '../../lib/allowlist'
import type { EntryRequestStatus } from '../../types'
import { useEntryRequestsColumns } from './entry-requests-columns'

const route = getRouteApi('/_authenticated/allowlist/')

export function EntryRequestsTable() {
  const { t } = useTranslation()
  const columns = useEntryRequestsColumns()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const search = route.useSearch()
  const navigate = route.useNavigate()
  // The status is one required choice, which the faceted filter cannot
  // express (clearing it would mean "all"), so it is a toggle group.
  const status = search.requestStatus ?? 'pending'
  const statusLabels: Record<EntryRequestStatus, string> = {
    pending: t('Pending'),
    approved: t('Approved'),
    rejected: t('Rejected'),
  }

  const { columnFilters, pagination, onPaginationChange, ensurePageInRange } =
    useTableUrlState({
      search,
      navigate,
      pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
      globalFilter: { enabled: false },
    })

  const params = {
    p: pagination.pageIndex + 1,
    page_size: pagination.pageSize,
    status,
  }
  const { data, isLoading, isFetching } = useQuery({
    queryKey: entryRequestsQueryKey(params),
    queryFn: async () => {
      const result = await getEntryRequests(params)
      if (!result.success) {
        throw createServerError(result, t('Failed to load entry requests'))
      }
      return {
        items: result.data?.items ?? [],
        total: result.data?.total ?? 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const { table } = useDataTable({
    data: data?.items ?? [],
    columns,
    getRowId: (row) => String(row.id),
    // Always empty: the toolbar reads it to decide whether to offer Reset.
    columnFilters,
    pagination,
    onPaginationChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total ?? 0,
    ensurePageInRange,
  })

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No entry requests')}
      emptyDescription={t(
        'Discord accounts outside the allowlist show up here after they try to sign in.'
      )}
      skeletonKeyPrefix='entry-requests-skeleton'
      applyHeaderSize
      toolbarProps={{
        hideViewOptions: true,
        customSearch: (
          <ToggleGroup
            value={[status]}
            onValueChange={(values) => {
              const next = ENTRY_REQUEST_STATUSES.find(
                (value) => value === values[0]
              )
              if (!next) return
              void navigate({
                search: (previous) => ({
                  ...previous,
                  page: undefined,
                  requestStatus: next === 'pending' ? undefined : next,
                }),
              })
            }}
            variant='outline'
            size='sm'
            aria-label={t('Entry request status')}
          >
            {ENTRY_REQUEST_STATUSES.map((value) => (
              <ToggleGroupItem key={value} value={value}>
                {statusLabels[value]}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
        ),
      }}
    />
  )
}
