/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useLocation } from '@tanstack/react-router'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { BRAND, FORK_REPO_URL, UPSTREAM_REPO_URL } from '../constants'

const FORK_BRANCH_URL = `${FORK_REPO_URL}/tree/wind-tide`
const linkClass = 'text-primary underline-offset-4 hover:underline'

/**
 * The legal notices the license asks of a modified version (AGPLv3 §5(a),
 * §7(b), §13 and NOTICE), kept at /about#license where the footer points
 * (docs/adr/0002). The attribution sentence stays in English, as NOTICE
 * words it.
 */
export function LicenseNotice() {
  const { t } = useTranslation()
  const hash = useLocation({ select: (location) => location.hash })
  const section = useRef<HTMLElement>(null)

  // The page may still have been loading when the router looked for the
  // anchor, so the notice brings itself into view once it is on screen.
  useEffect(() => {
    if (hash === 'license') section.current?.scrollIntoView()
  }, [hash])

  return (
    <section
      ref={section}
      id='license'
      aria-labelledby='license-title'
      className='mx-auto max-w-3xl scroll-mt-20 space-y-4 px-4 py-12 text-sm leading-relaxed'
    >
      <h2 id='license-title' className='text-xl font-semibold'>
        {t('Open-source license')}
      </h2>
      <p>
        {t(
          '{{name}} runs a modified version of new-api. The changes, made in 2026, give it a new look and add Discord-only sign-in with an allowlist, a daily check-in, leaderboards and an owner view.',
          { name: BRAND.name }
        )}
      </p>
      <p lang='en'>Frontend design and development by New API contributors.</p>
      <ul className='text-muted-foreground space-y-1'>
        <li>
          new-api © 2023-2026 QuantumNous and contributors ·{' '}
          <a
            href={UPSTREAM_REPO_URL}
            target='_blank'
            rel='noopener noreferrer'
            className={linkClass}
          >
            github.com/QuantumNous/new-api
          </a>
        </li>
        <li>
          One API © 2023 JustSong ·{' '}
          <a
            href='https://github.com/songquanpeng/one-api'
            target='_blank'
            rel='noopener noreferrer'
            className={linkClass}
          >
            github.com/songquanpeng/one-api
          </a>
        </li>
      </ul>
      <p>
        {t(
          'This program is free software: you can redistribute it and/or modify it under the GNU Affero General Public License, version 3 or later. It comes with no warranty.'
        )}{' '}
        <a
          href={`${FORK_REPO_URL}/blob/wind-tide/LICENSE`}
          target='_blank'
          rel='noopener noreferrer'
          className={linkClass}
        >
          LICENSE
        </a>{' '}
        ·{' '}
        <a
          href={`${FORK_REPO_URL}/blob/wind-tide/NOTICE`}
          target='_blank'
          rel='noopener noreferrer'
          className={linkClass}
        >
          NOTICE
        </a>
      </p>
      <p>
        {t('The complete source code of the version running on this site:')}{' '}
        <a
          href={FORK_BRANCH_URL}
          target='_blank'
          rel='noopener noreferrer'
          className={`${linkClass} break-all`}
        >
          {FORK_BRANCH_URL.replace('https://', '')}
        </a>
      </p>
    </section>
  )
}
