/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { rejectEntryRequest } from '../../api'
import { invalidateAccessQueries, memberName } from '../../lib/allowlist'
import { useAllowlist } from './allowlist-provider'

/** A rejected applicant has to wait 24 hours before asking again (§3). */
export function RejectRequestDialog() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { open, setOpen, currentRequest } = useAllowlist()
  const reject = useMutation({
    mutationFn: async (id: number) =>
      requireServerSuccess(await rejectEntryRequest(id)),
    onSuccess: async () => {
      toast.success(t('Entry request rejected'))
      setOpen(null)
      await invalidateAccessQueries(queryClient)
    },
    onError: (error) =>
      handleServerError(error, t('Failed to reject the entry request')),
  })

  return (
    <ConfirmDialog
      destructive
      open={open === 'reject'}
      onOpenChange={(isOpen) => {
        if (!isOpen && !reject.isPending) setOpen(null)
      }}
      title={t('Reject this entry request?')}
      desc={t('{{name}} can apply again 24 hours from now.', {
        name: currentRequest ? memberName(currentRequest) : '',
      })}
      confirmText={t('Reject')}
      isLoading={reject.isPending}
      handleConfirm={() => {
        if (currentRequest) reject.mutate(currentRequest.id)
      }}
    />
  )
}
