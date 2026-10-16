/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { motion, useReducedMotion, useScroll, useTransform } from 'motion/react'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { AnimateInView } from '@/components/animate-in-view'
import { cn } from '@/lib/utils'

import { ArtPicture } from '../art-picture'

type Act = {
  numeral: string
  label: string
  title: string
  body: string
  art: string
  alt: string
  objectPosition: string
}

type ActRowProps = {
  act: Act
  reverse: boolean
}

function ActRow(props: ActRowProps) {
  const ref = useRef<HTMLDivElement>(null)
  const reducedMotion = !!useReducedMotion()
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ['start end', 'end start'],
  })
  const drift = useTransform(scrollYProgress, [0, 1], [48, -48])

  return (
    <div
      ref={ref}
      className='grid items-center gap-10 lg:grid-cols-12 lg:gap-16'
    >
      <motion.div
        className={cn(
          'relative lg:col-span-6',
          props.reverse && 'lg:order-2 lg:col-start-7'
        )}
        style={reducedMotion ? undefined : { y: drift }}
      >
        <motion.div
          className='shadow-primary/10 aspect-[4/5] overflow-hidden rounded-[2rem] shadow-2xl sm:aspect-[5/4] lg:aspect-[4/5]'
          initial={
            reducedMotion
              ? false
              : { clipPath: 'inset(22% 0% 0% 0% round 2rem)', opacity: 0.4 }
          }
          whileInView={{
            clipPath: 'inset(0% 0% 0% 0% round 2rem)',
            opacity: 1,
          }}
          viewport={{ once: true, margin: '0px 0px -20% 0px' }}
          transition={{ duration: 1.2, ease: [0.16, 1, 0.3, 1] }}
        >
          <ArtPicture
            name={props.act.art}
            alt={props.act.alt}
            sizes='(min-width: 1024px) 45vw, 92vw'
            className='size-full'
            imgClassName='transition-transform duration-[1.6s] ease-[cubic-bezier(0.16,1,0.3,1)] hover:scale-[1.04]'
            objectPosition={props.act.objectPosition}
          />
        </motion.div>
      </motion.div>

      <AnimateInView
        className={cn(
          'relative lg:col-span-5',
          props.reverse ? 'lg:col-start-1' : 'lg:col-start-8'
        )}
      >
        <span
          aria-hidden='true'
          className='font-display text-wt-sakura/45 dark:text-wt-tide/15 pointer-events-none absolute -top-16 -left-2 text-[9rem] leading-none italic select-none sm:-top-20 sm:text-[11rem]'
        >
          {props.act.numeral}
        </span>
        <div className='relative flex items-start gap-5'>
          <p className='wt-vertical font-mincho text-wt-crimson mt-1 text-sm tracking-[0.45em]'>
            {props.act.label}
          </p>
          <div>
            <h3 className='font-brush text-5xl leading-tight sm:text-6xl'>
              {props.act.title}
            </h3>
            <p className='text-muted-foreground mt-5 max-w-md text-base leading-loose'>
              {props.act.body}
            </p>
          </div>
        </div>
      </AnimateInView>
    </div>
  )
}

/** 节目单 — how the site works, told as three acts. */
export function ThreeActs() {
  const { t } = useTranslation()

  const acts: Act[] = [
    {
      numeral: 'I',
      label: t('Act one'),
      title: t('Entry'),
      body: t(
        'Sign in with Discord. An account outside the allowlist files an entry request and waits for the owner to approve it.'
      ),
      art: 'gallery-2',
      alt: t('A girl reaching out a hand across a balcony railing'),
      objectPosition: '30% 50%',
    },
    {
      numeral: 'II',
      label: t('Act two'),
      title: t('Check-in'),
      body: t(
        'Check in once every check-in day to receive $9,999. Your balance has no upper limit.'
      ),
      art: 'gallery-3',
      alt: t('Afternoon tea with a heap of fresh scones'),
      objectPosition: '38% 80%',
    },
    {
      numeral: 'III',
      label: t('Act three'),
      title: t('Call'),
      body: t(
        'Create a token, point your client at this site and call models as usual.'
      ),
      art: 'gallery-9',
      alt: t('A girl with a long braid holding a teacup'),
      objectPosition: '50% 30%',
    },
  ]

  return (
    <section className='relative overflow-hidden py-24 sm:py-32'>
      <div className='mx-auto max-w-6xl px-5 sm:px-8'>
        <AnimateInView className='mb-20 flex flex-wrap items-end justify-between gap-6 sm:mb-28'>
          <h2 className='font-brush text-5xl leading-none sm:text-7xl'>
            {t('Programme')}
          </h2>
          <p className='font-display text-muted-foreground max-w-xs text-lg italic'>
            {t('Three acts, from your first sign-in to your first call.')}
          </p>
        </AnimateInView>
        <div className='flex flex-col gap-28 sm:gap-36'>
          {acts.map((act, index) => (
            <ActRow key={act.numeral} act={act} reverse={index % 2 === 1} />
          ))}
        </div>
      </div>
    </section>
  )
}
