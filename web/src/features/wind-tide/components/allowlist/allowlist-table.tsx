/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
  useDataTable,
} from '@/components/data-table'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { createServerError } from '@/lib/server-error-message'

import { getAllowlist } from '../../api'
import { ALLOWLIST_STATUSES, allowlistQueryKey } from '../../lib/allowlist'
import { useAllowlistColumns } from './allowlist-columns'

const route = getRouteApi('/_authenticated/allowlist/')

export function AllowlistTable() {
  const { t } = useTranslation()
  const columns = useAllowlistColumns()
  const isMobile = useMediaQuery('(max-width: 640px)')

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
    columnFilters: [{ columnId: 'status', searchKey: 'status', type: 'array' }],
  })
  const statusFilter = columnFilters.find((filter) => filter.id === 'status')
    ?.value as string[] | undefined
  const status = ALLOWLIST_STATUSES.find((value) => value === statusFilter?.[0])

  const params = {
    p: pagination.pageIndex + 1,
    page_size: pagination.pageSize,
    keyword: globalFilter?.trim() || undefined,
    status,
  }
  const { data, isLoading, isFetching } = useQuery({
    queryKey: allowlistQueryKey(params),
    queryFn: async () => {
      const result = await getAllowlist(params)
      if (!result.success) {
        throw createServerError(result, t('Failed to load the allowlist'))
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
    columnFilters,
    globalFilter,
    pagination,
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
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
      emptyTitle={t('No allowlist entries')}
      emptyDescription={t(
        'Add a Discord account here, or approve an entry request.'
      )}
      skeletonKeyPrefix='allowlist-skeleton'
      applyHeaderSize
      toolbarProps={{
        searchPlaceholder: t('Search Discord ID, username or note...'),
        searchDebounceMs: 500,
        filters: [
          {
            columnId: 'status',
            title: t('Status'),
            // The filter translates its option labels itself.
            options: [
              { label: 'Active', value: 'active' },
              { label: 'Removed', value: 'removed' },
            ],
            singleSelect: true,
          },
        ],
      }}
      getRowClassName={(row, context) => {
        if (row.original.status !== 'removed') return undefined
        return context.isMobile ? DISABLED_ROW_MOBILE : DISABLED_ROW_DESKTOP
      }}
    />
  )
}
