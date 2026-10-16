/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { useOwnerViewStore } from '@/stores/owner-view-store'

/** A thin strip that reminds the owner they are in the members' view. */
export function MemberViewBanner() {
  const { t } = useTranslation()
  const isOwner = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const memberView = useOwnerViewStore((state) => state.view === 'member')
  const setView = useOwnerViewStore((state) => state.setView)

  if (!isOwner || !memberView) return null

  return (
    <div
      role='status'
      className='bg-muted/60 text-muted-foreground flex shrink-0 items-center justify-center gap-3 border-b px-4 py-1.5 text-xs'
    >
      <span>{t('You are browsing as a member')}</span>
      <Button
        variant='link'
        size='sm'
        className='h-auto p-0 text-xs'
        onClick={() => setView('admin')}
      >
        {t('Back to admin view')}
      </Button>
    </div>
  )
}
