/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { createFileRoute } from '@tanstack/react-router'

import { DonationPage } from '@/features/wind-tide/components/donation/donation-page'

// wind-tide: members donate spare Antigravity accounts to the shared pool
// (wind-tide/API.md §14). The sidebar hides the entry when the site has no
// pool; a member opening the address by hand sees the server's refusal.
export const Route = createFileRoute('/_authenticated/donations/')({
  component: DonationPage,
})
