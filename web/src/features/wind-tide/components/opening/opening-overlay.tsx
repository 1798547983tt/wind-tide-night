/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { useReducedMotion } from 'motion/react'
import { useCallback, useEffect, useState, type CSSProperties } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useTheme } from '@/context/theme-provider'
import { useMediaQuery } from '@/hooks/use-media-query'
import { cn } from '@/lib/utils'
import { useOpeningStore } from '@/stores/opening-store'

import { artManifestQueryOptions } from '../../api'
import { BRAND } from '../../constants'
import { artFile, coverWidth, pickArtUrl, stageArtName } from '../../lib/art'
import { DepthPainting } from '../depth-painting'
import { DriftParticles } from '../drift-particles'
import { VerticalTitle } from '../vertical-title'
import { StageCurtain } from './stage-curtain'
import { TideFront } from './tide-front'

/**
 * title   — a crescent is drawn and the calligraphy title soaks in.
 * curtain — the curtains part on the painting.
 * push    — the camera pushes in toward the two faces.
 * tide    — a wave rises and washes the theatre away.
 */
type Phase = 'title' | 'curtain' | 'push' | 'tide'

// Milliseconds from the start, plus guards for slow networks.
const AT = { curtain: 1900, push: 3500, tide: 4500 }
const SKIP_AFTER = 1000
const ART_WAIT_LIMIT = 4500
const TIDE_MS = 1100
const QUIET_MS = 1000

// Where the stage and the two faces sit in 1.png, for a manifest that does
// not say where they sit in the painting it lists.
const STAGE_FOCUS: [number, number] = [0.5, 0.27]
const FACES_FOCUS: [number, number] = [0.49, 0.235]

export function OpeningOverlay() {
  const stage = useOpeningStore((state) => state.stage)
  const reducedMotion = !!useReducedMotion()
  if (stage === 'done') return null
  return reducedMotion ? <QuietOpening /> : <TheatreOpening />
}

/** The opening painting for the time of day and the screen's shape. */
function useStageArt() {
  const manifest = useQuery(artManifestQueryOptions)
  const { resolvedTheme } = useTheme()
  const wide = useMediaQuery('(min-aspect-ratio: 1/1)')
  const night = resolvedTheme === 'dark'
  const name = stageArtName(manifest.data, night, wide)
  const entry = manifest.data?.[name]
  // Allow for part of the push-in; the camera is moving by then, which
  // hides the rest, and the opening should not wait on a larger file.
  const needed = coverWidth(entry) * 1.3
  return {
    settled: !manifest.isLoading,
    night,
    image: pickArtUrl(name, entry, needed),
    depth: artFile(name, 'depth'),
    fallback: artFile(name, entry?.sizes.at(-1) ?? 'full'),
    focus: entry?.focus ?? STAGE_FOCUS,
    faces: entry?.faces ?? FACES_FOCUS,
  }
}

function useScrollLock() {
  useEffect(() => {
    const root = document.documentElement
    const previous = root.style.overflow
    root.style.overflow = 'hidden'
    return () => {
      root.style.overflow = previous
    }
  }, [])
}

function TheatreOpening() {
  const { t } = useTranslation()
  const wash = useOpeningStore((state) => state.wash)
  const finish = useOpeningStore((state) => state.finish)
  const art = useStageArt()
  const [phase, setPhase] = useState<Phase>('title')
  const [titleDone, setTitleDone] = useState(false)
  const [artReady, setArtReady] = useState(false)
  const [artTimedOut, setArtTimedOut] = useState(false)
  const [canSkip, setCanSkip] = useState(false)

  useScrollLock()

  useEffect(() => {
    const timers = [
      window.setTimeout(() => setCanSkip(true), SKIP_AFTER),
      window.setTimeout(() => setTitleDone(true), AT.curtain),
      window.setTimeout(() => setArtTimedOut(true), ART_WAIT_LIMIT),
    ]
    return () => timers.forEach((timer) => window.clearTimeout(timer))
  }, [])

  // The curtain waits for the painting, but never longer than the limit.
  useEffect(() => {
    if (phase === 'title' && titleDone && (artReady || artTimedOut)) {
      setPhase('curtain')
    }
  }, [phase, titleDone, artReady, artTimedOut])

  useEffect(() => {
    let timer = 0
    if (phase === 'curtain') {
      timer = window.setTimeout(() => setPhase('push'), AT.push - AT.curtain)
    }
    if (phase === 'push') {
      timer = window.setTimeout(() => setPhase('tide'), AT.tide - AT.push)
    }
    if (phase === 'tide') {
      wash()
      timer = window.setTimeout(finish, TIDE_MS + 80)
    }
    return () => window.clearTimeout(timer)
  }, [phase, wash, finish])

  const skip = useCallback(() => setPhase('tide'), [])

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && canSkip) skip()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [canSkip, skip])

  const opened = phase !== 'title'
  const pushed = phase === 'push' || phase === 'tide'
  let zoom = 1.06
  if (phase === 'curtain') zoom = 1
  if (pushed) zoom = 1.6
  // By moonlight the painting's own window lights the room; the warm beams
  // only haze it.
  let beams = 'opacity-0'
  if (opened) beams = art.night ? 'opacity-40' : 'opacity-100'

  return (
    <div
      className='fixed inset-0 z-[100] overflow-hidden'
      style={{ '--wt-tide-duration': `${TIDE_MS}ms` } as CSSProperties}
    >
      <div
        className='wt-tide-mask wt-grain absolute inset-0 bg-[#13040a]'
        data-tide={phase === 'tide' ? 'out' : undefined}
      >
        {art.settled && (
          <DepthPainting
            imageUrl={art.image}
            depthUrl={art.depth}
            fallbackSrc={art.fallback}
            alt={
              art.night
                ? t(
                    'Two girls taking afternoon tea in a rose-coloured library by moonlight'
                  )
                : t('Two girls taking afternoon tea in a rose-coloured library')
            }
            focus={pushed ? art.faces : art.focus}
            zoom={zoom}
            parallax={0.022}
            focalDepth={0.62}
            rays={art.night ? 0.35 : 0.9}
            rayOrigin={[0.12, -0.08]}
            className='absolute inset-0'
            onReady={() => setArtReady(true)}
          />
        )}
        <div
          aria-hidden='true'
          className={cn(
            'wt-beams pointer-events-none absolute inset-0 transition-opacity duration-1000',
            beams
          )}
        />
        {opened && <DriftParticles kind='dust' density={0.9} />}
        <StageCurtain open={opened} lift={pushed} />

        <div
          className={cn(
            'absolute inset-0 flex items-center justify-center transition-[opacity,filter] duration-700',
            opened && 'pointer-events-none opacity-0 blur-sm'
          )}
        >
          <div className='relative flex items-start gap-5 text-[#fff1dc] sm:gap-8'>
            {/* The crescent heads the welcome line, clear of the valance. */}
            <div className='mt-4 flex flex-col items-center gap-5'>
              <svg
                aria-hidden='true'
                viewBox='0 0 80 80'
                className='size-10 sm:size-12'
              >
                <path
                  className='wt-crescent-path'
                  pathLength={1}
                  d='M38.7 10.03A30 30 0 1 0 69.24 46.69A24 24 0 1 1 38.7 10.03Z'
                  fill='none'
                  stroke='#e8c37a'
                  strokeWidth={2.4}
                />
              </svg>
              <p className='wt-vertical font-mincho text-sm tracking-[0.55em] text-[#f3d9b4]/80 sm:text-base'>
                {BRAND.welcomeJa}
              </p>
            </div>
            <VerticalTitle
              text={BRAND.name}
              delay={260}
              stagger={230}
              className='text-[clamp(4.25rem,14vh,9.5rem)] drop-shadow-[0_0_28px_rgba(255,190,140,0.35)]'
            />
          </div>
        </div>
      </div>

      <TideFront active={phase === 'tide'} />

      <Button
        type='button'
        variant='ghost'
        size='sm'
        onClick={skip}
        className={cn(
          'absolute right-5 bottom-[max(1.25rem,env(safe-area-inset-bottom))] z-10 rounded-full border border-white/25 bg-black/25 px-4 text-xs tracking-[0.3em] text-white/85 backdrop-blur hover:bg-black/40 hover:text-white',
          'transition-opacity duration-500',
          canSkip && phase !== 'tide'
            ? 'opacity-100'
            : 'pointer-events-none opacity-0'
        )}
      >
        {t('Skip opening')}
      </Button>
    </div>
  )
}

/** Reduced motion: the stage simply fades in with its title and out again. */
function QuietOpening() {
  const finish = useOpeningStore((state) => state.finish)
  const art = useStageArt()
  const [leaving, setLeaving] = useState(false)

  useScrollLock()

  useEffect(() => {
    const leave = window.setTimeout(() => setLeaving(true), QUIET_MS)
    const end = window.setTimeout(finish, QUIET_MS + 400)
    return () => {
      window.clearTimeout(leave)
      window.clearTimeout(end)
    }
  }, [finish])

  return (
    <div
      className={cn(
        'fixed inset-0 z-[100] bg-[#13040a] transition-opacity duration-400',
        leaving && 'opacity-0'
      )}
    >
      <img
        src={art.fallback}
        alt=''
        className='absolute inset-0 size-full object-cover opacity-60'
        style={{
          objectPosition: `${art.focus[0] * 100}% ${art.focus[1] * 100}%`,
        }}
      />
      <div className='absolute inset-0 flex items-center justify-center text-[#fff1dc]'>
        <h1 className='wt-vertical font-brush text-[clamp(4rem,13vh,9rem)] leading-none'>
          {BRAND.name}
        </h1>
      </div>
    </div>
  )
}
