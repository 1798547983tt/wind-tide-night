/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { z } from 'zod'

import type { SystemStatus } from '@/features/auth/types'

// The tabs of the donated accounts page (wind-tide/API.md §15): the
// Antigravity pool, the AI Studio pool, the BUILD pool and the compute pool
// dashboard.

export const DONATION_TABS = [
  'antigravity',
  'aistudio',
  'build',
  'pool',
] as const

export type DonationTab = (typeof DONATION_TABS)[number]

/** URL state of the /donations page; absent means the first enabled tab. */
export const donationsSearchSchema = z.object({
  tab: z.enum(DONATION_TABS).optional().catch(undefined),
})

/** Tab names; i18n keys. */
export const DONATION_TAB_LABELS: Record<DonationTab, string> = {
  antigravity: 'Antigravity',
  aistudio: 'AIS playground',
  build: 'BUILD',
  pool: 'Compute pool',
}

export function isDonationTab(value: unknown): value is DonationTab {
  return (
    typeof value === 'string' &&
    (DONATION_TABS as readonly string[]).includes(value)
  )
}

type PoolFlags = Pick<
  SystemStatus,
  'wt_donation_enabled' | 'wt_ais_enabled' | 'wt_build_enabled'
>

/**
 * The tabs a site offers, in display order. The pool dashboard needs at least
 * one pool. A site with none still shows the Antigravity tab, so a member who
 * opens the address by hand sees the server's refusal as before.
 */
export function enabledDonationTabs(
  status: PoolFlags | null | undefined
): DonationTab[] {
  const tabs: DonationTab[] = []
  if (status?.wt_donation_enabled === true) tabs.push('antigravity')
  if (status?.wt_ais_enabled === true) tabs.push('aistudio')
  if (status?.wt_build_enabled === true) tabs.push('build')
  if (tabs.length === 0) return ['antigravity']
  tabs.push('pool')
  return tabs
}

/** The tab to show: the one asked for when it is offered, else the first. */
export function resolveDonationTab(
  requested: DonationTab | undefined,
  tabs: DonationTab[]
): DonationTab {
  if (requested && tabs.includes(requested)) return requested
  return tabs[0] ?? 'antigravity'
}
