/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useTranslation } from 'react-i18next'

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import {
  DONATION_TAB_LABELS,
  isDonationTab,
  type DonationTab,
} from '../../lib/donation-tabs'

type DonationTabsProps = {
  value: DonationTab
  /** The tabs the site offers, in order. */
  tabs: DonationTab[]
  onChange: (tab: DonationTab) => void
}

/**
 * The strip that switches between the pools (wind-tide/API.md §15). A site
 * with a single pool has nothing to switch to and shows none.
 */
export function DonationTabs(props: DonationTabsProps) {
  const { t } = useTranslation()
  if (props.tabs.length < 2) return null

  return (
    <Tabs
      value={props.value}
      onValueChange={(value) => {
        if (isDonationTab(value) && value !== props.value) props.onChange(value)
      }}
    >
      <TabsList aria-label={t('Pools')}>
        {props.tabs.map((tab) => (
          <TabsTrigger key={tab} value={tab}>
            {t(DONATION_TAB_LABELS[tab])}
          </TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  )
}
