/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { useOwnerViewStore } from '@/stores/owner-view-store'

/**
 * The role the console is shown for: the signed-in user's own role, except
 * that the owner (super admin) in member view sees what a member sees. The
 * server still checks the real role, so this only shapes the interface.
 */
export function useViewRole(): number {
  const role = useAuthStore((state) => state.auth.user?.role ?? ROLE.GUEST)
  const memberView = useOwnerViewStore((state) => state.view === 'member')
  return memberView && role === ROLE.SUPER_ADMIN ? ROLE.USER : role
}
