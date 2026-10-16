/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { IconDiscord } from '@/assets/brand-icons'
import { AnimateInView } from '@/components/animate-in-view'
import { Button } from '@/components/ui/button'

import { BRAND } from '../../constants'
import { SealStamp } from '../seal-stamp'
import { WindLines } from '../wind-lines'

type LandingCodaProps = {
  isAuthenticated: boolean
}

/** The closing line of the page, with one last way in. */
export function LandingCoda(props: LandingCodaProps) {
  const { t } = useTranslation()

  return (
    <section className='relative isolate overflow-hidden py-28 sm:py-40'>
      <div
        aria-hidden='true'
        className='from-secondary/60 dark:from-secondary/40 absolute inset-0 -z-10 bg-radial-[ellipse_at_50%_100%] to-transparent to-70%'
      />
      <WindLines className='text-primary/25 dark:text-wt-tide/30' />
      <AnimateInView className='relative mx-auto flex max-w-4xl flex-col items-center px-5 text-center'>
        <SealStamp className='mb-10 size-14' />
        <p className='font-brush text-[clamp(2.4rem,6.5vw,5.25rem)] leading-tight whitespace-pre-line'>
          {t('The wind is up;\nthe tide is on its way.')}
        </p>
        <p className='font-display text-muted-foreground mt-5 text-lg italic'>
          {BRAND.nameEn}
        </p>
        <div className='mt-12'>
          {props.isAuthenticated ? (
            <Button
              size='lg'
              className='h-12 rounded-full px-8'
              render={<Link to='/dashboard' />}
            >
              {t('Enter console')}
            </Button>
          ) : (
            <Button
              size='lg'
              className='h-12 rounded-full px-8'
              render={<Link to='/sign-in' />}
            >
              <IconDiscord className='size-4' />
              {t('Sign in with Discord')}
            </Button>
          )}
        </div>
      </AnimateInView>
    </section>
  )
}
