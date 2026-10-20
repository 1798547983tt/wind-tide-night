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

/**
 * The members' boards (wind-tide/API.md §7): successful and failed calls over
 * a period, the longest runs of each, and the balance of the moment.
 */
export type LeaderboardBoard =
  | 'calls'
  | 'failed'
  | 'success_streak'
  | 'fail_streak'
  | 'balance'

export type LeaderboardPeriod = 'today' | 'week' | 'all'

export type LeaderboardItem = {
  rank: number
  user_id: number
  display_name: string
  avatar_url: string
  /**
   * Calls on the calls and failed boards, the longest run on the streak
   * boards, raw quota on the balance board.
   */
  value: number
  /** On a streak board, the member's run right now; left out when it is 0. */
  current?: number
}

export type Leaderboard = {
  board: LeaderboardBoard
  /** Empty on the streak and balance boards, which have no period. */
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

// wind-tide: donated Antigravity accounts (wind-tide/API.md §14), donated
// AI Studio accounts, to the AIS playground pool or the BUILD pool (§15),
// and donated WorkBuddy accounts (§16). All four kinds share the donations
// list.

/** Which pool a donated account went to. */
export type DonationProvider =
  | 'antigravity'
  | 'aistudio'
  | 'build'
  | 'workbuddy'

/** The pools fed with the Cookie of a signed-in AI Studio page. */
export type CookieProvider = Exclude<
  DonationProvider,
  'antigravity' | 'workbuddy'
>

export type DonationStatus = 'verifying' | 'active' | 'failed'

/** The pool's live verdict on an active donation (wind-tide/API.md §14). */
export type PoolState =
  | 'ok'
  | 'cooldown'
  | 'token_invalid'
  | 'missing'
  | 'disabled'
  | 'unknown'

export type DonationPool = {
  state: PoolState
  /** CPA's status message, cut to 200 characters; may be empty. */
  message: string
  /** Unix seconds when a cooldown ends; 0 when the pool gave no time. */
  retry_at: number
  success: number
  failed: number
}

/** The AI Studio pool's verdict on an active donation (wind-tide/API.md §15). */
export type AisPoolState =
  | 'available'
  | 'exhausted'
  | 'cooldown'
  | 'expired'
  | 'missing'

/** One upstream model an AI Studio account cannot serve until `retry_at`. */
export type AisCoolingEntry = {
  model: string
  retry_at: number
}

export type AisDonationPool = {
  state: AisPoolState
  /** An English message key for the state. */
  message: string
  /** Unix seconds when a cooldown or the upstream quota day ends; 0 when none. */
  retry_at: number
  success: number
  failed: number
  /** Upstream requests on the pool's current quota day, all models together. */
  requests_today: number
  /** The models whose quota is used up, each with the end of its day. */
  cooling: AisCoolingEntry[]
}

export type DonationTest = {
  at: number
  ok: boolean
  latency_ms: number
  message: string
}

type DonationRecord = {
  id: number
  email: string
  /** What the account goes by; only a WorkBuddy account has one. */
  nickname?: string
  status: DonationStatus
  /** An English message key; empty unless the status is failed. */
  fail_reason: string
  /** Null until the first test. */
  last_test: DonationTest | null
  created_at: number
  verified_at: number
}

export type AntigravityDonation = DonationRecord & {
  provider: 'antigravity'
  /** Only an active donation has one. */
  pool: DonationPool | null
}

export type AisDonation = DonationRecord & {
  provider: 'aistudio'
  /** Empty until the pool has read the signed-in email. */
  email: string
  /** Only an active donation has one. */
  pool: AisDonationPool | null
}

/** The BUILD pool's verdict on an active donation (wind-tide/API.md §15). */
export type BuildPoolState = 'available' | 'expired' | 'invalid' | 'missing'

export type BuildDonationPool = {
  state: BuildPoolState
  /** An English message key for the state. */
  message: string
  /** The account's slot in the pool: the number of its sign-in file. */
  index: number
  /** Whether the pool is serving requests with this account right now. */
  current: boolean
  /** Whether the pool has a browser context open for this account. */
  has_context: boolean
}

export type BuildDonation = DonationRecord & {
  provider: 'build'
  /** Always empty: the BUILD pool never reads the account's email. */
  email: string
  /** Only an active donation has one. */
  pool: BuildDonationPool | null
}

/** The two sign-ins the WorkBuddy pool knows: CodeBuddy in China, WorkBuddy elsewhere. */
export type WbRealm = 'cn' | 'global'

/** The WorkBuddy pool's verdict on an active donation (wind-tide/API.md §16). */
export type WbPoolState =
  | 'available'
  | 'cooldown'
  | 'paused'
  | 'disabled'
  | 'missing'

export type WbDonationPool = {
  state: WbPoolState
  /** An English message key for the state. */
  message: string
  nickname: string
  realm: WbRealm | string
  /** Credits the account has left, against what it had. */
  credits: number
  credits_total: number
  /** Unix seconds when a cooldown ends; 0 when none. */
  retry_at: number
  success: number
  failed: number
  /** Whether the pool has checked the account in today. */
  checked_in: boolean
}

export type WbDonation = DonationRecord & {
  provider: 'workbuddy'
  /** The nickname Tencent gave the account; empty until the pool has it. */
  nickname: string
  /** Always empty: the WorkBuddy pool never reads the account's email. */
  email: string
  /** Only an active donation has one. */
  pool: WbDonationPool | null
}

export type Donation =
  | AntigravityDonation
  | AisDonation
  | BuildDonation
  | WbDonation

export type DonationList = {
  max: number
  items: Donation[]
}

export type DonationStart = {
  flow_token: string
  authorization_url: string
  expires_at: number
  email: string
}

export type DonationSubmitted = {
  id: number
  status: DonationStatus
}

/** A WorkBuddy donation the pool started: Tencent's link for the member to open. */
export type WbDonationStart = DonationSubmitted & {
  authorization_url: string
  /** Unix seconds; the pool forgets the sign-in after this. */
  expires_at: number
}

export type DonationProgress = {
  id: number
  email: string
  /** Only a WorkBuddy account has one. */
  nickname?: string
  status: DonationStatus
  fail_reason: string
}

export type UsageBucket = {
  name: string
  /** As Google names it: `5h` or `weekly` so far. */
  window: string
  /** 0 to 1. */
  remaining_fraction: number
  /** Unix seconds; 0 when Google gave none. */
  reset_at: number
}

export type UsageGroup = {
  name: string
  buckets: UsageBucket[]
}

export type DonationUsage = {
  fetched_at: number
  /** free, pro, ultra, ultra_lite or unknown. */
  tier: string
  groups: UsageGroup[]
}

export type DonationTestResult = {
  ok: boolean
  latency_ms: number
  message: string
  /** Null for an AI Studio account, whose pool reports no usage. */
  usage: DonationUsage | null
}

// wind-tide: the compute pool tab (wind-tide/API.md §15).

export type PoolBucketAverage = {
  name: string
  /** As Google names it: `5h` or `weekly` so far. */
  window: string
  /** 0 to 1, averaged over the accounts that answered. */
  average_remaining_fraction: number
}

/** The last reading of every Antigravity credential, taken every 5 minutes. */
export type AntigravityPoolLevel = {
  /** Unix seconds; 0 before the first reading. */
  fetched_at: number
  accounts: number
  healthy: number
  buckets: PoolBucketAverage[]
}

/** One pool model: the member's calls today and its public pool. */
export type AisModelPool = {
  /** The pool the model belongs to: the AIS playground or BUILD. */
  provider: CookieProvider
  /**
   * The ledger key the API takes: the upstream name, `gemini-4-argon`, in
   * the AIS playground pool, the relay name in the BUILD pool.
   */
  model: string
  /** What members call it: `AIS PG-gemini-4-argon`, `Build-gemini-3.8-flash`. */
  relay_model: string
  /** Calls a day each active account adds. */
  per_account: number
  /** The most calls a member may take from this pool in a day. */
  take_max: number
  allowance: number
  calls: number
  failed_today: number
  given: number
  taken: number
  remaining: number
  /** How many more calls the member may take from the pool today. */
  take_left: number
  /** What the public pool holds right now: given minus taken today. */
  pool_balance: number
  /** Everything members gave to the public pool today. */
  pool_given: number
  /** Everything members took from the public pool today. */
  pool_taken: number
}

/** The member's own calls today in one pool and its public pools, per model. */
export type AisPoolStatus = {
  provider: CookieProvider
  /** Beijing-time `YYYY-MM-DD`. */
  day: string
  /** The member's active accounts in this pool; 0 for a non-donor. */
  accounts: number
  /** In the site's order: argon first in the AIS playground. */
  models: AisModelPool[]
}

export type PoolEventKind =
  | 'antigravity_donated'
  | 'antigravity_removed'
  | 'ais_account_donated'
  | 'ais_account_removed'
  | 'ais_account_expired'
  | 'ais_given'
  | 'ais_taken'
  | 'ais_withdrawn'
  | 'ais_returned'
  | 'build_account_donated'
  | 'build_account_removed'
  | 'build_account_expired'
  | 'build_given'
  | 'build_taken'
  | 'build_withdrawn'
  | 'build_returned'
  | 'wb_account_donated'
  | 'wb_account_removed'

export type PoolEvent = {
  id: number
  kind: PoolEventKind
  provider: string
  /** The member's display name, else their username. */
  display_name: string
  /** Calls given or taken; 0 for account events. */
  amount: number
  /** The upstream model of a give or take; empty for account events. */
  model: string
  created_at: number
}

/**
 * The WorkBuddy pool's last reading (wind-tide/API.md §16): the accounts it
 * holds and the credits left across them. `fetched_at` is 0 while the pool
 * has not answered.
 */
export type WbPoolLevel = {
  fetched_at: number
  accounts: number
  healthy: number
  cooling: number
  credits: number
  credits_total: number
}

export type PoolOverview = {
  /** Null when the site has no Antigravity pool. */
  antigravity: AntigravityPoolLevel | null
  /** Null when the site has no AIS playground pool. */
  aistudio: AisPoolStatus | null
  /** Null when the site has no BUILD pool. */
  build: AisPoolStatus | null
  /** Null when the site has no WorkBuddy pool. */
  workbuddy: WbPoolLevel | null
  /** The latest 50, newest first. */
  events: PoolEvent[]
}

export type PoolEventList = {
  items: PoolEvent[]
}

/** What a give or take asks for: the upstream model and the calls. */
export type AisTransferInput = {
  model: string
  count: number
}

export type AisGiveResult = {
  model: string
  remaining: number
  pool_balance: number
}

export type AisTakeResult = AisGiveResult & {
  take_left: number
}
