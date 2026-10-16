/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useMutation, useQueryClient } from '@tanstack/react-query'

import { checkIn, walletQueryOptions } from '../api'

/**
 * Stamps today's check-in. Afterwards, failed or not, the check-in months and
 * the wallet are fetched again: a failure can mean another tab got there
 * first, and the page should then show the stamp it put down.
 */
export function useCheckIn() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: checkIn,
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ['wind-tide', 'checkin'] })
      void queryClient.invalidateQueries({
        queryKey: walletQueryOptions.queryKey,
      })
    },
  })
}
