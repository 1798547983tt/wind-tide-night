/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { getServerErrorMessage } from '@/lib/server-error-message'

import {
  donationQueryOptions,
  startDonation,
  submitDonationCallback,
} from '../api'
import { invalidateDonationQueries } from '../lib/donation'
import type { DonationStart } from '../types'

export type DonateStep = 'account' | 'authorize' | 'verify'

/** The record a re-authorization continues; null donates a new account. */
export type DonateTarget = { id: number; email: string } | null

const PROGRESS_POLL_MS = 3000

/**
 * The three-step donate flow (wind-tide/API.md §14): ask the server for
 * Google's link, hand back the callback address, then watch the record until
 * the pool confirms or rejects it. Server refusals stay inside the dialog as
 * text (the mutations own their onError, so nothing toasts), and the member
 * fixes the input without losing the step.
 */
export function useDonateFlow(initialTarget: DonateTarget) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  // A failed donation retried from step 3 continues its own record.
  const [continuation, setContinuation] = useState<DonateTarget>(null)
  const target = continuation ?? initialTarget
  const [started, setStarted] = useState<DonationStart | null>(null)
  const [submittedId, setSubmittedId] = useState<number | null>(null)
  const [startError, setStartError] = useState<string | null>(null)
  const [submitError, setSubmitError] = useState<string | null>(null)

  const start = useMutation({
    mutationFn: (email: string) =>
      startDonation(target ? { email, donation_id: target.id } : { email }),
    onMutate: () => setStartError(null),
    onSuccess: (data) => setStarted(data),
    onError: (error) =>
      setStartError(
        getServerErrorMessage(error, t('Could not start the donation'))
      ),
  })

  const submit = useMutation({
    mutationFn: (callbackUrl: string) => {
      if (!started) throw new Error('No donation has been started')
      return submitDonationCallback({
        flow_token: started.flow_token,
        callback_url: callbackUrl,
      })
    },
    onMutate: () => setSubmitError(null),
    onSuccess: (data) => setSubmittedId(data.id),
    onError: (error) =>
      setSubmitError(
        getServerErrorMessage(error, t('Could not submit the address'))
      ),
  })

  const progress = useQuery({
    ...donationQueryOptions(submittedId ?? 0),
    enabled: submittedId !== null,
    refetchInterval: (query) =>
      query.state.data?.status === 'verifying' ? PROGRESS_POLL_MS : false,
  })

  const settled =
    progress.data !== undefined && progress.data.status !== 'verifying'
  useEffect(() => {
    if (settled) void invalidateDonationQueries(queryClient)
  }, [settled, queryClient])

  let step: DonateStep = 'account'
  if (submittedId !== null) step = 'verify'
  else if (started) step = 'authorize'

  /** Step 3's Retry: back to step 1 with the failed record and its email. */
  const retry = () => {
    if (progress.data) {
      setContinuation({ id: progress.data.id, email: progress.data.email })
    }
    setStarted(null)
    setSubmittedId(null)
    setSubmitError(null)
    submit.reset()
  }

  /** Step 2's Start again after the link expired: back to step 1, email kept. */
  const restart = () => {
    setStarted(null)
    setSubmitError(null)
    submit.reset()
  }

  return {
    step,
    target,
    started,
    start,
    startError,
    submit,
    submitError,
    progress,
    retry,
    restart,
  }
}
