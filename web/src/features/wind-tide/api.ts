/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { queryOptions } from '@tanstack/react-query'
import type { AxiosRequestConfig } from 'axios'

import type { UserWalletData } from '@/features/wallet/types'
import { api, getSelf } from '@/lib/api'
import { requireServerSuccess } from '@/lib/server-error-message'

import { ART_BASE, type ArtManifest } from './lib/art'
import type {
  AllowlistEntry,
  AllowlistEntryInput,
  AllowlistStatus,
  ApiResponse,
  CheckinMonth,
  CheckinResult,
  DonationList,
  DonationProgress,
  DonationStart,
  DonationSubmitted,
  DonationTestResult,
  DonationUsage,
  EntryRequest,
  EntryRequestStatus,
  Leaderboard,
  LeaderboardBoard,
  LeaderboardPeriod,
  PageData,
  PublicStats,
} from './types'

export const publicStatsQueryOptions = queryOptions({
  queryKey: ['wind-tide', 'public-stats'],
  queryFn: async () => {
    const response = await api.get<ApiResponse<PublicStats>>(
      '/api/wt/public-stats'
    )
    if (!response.data.success || !response.data.data) {
      throw new Error(response.data.message || 'Public stats unavailable')
    }
    return response.data.data
  },
  // The server caches these figures for a minute; poll at the same pace.
  staleTime: 60_000,
  refetchInterval: 60_000,
  retry: false,
  meta: { errorToast: false },
})

export const artManifestQueryOptions = queryOptions({
  queryKey: ['wind-tide', 'art-manifest'],
  queryFn: async (): Promise<ArtManifest> => {
    const response = await fetch(`${ART_BASE}/manifest.json`, {
      cache: 'no-cache',
    })
    if (!response.ok) throw new Error('Art manifest unavailable')
    return (await response.json()) as ArtManifest
  },
  staleTime: Infinity,
  retry: 1,
  meta: { errorToast: false },
})

// Exchanges a one-time link from `wt emergency-login` for an owner session.
// The answer is the ordinary login result: a session or a second-factor step.
export function emergencyLogin(token: string) {
  return api.post<ApiResponse<unknown>>(
    '/api/wt/emergency-login',
    { token },
    { skipBusinessError: true, skipAuthRefresh: true }
  )
}

// The member's own balance, usage and request count for the wallet page.
export const walletQueryOptions = queryOptions({
  queryKey: ['wind-tide', 'wallet'],
  queryFn: async () =>
    requireServerSuccess(await getSelf()).data as UserWalletData,
})

// The member's check-ins for one Beijing-time month, `YYYY-MM`. Without a
// month the server answers for the current one, which the wallet and the
// daily reward share.
export function checkinQueryOptions(month?: string) {
  return queryOptions({
    queryKey: ['wind-tide', 'checkin', month ?? 'current'],
    queryFn: async () => {
      const response = await api.get<ApiResponse<CheckinMonth>>(
        '/api/user/checkin',
        { params: month ? { month } : undefined }
      )
      return requireServerSuccess(response.data).data as CheckinMonth
    },
    staleTime: 30_000,
  })
}

export async function checkIn() {
  const response =
    await api.post<ApiResponse<CheckinResult>>('/api/user/checkin')
  return requireServerSuccess(response.data).data as CheckinResult
}

// The balance board has no period, so its cache key leaves the period out.
export function leaderboardQueryOptions(
  board: LeaderboardBoard,
  period: LeaderboardPeriod
) {
  const params = board === 'calls' ? { board, period } : { board }
  return queryOptions({
    queryKey: ['wind-tide', 'leaderboard', params],
    queryFn: async () => {
      const response = await api.get<ApiResponse<Leaderboard>>(
        '/api/wt/leaderboard',
        { params }
      )
      return requireServerSuccess(response.data).data as Leaderboard
    },
    // The server rebuilds each board at most once a minute.
    staleTime: 60_000,
  })
}

// ============================================================================
// Owner: allowlist and entry requests
// ============================================================================

export type AllowlistQuery = {
  p: number
  page_size: number
  keyword?: string
  status?: AllowlistStatus
}

export async function getAllowlist(
  params: AllowlistQuery
): Promise<ApiResponse<PageData<AllowlistEntry>>> {
  const res = await api.get('/api/wt/allowlist', { params })
  return res.data
}

export async function addAllowlistEntry(
  input: AllowlistEntryInput
): Promise<ApiResponse<AllowlistEntry>> {
  const res = await api.post('/api/wt/allowlist', input)
  return res.data
}

export async function updateAllowlistNote(
  id: number,
  note: string
): Promise<ApiResponse<AllowlistEntry>> {
  const res = await api.put(`/api/wt/allowlist/${id}`, { note })
  return res.data
}

// Removing or restoring an entry that has an account disables or re-enables
// that account, so it carries the same `admin.user.manage` proof as the users
// page. A proof is single-use: the auth-refresh interceptor refreshes before
// the request instead of replaying it.
function allowlistProofConfig(proofToken?: string): AxiosRequestConfig {
  if (!proofToken) return {}
  return {
    headers: { 'X-Security-Proof': proofToken },
    singleUseAuthorization: true,
  }
}

export async function removeAllowlistEntry(
  id: number,
  proofToken?: string
): Promise<ApiResponse<AllowlistEntry>> {
  const res = await api.post(
    `/api/wt/allowlist/${id}/remove`,
    undefined,
    allowlistProofConfig(proofToken)
  )
  return res.data
}

export async function restoreAllowlistEntry(
  id: number,
  proofToken?: string
): Promise<ApiResponse<AllowlistEntry>> {
  const res = await api.post(
    `/api/wt/allowlist/${id}/restore`,
    undefined,
    allowlistProofConfig(proofToken)
  )
  return res.data
}

export type EntryRequestQuery = {
  p: number
  page_size: number
  status?: EntryRequestStatus
}

export async function getEntryRequests(
  params: EntryRequestQuery
): Promise<ApiResponse<PageData<EntryRequest>>> {
  const res = await api.get('/api/wt/entry-requests', { params })
  return res.data
}

// Feeds the red dot on the owner's console, so it stays quiet on failure.
export const pendingEntryCountQueryOptions = queryOptions({
  queryKey: ['wind-tide', 'entry-requests', 'pending-count'],
  queryFn: async () => {
    const response = await api.get<ApiResponse<{ count: number }>>(
      '/api/wt/entry-requests/pending-count'
    )
    return requireServerSuccess(response.data).data?.count ?? 0
  },
  staleTime: 60_000,
  refetchInterval: 60_000,
  meta: { errorToast: false },
})

export async function approveEntryRequest(
  id: number
): Promise<ApiResponse<EntryRequest>> {
  const res = await api.post(`/api/wt/entry-requests/${id}/approve`)
  return res.data
}

export async function rejectEntryRequest(
  id: number
): Promise<ApiResponse<EntryRequest>> {
  const res = await api.post(`/api/wt/entry-requests/${id}/reject`)
  return res.data
}

// wind-tide: donated Antigravity accounts (wind-tide/API.md §14).

export const donationsQueryOptions = queryOptions({
  queryKey: ['wind-tide', 'donations'],
  queryFn: async () => {
    const response =
      await api.get<ApiResponse<DonationList>>('/api/wt/donations')
    return requireServerSuccess(response.data).data as DonationList
  },
  // The page renders its own error state.
  meta: { errorToast: false },
})

/** One record, polled by the donate dialog while the pool verifies it. */
export function donationQueryOptions(id: number) {
  return queryOptions({
    queryKey: ['wind-tide', 'donations', id],
    queryFn: async () => {
      const response = await api.get<ApiResponse<DonationProgress>>(
        `/api/wt/donations/${id}`
      )
      return requireServerSuccess(response.data).data as DonationProgress
    },
    meta: { errorToast: false },
  })
}

export function donationUsageQueryOptions(id: number) {
  return queryOptions({
    queryKey: ['wind-tide', 'donations', id, 'usage'],
    queryFn: async () => {
      const response = await api.get<ApiResponse<DonationUsage>>(
        `/api/wt/donations/${id}/usage`
      )
      return requireServerSuccess(response.data).data as DonationUsage
    },
    // The server caches usage for a minute; asking sooner returns the same.
    staleTime: 60_000,
    retry: false,
    meta: { errorToast: false },
  })
}

export async function startDonation(input: {
  email: string
  donation_id?: number
}) {
  const response = await api.post<ApiResponse<DonationStart>>(
    '/api/wt/donations/start',
    input
  )
  return requireServerSuccess(response.data).data as DonationStart
}

export async function submitDonationCallback(input: {
  flow_token: string
  callback_url: string
}) {
  const response = await api.post<ApiResponse<DonationSubmitted>>(
    '/api/wt/donations/callback',
    input
  )
  return requireServerSuccess(response.data).data as DonationSubmitted
}

export async function testDonation(id: number) {
  const response = await api.post<ApiResponse<DonationTestResult>>(
    `/api/wt/donations/${id}/test`
  )
  return requireServerSuccess(response.data).data as DonationTestResult
}

export async function removeDonation(id: number) {
  const response = await api.post<ApiResponse<null>>(
    `/api/wt/donations/${id}/remove`
  )
  return requireServerSuccess(response.data)
}
