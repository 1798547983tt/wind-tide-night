/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { getRouteApi } from '@tanstack/react-router'

import { useStatus } from '@/hooks/use-status'

import {
  enabledDonationTabs,
  resolveDonationTab,
  type DonationTab,
} from '../../lib/donation-tabs'
import { PoolPage } from '../pool/pool-page'
import { AisDonationPage } from './ais-donation-page'
import { DonationPage } from './donation-page'
import { DonationTabs } from './donation-tabs'

const route = getRouteApi('/_authenticated/donations/')

/**
 * The donated accounts page (wind-tide/API.md §15): the Antigravity pool,
 * the AI Studio pool, the BUILD pool and the compute pool dashboard, one per
 * tab. The tab lives in the address, so a link opens the same one; a tab the
 * site does not offer falls back to the first it does.
 */
export function DonationTabsPage() {
  const { status } = useStatus()
  const navigate = route.useNavigate()
  const requested = route.useSearch({ select: (search) => search.tab })
  const tabs = enabledDonationTabs(status)
  const tab = resolveDonationTab(requested, tabs)

  const show = (next: DonationTab) => {
    void navigate({ search: { tab: next } })
  }
  const strip = <DonationTabs value={tab} tabs={tabs} onChange={show} />

  if (tab === 'pool') return <PoolPage tabs={strip} />
  if (tab === 'aistudio' || tab === 'build') {
    return <AisDonationPage provider={tab} tabs={strip} />
  }
  return <DonationPage tabs={strip} />
}
