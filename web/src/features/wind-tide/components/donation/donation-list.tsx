/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import type { Donation } from '../../types'
import { DonationCard } from './donation-card'

type DonationListProps = {
  items: Donation[]
  onReauthorize: (donation: Donation) => void
}

/** At most ten cards, newest first as the server orders them. */
export function DonationList(props: DonationListProps) {
  return (
    <div className='grid gap-4 md:grid-cols-2'>
      {props.items.map((donation) => (
        <DonationCard
          key={donation.id}
          donation={donation}
          onReauthorize={props.onReauthorize}
        />
      ))}
    </div>
  )
}
