/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { TableId } from '@/components/table-id'
import { formatTimestampToDate } from '@/lib/format'

import { memberName } from '../../lib/allowlist'
import type { EntryRequest } from '../../types'
import { EntryRequestActions } from './entry-request-actions'
import { MemberIdentity } from './member-identity'

export function useEntryRequestsColumns(): ColumnDef<EntryRequest>[] {
  const { t } = useTranslation()
  return [
    {
      id: 'applicant',
      header: t('Applicant'),
      meta: { mobileTitle: true },
      cell: ({ row }) => (
        <MemberIdentity
          name={memberName(row.original)}
          username={row.original.discord_username}
          avatarUrl={row.original.avatar_url}
        />
      ),
      size: 240,
    },
    {
      accessorKey: 'discord_id',
      header: t('Discord ID'),
      cell: ({ row }) => <TableId value={row.original.discord_id} />,
      size: 200,
    },
    {
      accessorKey: 'requested_at',
      header: t('Requested'),
      cell: ({ row }) => (
        <span className='font-mono text-sm'>
          {formatTimestampToDate(row.original.requested_at)}
        </span>
      ),
      size: 180,
    },
    {
      accessorKey: 'decided_at',
      header: t('Decided'),
      cell: ({ row }) => (
        <span className='font-mono text-sm'>
          {formatTimestampToDate(row.original.decided_at)}
        </span>
      ),
      size: 180,
    },
    {
      id: 'actions',
      header: () => t('Actions'),
      cell: ({ row }) => <EntryRequestActions request={row.original} />,
      meta: { pinned: 'right' as const },
    },
  ]
}
