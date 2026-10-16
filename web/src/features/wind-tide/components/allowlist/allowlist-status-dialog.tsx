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
import { AuthOperationError } from '@/lib/secure-verification'
import { requireServerSuccess } from '@/lib/server-error-message'

import { removeAllowlistEntry, restoreAllowlistEntry } from '../../api'
import { invalidateAccessQueries, memberName } from '../../lib/allowlist'
import type { AllowlistEntry } from '../../types'
import { useAllowlist } from './allowlist-provider'

type StatusChange = { entry: AllowlistEntry; remove: boolean }

/**
 * Confirms removing or restoring an allowlist entry. Removal takes effect at
 * once (wind-tide/DESIGN.md §3); restoring brings back the same account and
 * balance.
 */
export function AllowlistStatusDialog() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const {
    open,
    setOpen,
    currentEntry,
    requestVerification,
    verificationActive,
  } = useAllowlist()
  const remove = open === 'remove'
  const change = useMutation({
    mutationFn: async (variables: StatusChange) => {
      const entry = variables.entry
      let proofToken: string | undefined
      // An entry with an account disables or re-enables that account, so the
      // owner proves it is them the same way the users page asks.
      if (entry.user_id > 0) {
        const proof = await requestVerification({
          scope: 'admin.user.manage',
          context: {
            user_id: entry.user_id,
            action: variables.remove ? 'disable' : 'enable',
          },
          title: variables.remove
            ? t('Verify to remove from the allowlist')
            : t('Verify to restore to the allowlist'),
          description: t(
            'Confirm your identity before changing the account {{username}}.',
            { username: memberName(entry) }
          ),
        })
        // Cancelled: nothing changes and the confirmation shows again.
        if (!proof) return null
        proofToken = proof.proof_token
      }
      const result = variables.remove
        ? await removeAllowlistEntry(entry.id, proofToken)
        : await restoreAllowlistEntry(entry.id, proofToken)
      return requireServerSuccess(result)
    },
    onSuccess: async (result, variables) => {
      if (!result) return
      toast.success(
        variables.remove
          ? t('Removed from the allowlist')
          : t('Restored to the allowlist')
      )
      setOpen(null)
      await invalidateAccessQueries(queryClient)
    },
    onError: (error) =>
      handleServerError(
        AuthOperationError.from(error),
        t('An unexpected error occurred')
      ),
  })

  const name = currentEntry ? memberName(currentEntry) : ''
  const linked = (currentEntry?.user_id ?? 0) > 0
  let title = t('Restore {{name}} to the allowlist?', { name })
  let description = t('They can sign in with Discord once restored.')
  if (linked) {
    // Removal turned their keys off, and restoring leaves them off.
    description = t(
      'Their account and balance come back, and they can sign in again. Their API keys stay off until they turn them back on or create new ones.'
    )
  } else if (currentEntry?.account_status === 'deleted') {
    // The server unlinks a deleted account, so the next sign-in starts over.
    description = t(
      'Their old account was deleted. Once restored, signing in with Discord creates a new account.'
    )
  }
  if (remove) {
    title = t('Remove {{name}} from the allowlist?', { name })
    description = linked
      ? t(
          'This takes effect at once: they are signed out and their API keys stop working. They cannot apply again until you restore them.'
        )
      : t(
          'This takes effect at once: this Discord account cannot sign in or apply again until you restore it.'
        )
  }

  return (
    <ConfirmDialog
      destructive={remove}
      // Hidden, not closed, while the step-up dialog is showing.
      open={(open === 'remove' || open === 'restore') && !verificationActive}
      onOpenChange={(isOpen) => {
        if (!isOpen && !change.isPending) setOpen(null)
      }}
      title={title}
      desc={description}
      confirmText={remove ? t('Remove') : t('Restore')}
      isLoading={change.isPending}
      handleConfirm={() => {
        if (currentEntry) change.mutate({ entry: currentEntry, remove })
      }}
    />
  )
}
