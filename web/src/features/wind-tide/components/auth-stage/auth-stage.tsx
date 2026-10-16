/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { useTheme } from '@/context/theme-provider'
import { cn } from '@/lib/utils'

import { artManifestQueryOptions } from '../../api'
import { BRAND } from '../../constants'
import { ArtPicture } from '../art-picture'
import { BrandMark } from '../brand-mark'
import { DriftParticles } from '../drift-particles'
import { SealStamp } from '../seal-stamp'
import { SiteAttribution } from '../site-footer'
import { VerticalTitle } from '../vertical-title'
import { WaveEdge } from '../wave-edge'
import { WindLines } from '../wind-lines'

export type AuthPainting = 'welcome' | 'tea'

type Painting = {
  name: string
  /** Keeps her face in a narrow crop. */
  position: string
}

const PAINTINGS: Record<AuthPainting, Painting> = {
  welcome: { name: 'auth-welcome', position: '44% 20%' },
  tea: { name: 'auth-tea', position: '58% 22%' },
}

// Stands in while a deployment has not uploaded the sign-in paintings yet.
const LANDING_PAINTING: Painting = { name: 'hero-day', position: '55% 30%' }

type AuthStageProps = {
  children: React.ReactNode
  /** The painting beside the page: the welcome one unless a page picks. */
  painting?: AuthPainting
}

/**
 * The sign-in pages' stage: a painting on one side, re-lit as a moonlit scene
 * at night, and the page itself on paper beside it, with the tide line
 * between them. On a phone the painting becomes a band above the page.
 */
export function AuthStage(props: AuthStageProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const manifest = useQuery(artManifestQueryOptions)
  const night = resolvedTheme === 'dark'
  const choice = props.painting ?? 'welcome'

  let painting: Painting | null = PAINTINGS[choice]
  if (manifest.isError) painting = null
  if (manifest.data && !manifest.data[PAINTINGS[choice].name]) {
    painting = manifest.data[LANDING_PAINTING.name] ? LANDING_PAINTING : null
  }

  let alt = t(
    'Cecilia crouching with a flower on her blouse, smiling up at you'
  )
  if (painting === LANDING_PAINTING) {
    alt = t('A girl in a straw hat looking out over the summer sea')
  } else if (choice === 'tea') {
    alt = t('Cecilia holding a teacup and smiling up at the light')
  }
  // The calligraphy goes on the side her face leaves free.
  const titleOnLeft = painting === PAINTINGS.tea

  return (
    <div className='bg-background text-foreground relative min-h-svh lg:grid lg:grid-cols-[minmax(0,1.1fr)_minmax(30rem,0.9fr)]'>
      <div className='wt-grain relative h-[44svh] min-h-64 overflow-hidden lg:sticky lg:top-0 lg:h-svh'>
        {painting ? (
          <ArtPicture
            name={painting.name}
            alt={alt}
            sizes='(min-width: 1024px) 56vw, 100vw'
            priority
            className='absolute inset-0 -z-20'
            objectPosition={painting.position}
            imgClassName={cn(
              'transition-[filter] duration-700',
              night && 'brightness-[0.6] saturate-[0.7]'
            )}
          />
        ) : (
          <div
            aria-hidden='true'
            className='from-wt-sakura/45 via-wt-paper to-wt-tide/35 absolute inset-0 -z-20 bg-linear-to-br dark:from-[#1b2653] dark:via-[#0a0f24] dark:to-[#0d3b45]'
          />
        )}
        {night && (
          <div
            aria-hidden='true'
            className='wt-moonwash pointer-events-none absolute inset-0 -z-10'
          />
        )}
        {/* Keeps the home link legible over the painting. */}
        <div
          aria-hidden='true'
          className='from-background/75 pointer-events-none absolute inset-x-0 top-0 -z-10 h-32 bg-linear-to-b to-transparent'
        />

        <WindLines className={night ? 'text-wt-tide/40' : 'text-white/80'} />
        <DriftParticles
          kind={night ? 'fireflies' : 'petals'}
          density={night ? 0.6 : 0.45}
        />

        {/* The home link already names the site; this is the painting's
            calligraphy. On the left of the short band above a page it would
            stack right under the home link, so there it waits for wide
            screens. */}
        <div
          aria-hidden='true'
          className={cn(
            'absolute top-16 flex items-start gap-3 sm:top-20 lg:top-28',
            titleOnLeft
              ? 'hidden lg:left-12 lg:flex'
              : 'right-5 sm:right-8 lg:right-24'
          )}
        >
          <p className='wt-vertical font-mincho text-foreground/70 mt-16 hidden text-xs tracking-[0.45em] lg:block'>
            {BRAND.welcomeJa}
          </p>
          <div className='relative'>
            <VerticalTitle
              as='p'
              text={BRAND.name}
              delay={120}
              stagger={160}
              className={cn(
                'text-[2.1rem] sm:text-[2.6rem] lg:text-[clamp(3.6rem,min(6vw,11vh),6rem)]',
                night
                  ? 'text-wt-moon [text-shadow:0_0_28px_var(--wt-glow)]'
                  : 'text-wt-ink [text-shadow:0_0_24px_var(--wt-glow)]'
              )}
            />
            <SealStamp className='absolute -bottom-10 -left-6 size-9 lg:-bottom-12 lg:-left-8 lg:size-11' />
          </div>
        </div>

        {/* Above the grain: the tide is the page's own paper, so it has to
            meet the page without a seam. */}
        <WaveEdge className='absolute inset-x-0 -bottom-px z-[2] lg:hidden' />
        <WaveEdge
          orientation='vertical'
          className='absolute inset-y-0 -right-px z-[2] hidden lg:block'
        />
      </div>

      <div className='relative flex flex-col px-5 pb-6 sm:px-10 lg:min-h-svh lg:px-14 xl:px-20'>
        <main className='flex flex-1 items-center py-6 lg:py-16'>
          <div className='mx-auto w-full max-w-md'>{props.children}</div>
        </main>
        <SiteAttribution className='text-muted-foreground justify-center text-[0.7rem] leading-relaxed lg:justify-start' />
      </div>

      <Link
        to='/'
        className='absolute top-4 left-4 z-10 flex items-center gap-2.5 transition-opacity hover:opacity-80 sm:top-6 sm:left-6 lg:top-8 lg:left-8'
      >
        <BrandMark className='size-8' />
        <h1 className='font-brush text-2xl leading-none'>{BRAND.name}</h1>
      </Link>
    </div>
  )
}
