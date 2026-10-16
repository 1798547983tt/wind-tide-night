/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useNavigate } from '@tanstack/react-router'
import { ShieldCheck, User } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'
import { useOwnerViewStore } from '@/stores/owner-view-store'

/** The owner's switch between the admin console and the members' view. */
export function OwnerViewSwitch() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isOwner = useAuthStore(
    (state) => state.auth.user?.role === ROLE.SUPER_ADMIN
  )
  const view = useOwnerViewStore((state) => state.view)
  const setView = useOwnerViewStore((state) => state.setView)

  if (!isOwner) return null

  return (
    <ToggleGroup
      value={[view]}
      onValueChange={(values) => {
        if (values[0] === 'admin') setView('admin')
        if (values[0] === 'member') {
          setView('member')
          // Admin pages have no place in the members' view.
          void navigate({ to: '/dashboard' })
        }
      }}
      variant='outline'
      size='sm'
      aria-label={t('Console view')}
    >
      {/* Phone headers have room for the icons only. */}
      <ToggleGroupItem value='admin' aria-label={t('Admin view')}>
        <ShieldCheck aria-hidden='true' />
        <span className='hidden sm:inline'>{t('Admin view')}</span>
      </ToggleGroupItem>
      <ToggleGroupItem value='member' aria-label={t('Member view')}>
        <User aria-hidden='true' />
        <span className='hidden sm:inline'>{t('Member view')}</span>
      </ToggleGroupItem>
    </ToggleGroup>
  )
}
