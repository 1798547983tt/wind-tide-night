/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Check, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { approveEntryRequest } from '../../api'
import { invalidateAccessQueries } from '../../lib/allowlist'
import type { EntryRequest } from '../../types'
import { useAllowlist } from './allowlist-provider'

/**
 * Approve admits the applicant at once; Reject asks first because the
 * applicant then has to wait a day to ask again. A rejected request can still
 * be approved; an approved one is done.
 */
export function EntryRequestActions(props: { request: EntryRequest }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { setOpen, setCurrentRequest } = useAllowlist()
  const approve = useMutation({
    mutationFn: async (id: number) =>
      requireServerSuccess(await approveEntryRequest(id)),
    onSuccess: async () => {
      toast.success(t('Entry request approved'))
      await invalidateAccessQueries(queryClient)
    },
    onError: (error) =>
      handleServerError(error, t('Failed to approve the entry request')),
  })

  if (props.request.status === 'approved') return null

  return (
    <div className='flex items-center gap-2'>
      <Button
        size='sm'
        disabled={approve.isPending}
        onClick={() => approve.mutate(props.request.id)}
      >
        <Check />
        {t('Approve')}
      </Button>
      {props.request.status === 'pending' && (
        <Button
          size='sm'
          variant='outline'
          disabled={approve.isPending}
          onClick={() => {
            setCurrentRequest(props.request)
            setOpen('reject')
          }}
        >
          <X />
          {t('Reject')}
        </Button>
      )}
    </div>
  )
}
