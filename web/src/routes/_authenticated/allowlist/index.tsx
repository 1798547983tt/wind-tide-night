/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { createFileRoute, redirect } from '@tanstack/react-router'

import { AllowlistPage } from '@/features/wind-tide/components/allowlist/allowlist-page'
import { allowlistSearchSchema } from '@/features/wind-tide/lib/allowlist'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

// wind-tide: the owner's allowlist and entry requests (wind-tide/DESIGN.md
// §3). The /api/wt owner routes are root-only, and so is this page.
export const Route = createFileRoute('/_authenticated/allowlist/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()

    if (!auth.user || auth.user.role < ROLE.SUPER_ADMIN) {
      throw redirect({
        to: '/403',
      })
    }
  },
  validateSearch: allowlistSearchSchema,
  component: AllowlistPage,
})
