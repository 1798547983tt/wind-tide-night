/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { motion, useReducedMotion, useScroll, useTransform } from 'motion/react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useMediaQuery } from '@/hooks/use-media-query'
import { cn } from '@/lib/utils'

import { ArtPicture } from '../art-picture'

const SCENES = [
  'gallery-5',
  'gallery-6',
  'gallery-7',
  'gallery-8',
  'gallery-10',
]

/**
 * 风物诗 — a strip of illustrations. On large screens the section pins and
 * vertical scrolling slides the strip sideways; elsewhere it is a plain
 * horizontally scrolling row.
 */
export function SceneGallery() {
  const { t } = useTranslation()
  const sectionRef = useRef<HTMLElement>(null)
  const trackRef = useRef<HTMLDivElement>(null)
  const [distance, setDistance] = useState(0)
  const reducedMotion = !!useReducedMotion()
  const wide = useMediaQuery('(min-width: 1024px)')
  const pinned = wide && !reducedMotion

  const { scrollYProgress } = useScroll({
    target: sectionRef,
    offset: ['start start', 'end end'],
  })
  const x = useTransform(scrollYProgress, [0, 1], [0, -distance])

  useEffect(() => {
    const track = trackRef.current
    if (!track || !pinned) return
    const measure = () =>
      setDistance(Math.max(0, track.scrollWidth - window.innerWidth))
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(track)
    window.addEventListener('resize', measure)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', measure)
    }
  }, [pinned])

  return (
    <section
      ref={sectionRef}
      className='relative'
      style={pinned ? { height: `calc(100svh + ${distance}px)` } : undefined}
    >
      <div
        className={cn(
          pinned
            ? 'sticky top-0 flex h-svh flex-col justify-center overflow-hidden'
            : 'py-20'
        )}
      >
        <div className='mx-auto mb-10 flex w-full max-w-6xl flex-wrap items-end justify-between gap-6 px-5 sm:px-8 lg:mb-14'>
          <h2 className='font-brush text-5xl leading-none sm:text-7xl'>
            {t('Scenes')}
          </h2>
          <p className='font-display text-muted-foreground max-w-xs text-lg italic'>
            {t('Moments of her, kept between wind and tide.')}
          </p>
        </div>
        <motion.div
          ref={trackRef}
          style={pinned ? { x } : undefined}
          className={cn(
            'flex gap-5 px-5 sm:gap-8 sm:px-8',
            pinned
              ? 'w-max pr-[10vw] pl-[max(2rem,calc((100vw-72rem)/2+2rem))]'
              : 'snap-x snap-mandatory overflow-x-auto pb-6'
          )}
        >
          {SCENES.map((name, index) => (
            <figure
              key={name}
              className={cn(
                'group relative w-[min(72vw,20rem)] shrink-0 snap-center lg:w-[min(24vw,24rem)]',
                index % 2 === 1 && 'lg:translate-y-14'
              )}
            >
              <div className='aspect-[1079/1525] overflow-hidden rounded-[1.75rem] shadow-xl'>
                <ArtPicture
                  name={name}
                  alt={t('An illustration of Cecilia')}
                  sizes='(min-width: 1024px) 24vw, 72vw'
                  className='size-full'
                  imgClassName='transition-transform duration-[1.4s] ease-[cubic-bezier(0.16,1,0.3,1)] group-hover:scale-[1.05]'
                />
              </div>
              <figcaption className='font-display text-muted-foreground mt-3 flex items-center justify-between text-sm italic'>
                <span>No. {String(index + 1).padStart(2, '0')}</span>
                <span className='font-mincho tracking-[0.3em] not-italic'>
                  {t('Cecilia')}
                </span>
              </figcaption>
            </figure>
          ))}
        </motion.div>
      </div>
    </section>
  )
}
