/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { createFileRoute, redirect } from '@tanstack/react-router'

import { GatePage } from '@/features/wind-tide/components/gate/gate-page'
import { useGateStore } from '@/stores/gate-store'

export const Route = createFileRoute('/(auth)/gate')({
  component: GatePage,
  // Only a Discord sign-in the gate turned away has anything to show here.
  beforeLoad: () => {
    if (!useGateStore.getState().refusal) {
      throw redirect({ to: '/sign-in', replace: true })
    }
  },
})
