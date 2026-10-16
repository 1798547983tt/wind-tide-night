/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'
import { useOpeningStore } from '@/stores/opening-store'

import { BRAND, FORK_REPO_URL, UPSTREAM_REPO_URL } from '../constants'
import { BrandMark } from './brand-mark'

type SiteFooterProps = {
  className?: string
}

const linkClass =
  'text-muted-foreground hover:text-foreground transition-colors duration-200'

/**
 * Site footer. It carries the attribution and the link to the original
 * project that NOTICE §7 requires, verbatim and untranslated, and the link to
 * this site's own source that AGPL §13 requires.
 */
export function SiteFooter(props: SiteFooterProps) {
  const { t } = useTranslation()
  const replay = useOpeningStore((state) => state.replay)
  const year = new Date().getFullYear()

  const replayOpening = () => {
    window.scrollTo({ top: 0 })
    replay()
  }

  return (
    <footer className={cn('border-t', props.className)}>
      <div className='mx-auto max-w-6xl px-5 py-12 sm:px-8 sm:py-14'>
        <div className='flex flex-col gap-10 md:flex-row md:items-end md:justify-between'>
          <div>
            <Link to='/' className='flex items-center gap-3'>
              <BrandMark className='size-9' />
              <span className='font-brush text-3xl leading-none'>
                {BRAND.name}
              </span>
            </Link>
            <p className='font-mincho text-muted-foreground mt-4 text-sm leading-relaxed whitespace-pre-line'>
              {t(
                'The wind carries the question away,\nthe tide brings the answer back.'
              )}
            </p>
          </div>
          <nav
            aria-label={t('Footer')}
            className='flex flex-wrap gap-x-7 gap-y-3 text-sm'
          >
            <Link to='/about' className={linkClass}>
              {t('About this site')}
            </Link>
            <Link to='/rankings' className={linkClass}>
              {t('Leaderboards')}
            </Link>
            <button
              type='button'
              onClick={replayOpening}
              className={cn(linkClass, 'cursor-pointer')}
            >
              {t('Replay opening')}
            </button>
            <a
              href={FORK_REPO_URL}
              target='_blank'
              rel='noopener noreferrer'
              className={linkClass}
            >
              {t('Source code')}
            </a>
          </nav>
        </div>

        <div className='text-muted-foreground mt-12 flex flex-col gap-3 border-t pt-6 text-xs leading-relaxed sm:flex-row sm:items-center sm:justify-between'>
          <p>
            © {year} {BRAND.name} · {t('A modified version of New API')}
          </p>
          <SiteAttribution />
        </div>
      </div>
    </footer>
  )
}

/**
 * The attribution line NOTICE §7 asks for, with the link to the original
 * project and to the license notice. Pages without the full footer, such as
 * the sign-in pages, show this line on its own.
 */
export function SiteAttribution(props: { className?: string }) {
  const { t } = useTranslation()

  return (
    <p
      className={cn(
        'flex flex-wrap items-center gap-x-2 gap-y-1',
        props.className
      )}
    >
      <span lang='en'>
        Frontend design and development by New API contributors.
      </span>
      <a
        href={UPSTREAM_REPO_URL}
        target='_blank'
        rel='noopener noreferrer'
        className='text-foreground/80 hover:text-foreground underline-offset-4 hover:underline'
      >
        github.com/QuantumNous/new-api
      </a>
      <span aria-hidden='true'>·</span>
      <Link
        to='/about'
        hash='license'
        className='hover:text-foreground underline-offset-4 hover:underline'
      >
        {t('Open-source license')}
      </Link>
    </p>
  )
}
