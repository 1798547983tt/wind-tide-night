/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { PublicLayout } from '@/components/layout'
import { useAuthStore } from '@/stores/auth-store'

import { BRAND } from '../../constants'
import { BrandMark } from '../brand-mark'
import { OpeningOverlay } from '../opening/opening-overlay'
import { SiteFooter } from '../site-footer'
import { LandingCoda } from './landing-coda'
import { LandingHero } from './landing-hero'
import { SceneGallery } from './scene-gallery'
import { ThreeActs } from './three-acts'
import { TideTable } from './tide-table'

/** The front door: the opening, then the landing page below it. */
export function LandingPage() {
  const isAuthenticated = useAuthStore((state) => !!state.auth.user)

  return (
    <PublicLayout
      showMainContainer={false}
      logo={<BrandMark className='size-7' />}
      siteName={BRAND.name}
      headerProps={{ showLanguageSwitcher: false }}
    >
      <OpeningOverlay />
      <main>
        <LandingHero isAuthenticated={isAuthenticated} />
        <TideTable />
        <ThreeActs />
        <SceneGallery />
        <LandingCoda isAuthenticated={isAuthenticated} />
      </main>
      <SiteFooter />
    </PublicLayout>
  )
}
