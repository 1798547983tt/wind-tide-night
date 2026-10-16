/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { Pencil, RotateCcw, UserX } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { DataTableRowActionMenu } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  DropdownMenuItem,
  DropdownMenuShortcut,
} from '@/components/ui/dropdown-menu'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import type { AllowlistEntry } from '../../types'
import { useAllowlist } from './allowlist-provider'

/** Remove is offered on active entries and Restore only on removed ones. */
export function AllowlistRowActions(props: { entry: AllowlistEntry }) {
  const { t } = useTranslation()
  const { setOpen, setCurrentEntry } = useAllowlist()
  const removed = props.entry.status === 'removed'

  return (
    <div className='-ml-1.5 flex items-center gap-1'>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              aria-label={t('Edit note')}
              onClick={() => {
                setCurrentEntry(props.entry)
                setOpen('edit-note')
              }}
            />
          }
        >
          <Pencil />
        </TooltipTrigger>
        <TooltipContent>{t('Edit note')}</TooltipContent>
      </Tooltip>

      <DataTableRowActionMenu ariaLabel={t('Open menu')} modal={false}>
        {removed ? (
          <DropdownMenuItem
            onClick={() => {
              setCurrentEntry(props.entry)
              setOpen('restore')
            }}
          >
            {t('Restore')}
            <DropdownMenuShortcut>
              <RotateCcw size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>
        ) : (
          <DropdownMenuItem
            className='text-destructive focus:text-destructive'
            onClick={() => {
              setCurrentEntry(props.entry)
              setOpen('remove')
            }}
          >
            {t('Remove')}
            <DropdownMenuShortcut>
              <UserX size={16} />
            </DropdownMenuShortcut>
          </DropdownMenuItem>
        )}
      </DataTableRowActionMenu>
    </div>
  )
}
