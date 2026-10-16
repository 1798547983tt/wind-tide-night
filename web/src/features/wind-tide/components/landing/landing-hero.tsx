/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { IconDiscord } from '@/assets/brand-icons'
import { Button } from '@/components/ui/button'
import { useTheme } from '@/context/theme-provider'
import { useMediaQuery } from '@/hooks/use-media-query'
import { cn } from '@/lib/utils'
import { useOpeningStore } from '@/stores/opening-store'

import { artManifestQueryOptions } from '../../api'
import { BRAND } from '../../constants'
import { artFile, coverWidth, pickArtUrl } from '../../lib/art'
import { DepthPainting } from '../depth-painting'
import { DriftParticles } from '../drift-particles'
import { SealStamp } from '../seal-stamp'
import { VerticalTitle } from '../vertical-title'
import { WindLines } from '../wind-lines'

type LandingHeroProps = {
  isAuthenticated: boolean
}

// Where her face and the horizon sit in 11.png, for a manifest that does not
// say where they sit in the painting it lists.
const HERO_FOCUS: [number, number] = [0.55, 0.3]
const HORIZON = 0.9
// The re-lit night's moon, on screen: by the hat brim on wide screens, clear
// of the title on the right; behind the hat on narrow ones, where only its
// glow shows past the brim.
const MOON_WIDE: [number, number] = [0.085, 0.24]
const MOON_NARROW: [number, number] = [0.12, 0.1]

function entranceClass(ready: boolean) {
  return ready ? 'landing-animate-fade-up' : 'opacity-0'
}

export function LandingHero(props: LandingHeroProps) {
  const { t } = useTranslation()
  const { resolvedTheme } = useTheme()
  const ready = useOpeningStore((state) => state.stage !== 'playing')
  const manifest = useQuery(artManifestQueryOptions)
  const wide = useMediaQuery('(min-width: 1024px)')
  const night = resolvedTheme === 'dark'

  // A dedicated night painting wins when it exists; until then the day
  // painting is re-lit as a moonlit night in the shader.
  const nightEntry = manifest.data?.['hero-night']
  const name = night && nightEntry ? 'hero-night' : 'hero-day'
  const entry = manifest.data?.[name]

  return (
    <section className='wt-grain relative isolate flex min-h-svh overflow-hidden'>
      {!manifest.isLoading && (
        <DepthPainting
          key={name}
          imageUrl={pickArtUrl(name, entry, coverWidth(entry))}
          depthUrl={artFile(name, 'depth')}
          fallbackSrc={artFile(name, entry?.sizes.at(-1) ?? 'full')}
          alt={
            night
              ? t('A girl in a straw hat looking out over the moonlit sea')
              : t('A girl in a straw hat looking out over the summer sea')
          }
          focus={entry?.focus ?? HERO_FOCUS}
          parallax={0.016}
          focalDepth={0.55}
          night={night && name === 'hero-day'}
          horizon={entry?.horizon ?? HORIZON}
          moon={wide ? MOON_WIDE : MOON_NARROW}
          className='absolute inset-0 -z-20'
        />
      )}

      {/* Scrims keep the header and the copy legible over the painting. */}
      <div
        aria-hidden='true'
        className='from-background/70 pointer-events-none absolute inset-x-0 top-0 -z-10 h-40 bg-linear-to-b to-transparent'
      />
      <div
        aria-hidden='true'
        className='wt-hero-scrim pointer-events-none absolute inset-0 -z-10'
      />

      <WindLines className={night ? 'text-wt-tide/55' : 'text-white/85'} />
      <DriftParticles
        kind={night ? 'fireflies' : 'petals'}
        density={night ? 0.8 : 0.7}
      />

      <div className='relative mx-auto flex w-full max-w-7xl flex-col justify-end px-5 pt-28 pb-[max(2.75rem,env(safe-area-inset-bottom))] sm:px-8 lg:pb-16'>
        {/* Calligraphy title, right side, read top to bottom. */}
        <div className='absolute top-24 right-4 flex items-start gap-3 sm:top-28 sm:right-10 sm:gap-5 lg:right-16'>
          <p
            className={cn(
              'wt-vertical font-mincho mt-14 text-[0.7rem] tracking-[0.5em] sm:mt-20 sm:text-sm',
              night ? 'text-wt-moon/75' : 'text-wt-ink/70',
              entranceClass(ready)
            )}
            style={{ animationDelay: '900ms' }}
          >
            {BRAND.welcomeJa}
          </p>
          <div className='relative'>
            <VerticalTitle
              text={BRAND.name}
              play={ready}
              delay={150}
              stagger={200}
              className={cn(
                // Five characters tall: bounded by the viewport height too,
                // so the seal under the title stays on screen.
                'text-[clamp(3.6rem,15vw,5.5rem)] sm:text-[clamp(4.5rem,min(10vw,12.5vh),9.5rem)]',
                night
                  ? 'text-wt-moon [text-shadow:0_0_32px_var(--wt-glow)]'
                  : 'text-wt-ink [text-shadow:0_0_28px_var(--wt-glow)]'
              )}
            />
            <SealStamp
              className={cn(
                'absolute -bottom-12 -left-7 size-10 sm:-bottom-14 sm:-left-9 sm:size-12',
                entranceClass(ready)
              )}
            />
          </div>
        </div>

        <div className='max-w-xl'>
          <p
            className={cn(
              'font-display text-sm font-medium tracking-[0.32em] uppercase italic sm:text-base',
              night ? 'text-wt-tide' : 'text-primary',
              entranceClass(ready)
            )}
            style={{ animationDelay: '250ms' }}
          >
            {BRAND.nameEn}
          </p>
          <p
            className={cn(
              'font-mincho mt-4 text-[clamp(1.65rem,4.6vw,2.9rem)] leading-[1.35] font-semibold whitespace-pre-line',
              entranceClass(ready)
            )}
            style={{ animationDelay: '400ms' }}
          >
            {t(
              'The wind carries the question away,\nthe tide brings the answer back.'
            )}
          </p>
          <p
            className={cn(
              'text-muted-foreground mt-5 max-w-md text-sm leading-relaxed sm:text-base',
              entranceClass(ready)
            )}
            style={{ animationDelay: '560ms' }}
          >
            {t(
              'A free AI API relay open only to invited members. Check in every day to receive $9,999 in balance.'
            )}
          </p>
          <div
            className={cn(
              'mt-8 flex flex-wrap items-center gap-3',
              entranceClass(ready)
            )}
            style={{ animationDelay: '720ms' }}
          >
            {props.isAuthenticated ? (
              <>
                <Button
                  size='lg'
                  className='h-11 rounded-full px-6'
                  render={<Link to='/dashboard' />}
                >
                  {t('Enter console')}
                </Button>
                <Button
                  size='lg'
                  variant='outline'
                  className='bg-background/40 h-11 rounded-full px-6 backdrop-blur'
                  render={<Link to='/wallet' />}
                >
                  {t('Check in today')}
                </Button>
              </>
            ) : (
              <>
                <Button
                  size='lg'
                  className='h-11 rounded-full px-6'
                  render={<Link to='/sign-in' />}
                >
                  <IconDiscord className='size-4' />
                  {t('Sign in with Discord')}
                </Button>
                <Button
                  size='lg'
                  variant='ghost'
                  className='h-11 rounded-full px-5'
                  render={<Link to='/about' />}
                >
                  {t('About this site')}
                </Button>
              </>
            )}
          </div>
        </div>

        <div
          aria-hidden='true'
          className='text-muted-foreground absolute right-8 bottom-10 hidden flex-col items-center gap-3 lg:flex'
        >
          <span className='wt-vertical font-mincho text-[0.7rem] tracking-[0.5em]'>
            {t('Tide table')}
          </span>
          <span className='relative h-16 w-px overflow-hidden bg-current/20'>
            <span className='wt-scroll-cue absolute inset-x-0 top-0 h-1/2 bg-current' />
          </span>
        </div>
      </div>
    </section>
  )
}
