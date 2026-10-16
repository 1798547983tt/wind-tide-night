/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import type { QueryClient } from '@tanstack/react-query'
import { z } from 'zod'

import type { AllowlistQuery, EntryRequestQuery } from '../api'
import type { AllowlistStatus, EntryRequestStatus } from '../types'

// The owner's allowlist page (wind-tide/DESIGN.md §3).

const ALLOWLIST_TABS = ['requests', 'allowlist'] as const

/** Pending comes first: it is the default view and the owner's to-do list. */
export const ENTRY_REQUEST_STATUSES = [
  'pending',
  'approved',
  'rejected',
] as const satisfies readonly EntryRequestStatus[]

export const ALLOWLIST_STATUSES = [
  'active',
  'removed',
] as const satisfies readonly AllowlistStatus[]

/** URL state of the /allowlist page; both tabs share page and pageSize. */
export const allowlistSearchSchema = z.object({
  /** Absent means the entry requests tab. */
  tab: z.enum(ALLOWLIST_TABS).optional().catch(undefined),
  page: z.number().optional().catch(1),
  pageSize: z.number().optional().catch(10),
  // Allowlist tab
  filter: z.string().optional().catch(''),
  status: z.array(z.enum(ALLOWLIST_STATUSES)).optional().catch([]),
  /** Entry requests tab; absent means pending. */
  requestStatus: z.enum(ENTRY_REQUEST_STATUSES).optional().catch(undefined),
})

const ALLOWLIST_QUERY_ROOT = ['wind-tide', 'allowlist'] as const
// Also the root of the pending-count query behind the sidebar badge.
const ENTRY_REQUESTS_QUERY_ROOT = ['wind-tide', 'entry-requests'] as const

export function allowlistQueryKey(params: AllowlistQuery) {
  return [...ALLOWLIST_QUERY_ROOT, 'list', params] as const
}

export function entryRequestsQueryKey(params: EntryRequestQuery) {
  return [...ENTRY_REQUESTS_QUERY_ROOT, 'list', params] as const
}

/**
 * Approving a request adds an allowlist entry, and adding an entry approves a
 * pending request for the same Discord ID, so every change refreshes both
 * lists and the pending count together.
 */
export function invalidateAccessQueries(queryClient: QueryClient) {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: ALLOWLIST_QUERY_ROOT }),
    queryClient.invalidateQueries({ queryKey: ENTRY_REQUESTS_QUERY_ROOT }),
  ])
}

/** The pending count as a badge shows it; a long queue stays narrow. */
export function pendingCountBadge(count: number): string {
  return count > 99 ? '99+' : String(count)
}

/** The name to show for a Discord account, best source first. */
export function memberName(member: {
  display_name: string
  discord_username: string
  discord_id: string
}): string {
  return member.display_name || member.discord_username || member.discord_id
}
