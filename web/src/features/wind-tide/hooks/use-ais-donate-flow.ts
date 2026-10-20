/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { getServerErrorMessage } from '@/lib/server-error-message'

import { COOKIE_DONATE, donationQueryOptions } from '../api'
import { invalidateDonationQueries } from '../lib/donation'
import { invalidatePoolQueries } from '../lib/pool'
import type { CookieProvider } from '../types'

export type AisDonateStep = 'consent' | 'cookie' | 'verify'

const PROGRESS_POLL_MS = 3000

/**
 * The three-step Cookie donate flow (wind-tide/API.md §15), for the AIS
 * playground pool or the BUILD pool: accept the risks, hand the pool the
 * Cookie, then watch the record until the pool has signed in with it or
 * given up. The Cookie lives in the step that collects it and in the one
 * request; nothing here keeps it. A refusal stays inside the dialog as text,
 * so the member pastes again without losing the step.
 */
export function useAisDonateFlow(provider: CookieProvider) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [accepted, setAccepted] = useState(false)
  const [submittedId, setSubmittedId] = useState<number | null>(null)
  const [submitError, setSubmitError] = useState<string | null>(null)

  const submit = useMutation({
    mutationFn: (cookies: string) =>
      COOKIE_DONATE[provider]({ cookies, accepted: true }),
    onMutate: () => setSubmitError(null),
    onSuccess: (data) => setSubmittedId(data.id),
    onError: (error) =>
      setSubmitError(
        getServerErrorMessage(error, t('Could not hand the Cookie to the pool'))
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
    if (!settled) return
    void invalidateDonationQueries(queryClient)
    // An account that came through raises the member's allowance.
    void invalidatePoolQueries(queryClient)
  }, [settled, queryClient])

  let step: AisDonateStep = 'consent'
  if (submittedId !== null) step = 'verify'
  else if (accepted) step = 'cookie'

  /** Step 3's Retry: back to step 2 for a fresh Cookie; the consent stands. */
  const retry = () => {
    setSubmittedId(null)
    setSubmitError(null)
    submit.reset()
  }

  return {
    step,
    accept: () => setAccepted(true),
    submit,
    submitError,
    progress,
    retry,
  }
}
