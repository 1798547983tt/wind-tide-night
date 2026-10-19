/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { handleServerError } from '@/lib/handle-server-error'

import {
  donationUsageQueryOptions,
  removeDonation,
  testDonation,
} from '../../api'
import {
  invalidateDonationQueries,
  needsReauthorization,
} from '../../lib/donation'
import type { Donation } from '../../types'

type DonationCardActionsProps = {
  donation: Donation
  onReauthorize: (donation: Donation) => void
}

/** Test, re-authorize (or retry) and remove, with the server's limits toasted. */
export function DonationCardActions(props: DonationCardActionsProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [confirmRemove, setConfirmRemove] = useState(false)
  const donation = props.donation

  const test = useMutation({
    mutationFn: () => testDonation(donation.id),
    onSuccess: async (result) => {
      if (result.ok) {
        if (result.usage) {
          queryClient.setQueryData(
            donationUsageQueryOptions(donation.id).queryKey,
            result.usage
          )
        }
        toast.success(
          t('Test passed in {{latency}} ms', { latency: result.latency_ms })
        )
      } else {
        toast.error(
          t('Test failed: {{message}}', { message: t(result.message) })
        )
      }
      await invalidateDonationQueries(queryClient)
    },
    onError: (error) =>
      handleServerError(error, t('Could not test the account')),
  })

  const remove = useMutation({
    mutationFn: () => removeDonation(donation.id),
    onSuccess: async () => {
      toast.success(t('Donation removed'))
      setConfirmRemove(false)
      await invalidateDonationQueries(queryClient)
    },
    onError: (error) =>
      handleServerError(error, t('Could not remove the donation')),
  })

  let reauthorizeLabel: string | null = null
  if (donation.status === 'failed') reauthorizeLabel = 'Retry'
  else if (needsReauthorization(donation)) reauthorizeLabel = 'Authorize again'
  // The pool holds a refresh token for an Antigravity account and a Cookie
  // for an AI Studio one; removal deletes whichever it is.
  const removeDescription =
    donation.provider === 'aistudio'
      ? t(
          'The account leaves the pool now and its Cookie is deleted. You can donate it again later.'
        )
      : t(
          'The account leaves the pool now and its token is deleted. You can donate it again later.'
        )

  return (
    <>
      {donation.status === 'active' && (
        <Button
          type='button'
          size='sm'
          variant='outline'
          disabled={test.isPending}
          onClick={() => test.mutate()}
        >
          {test.isPending ? t('Testing…') : t('Test')}
        </Button>
      )}
      {reauthorizeLabel && (
        <Button
          type='button'
          size='sm'
          onClick={() => props.onReauthorize(donation)}
        >
          {t(reauthorizeLabel)}
        </Button>
      )}
      <Button
        type='button'
        size='sm'
        variant='ghost'
        className='text-destructive'
        onClick={() => setConfirmRemove(true)}
      >
        {t('Remove')}
      </Button>
      <ConfirmDialog
        open={confirmRemove}
        onOpenChange={(open) => {
          if (!remove.isPending) setConfirmRemove(open)
        }}
        title={t('Remove this donated account?')}
        desc={removeDescription}
        confirmText={t('Remove')}
        destructive
        isLoading={remove.isPending}
        handleConfirm={() => remove.mutate()}
      />
    </>
  )
}
