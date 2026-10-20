/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { getServerErrorMessage } from '@/lib/server-error-message'

import { donateWorkbuddy, donationQueryOptions } from '../api'
import { invalidateDonationQueries } from '../lib/donation'
import { invalidatePoolQueries } from '../lib/pool'
import type { WbDonationStart, WbRealm } from '../types'

export type WbDonateStep = 'consent' | 'signin' | 'verify'

const PROGRESS_POLL_MS = 3000

/**
 * The three-step WorkBuddy donate flow (wind-tide/API.md §16): accept the
 * risks and pick the sign-in, open the Tencent link the pool hands out, then
 * read the pool's verdict once it has collected the account or given up.
 * Nothing secret passes through here: the pool keeps the account's token,
 * and this side sees the link and the record only. A refusal stays inside
 * the dialog as text.
 */
export function useWbDonateFlow() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [realm, setRealm] = useState<WbRealm>('cn')
  const [accepted, setAccepted] = useState(false)
  const [started, setStarted] = useState<WbDonationStart | null>(null)
  const [startError, setStartError] = useState<string | null>(null)

  const start = useMutation({
    mutationFn: (chosen: WbRealm) =>
      donateWorkbuddy({ realm: chosen, accepted: true }),
    onMutate: () => setStartError(null),
    onSuccess: (data) => setStarted(data),
    onError: (error) =>
      setStartError(
        getServerErrorMessage(error, t('Could not start the sign-in'))
      ),
  })

  const progress = useQuery({
    ...donationQueryOptions(started?.id ?? 0),
    enabled: started !== null,
    refetchInterval: (query) =>
      query.state.data?.status === 'verifying' ? PROGRESS_POLL_MS : false,
  })

  const settled =
    started !== null &&
    progress.data !== undefined &&
    progress.data.id === started.id &&
    progress.data.status !== 'verifying'
  useEffect(() => {
    if (!settled) return
    void invalidateDonationQueries(queryClient)
    // An account that came through fills the tank on the pool tab.
    void invalidatePoolQueries(queryClient)
  }, [settled, queryClient])

  let step: WbDonateStep = 'consent'
  if (accepted) step = 'signin'
  if (settled) step = 'verify'

  /** Step 1's Continue: the consent stands for the rest of this run. */
  const accept = (chosen: WbRealm) => {
    setRealm(chosen)
    setAccepted(true)
    start.mutate(chosen)
  }

  /** A fresh link: after the last one expired, or the sign-in failed. */
  const restart = () => {
    setStarted(null)
    start.mutate(realm)
  }

  return {
    step,
    realm,
    started,
    start,
    startError,
    progress,
    accept,
    restart,
  }
}
