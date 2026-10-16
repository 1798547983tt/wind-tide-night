/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/

import type {
  CheckinResponse,
  CheckinStatusResponse,
} from '@/features/profile/types'

// Shapes of the fork's /api/wt endpoints, as wind-tide/API.md defines them.
// Money fields are raw quota; the console converts them with quota_per_unit.

export type ApiResponse<T = unknown> = {
  success: boolean
  message?: string
  data?: T
}

export type PageData<T> = {
  page: number
  page_size: number
  total: number
  items: T[]
}

export type PublicStats = {
  members: number
  calls_today: number
  rewarded_quota_total: number
}

/**
 * One month of the member's check-ins (wind-tide/API.md §6). Days are
 * Beijing-time `YYYY-MM-DD` strings, whatever the browser's time zone.
 */
export type CheckinMonth = CheckinStatusResponse & {
  /** The day a check-in made now counts for. */
  today: string
  checked_in_today: boolean
  /** Days in a row up to today, or up to yesterday while today is open. */
  streak: number
  /** Quota one check-in awards. */
  reward_quota: number
}

export type CheckinResult = CheckinResponse & {
  checkin_date: string
  /** The streak including this check-in. */
  streak: number
}

export type LeaderboardBoard = 'calls' | 'balance'

export type LeaderboardPeriod = 'today' | 'week' | 'all'

export type LeaderboardItem = {
  rank: number
  user_id: number
  display_name: string
  avatar_url: string
  /** Successful calls on the calls board, raw quota on the balance board. */
  value: number
}

export type Leaderboard = {
  board: LeaderboardBoard
  /** Empty on the balance board, which has no period. */
  period: LeaderboardPeriod | ''
  items: LeaderboardItem[]
  /** The requester's own place; null for the owner, who is not ranked. */
  me: { rank: number; value: number } | null
  updated_at: number
}

export type AllowlistStatus = 'active' | 'removed'

/** State of the account an entry admits; empty when there is no account yet. */
export type AllowlistAccountStatus = 'enabled' | 'disabled' | 'deleted' | ''

export type AllowlistEntry = {
  id: number
  /** Empty while the entry only names a username nobody has signed in with. */
  discord_id: string
  discord_username: string
  note: string
  status: AllowlistStatus
  /**
   * The account the entry admits: the linked one, or before the first sign-in
   * an account that already holds the Discord ID. 0 when there is none or it
   * was deleted.
   */
  user_id: number
  account_status: AllowlistAccountStatus
  display_name: string
  avatar_url: string
  created_at: number
  updated_at: number
}

export type AllowlistEntryInput = {
  discord_id: string
  note?: string
}

export type EntryRequestStatus = 'pending' | 'approved' | 'rejected'

export type EntryRequest = {
  id: number
  discord_id: string
  discord_username: string
  display_name: string
  avatar_url: string
  status: EntryRequestStatus
  requested_at: number
  decided_at: number
}

/** Why a Discord sign-in was turned away without a session. */
export type GateState = 'pending' | 'rejected' | 'removed'

export type GateRefusal = {
  wt_gate: GateState
  applicant: {
    discord_id: string
    username: string
    display_name: string
    avatar_url: string
  }
  requested_at: number
  /** Unix seconds when a rejected applicant may ask again; 0 otherwise. */
  retry_after: number
}
