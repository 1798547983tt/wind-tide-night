/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery, useQueryClient } from '@tanstack/react-query'

import { fetchPoolEvents } from '../api'
import { mergePoolEvents, POOL_EVENTS_QUERY_KEY } from '../lib/pool'
import type { PoolEvent } from '../types'

const EVENTS_POLL_MS = 15_000

/**
 * The announcement feed (wind-tide/API.md §15): seeded with the events the
 * pool overview came with, then asking every 15 seconds only for what is
 * newer than the newest on show and folding that in. Newest first.
 */
export function usePoolEventsFeed(seed: PoolEvent[]): PoolEvent[] {
  const queryClient = useQueryClient()
  const feed = useQuery({
    queryKey: POOL_EVENTS_QUERY_KEY,
    queryFn: async () => {
      const current =
        queryClient.getQueryData<PoolEvent[]>(POOL_EVENTS_QUERY_KEY) ?? seed
      const newest = current[0]?.id ?? 0
      const fresh = await fetchPoolEvents(newest)
      return mergePoolEvents(current, fresh.items)
    },
    initialData: () => mergePoolEvents([], seed),
    staleTime: EVENTS_POLL_MS,
    refetchInterval: EVENTS_POLL_MS,
    retry: false,
    meta: { errorToast: false },
  })
  return feed.data
}
