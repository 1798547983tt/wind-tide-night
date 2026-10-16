/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { LongText } from '@/components/long-text'
import { StatusBadge } from '@/components/status-badge'
import { TableId } from '@/components/table-id'
import { formatTimestampToDate } from '@/lib/format'

import { memberName } from '../../lib/allowlist'
import type { AllowlistEntry } from '../../types'
import { AllowlistRowActions } from './allowlist-row-actions'
import { MemberIdentity } from './member-identity'

export function useAllowlistColumns(): ColumnDef<AllowlistEntry>[] {
  const { t } = useTranslation()
  return [
    {
      id: 'member',
      header: t('Member'),
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
      cell: ({ row }) =>
        // Empty until someone signs in with a prefilled username.
        row.original.discord_id ? (
          <TableId value={row.original.discord_id} />
        ) : (
          <span className='text-muted-foreground text-sm'>-</span>
        ),
      size: 200,
    },
    {
      accessorKey: 'note',
      header: t('Note'),
      cell: ({ row }) =>
        row.original.note ? (
          <LongText className='max-w-[220px] text-sm'>
            {row.original.note}
          </LongText>
        ) : (
          <span className='text-muted-foreground text-sm'>-</span>
        ),
      size: 220,
    },
    {
      accessorKey: 'status',
      header: t('Status'),
      meta: { mobileBadge: true },
      cell: ({ row }) =>
        row.original.status === 'removed' ? (
          <StatusBadge
            label={t('Removed')}
            variant='neutral'
            copyable={false}
            className='-ml-1.5'
          />
        ) : (
          <StatusBadge
            label={t('Active')}
            variant='success'
            copyable={false}
            className='-ml-1.5'
          />
        ),
      size: 110,
    },
    {
      accessorKey: 'user_id',
      header: t('Account'),
      cell: ({ row }) => {
        const entry = row.original
        if (entry.user_id > 0) {
          return (
            <span className='text-sm'>
              {t('User {{id}}', { id: entry.user_id })}
              {entry.account_status === 'disabled' && (
                <span className='text-muted-foreground'>
                  <span aria-hidden='true'>{' · '}</span>
                  {t('Disabled')}
                </span>
              )}
            </span>
          )
        }
        // A deleted account also has no user ID; the next sign-in after a
        // restore creates a new one.
        return (
          <span className='text-muted-foreground text-sm'>
            {entry.account_status === 'deleted'
              ? t('Deleted')
              : t('Not signed in yet')}
          </span>
        )
      },
      size: 140,
    },
    {
      accessorKey: 'created_at',
      header: t('Created'),
      meta: { mobileHidden: true },
      cell: ({ row }) => (
        <span className='font-mono text-sm'>
          {formatTimestampToDate(row.original.created_at)}
        </span>
      ),
      size: 180,
    },
    {
      id: 'actions',
      header: () => t('Actions'),
      cell: ({ row }) => <AllowlistRowActions entry={row.original} />,
      meta: { pinned: 'right' as const },
    },
  ]
}
