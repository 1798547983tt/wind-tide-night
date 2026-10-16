/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { useRouterState } from '@tanstack/react-router'
import { X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'
import { getBeijingHour } from '@/lib/beijing-time'
import { formatQuotaWithCurrency } from '@/lib/currency'

import { artManifestQueryOptions, checkinQueryOptions } from '../../api'
import { ArtPicture } from '../art-picture'
import { MascotFigure, type MascotMood } from './mascot-figure'

// Whether the member has let her rest. Only a convenience: without storage
// she is simply back in her corner on the next visit.
const RESTING_KEY = 'wt.mascot.resting'
// How long a line stays in her bubble.
const LINE_MS = 6000
// Pages whose bottom-right corner holds a send button: the playground and
// the chats embedded from other sites. She steps aside there.
const SEND_BUTTON_PATHS = ['/playground', '/chat/']

type Line = { mood: MascotMood; text: string }

function readResting() {
  try {
    return window.localStorage.getItem(RESTING_KEY) === '1'
  } catch {
    return false
  }
}

/**
 * The mascot keeping members company in the console's bottom-right corner on
 * desktops (wind-tide/DESIGN.md §8). She idles there peeking out of the
 * bottom edge, and each click has her stand up with her next line: a greeting
 * for the Beijing hour first and, until the member has checked in, the day's
 * reward. Letting her rest leaves a small portrait that calls her back. Page
 * footers leave her corner free (styles/wind-tide.css); phones keep their
 * small screen for the work.
 */
export function DeskMascot() {
  const { t } = useTranslation()
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const { status } = useStatus()
  const manifest = useQuery(artManifestQueryOptions)
  const month = useQuery({
    ...checkinQueryOptions(),
    enabled: status?.checkin_enabled === true,
    meta: { errorToast: false },
  })
  const [resting, setResting] = useState(readResting)
  // Lines said so far; the bubble shows the latest while she is speaking.
  const [said, setSaid] = useState(0)
  const [speaking, setSpeaking] = useState(false)

  useEffect(() => {
    if (!speaking) return
    const timer = window.setTimeout(() => setSpeaking(false), LINE_MS)
    return () => window.clearTimeout(timer)
  }, [speaking, said])

  if (!manifest.data?.['mascot-base']) return null
  if (SEND_BUTTON_PATHS.some((path) => pathname.startsWith(path))) return null

  const rest = (next: boolean) => {
    try {
      if (next) window.localStorage.setItem(RESTING_KEY, '1')
      else window.localStorage.removeItem(RESTING_KEY)
    } catch {
      // Without storage she is back on the next visit, which does no harm.
    }
    setResting(next)
    setSpeaking(false)
  }

  if (resting) {
    return (
      <Button
        data-slot='desk-mascot'
        variant='outline'
        aria-label={t('Call Cecilia back')}
        onClick={() => rest(false)}
        className='fixed right-4 bottom-4 z-40 hidden size-12 overflow-hidden rounded-full p-0 shadow-md lg:inline-flex'
      >
        <ArtPicture
          name='mascot-base'
          alt=''
          sizes='48px'
          className='size-full'
          imgClassName='object-cover object-top'
        />
      </Button>
    )
  }

  const hour = getBeijingHour()
  let greeting = t('Good evening~ The tide has come in.')
  if (hour < 5) greeting = t("Still up? I'll keep you company tonight.")
  else if (hour < 11) greeting = t('Good morning~ The wind is gentle today.')
  else if (hour < 13) greeting = t("It's noon~ Don't forget lunch.")
  else if (hour < 18) {
    greeting = t('Good afternoon~ Rest your eyes between calls.')
  }
  const lines: Line[] = [{ mood: 'wave', text: greeting }]
  if (month.data?.checked_in_today === false) {
    lines.push({
      mood: 'rich',
      text: t("Today's {{reward}} is still waiting for your stamp~", {
        reward: formatQuotaWithCurrency(month.data.reward_quota, {
          digitsLarge: 0,
        }),
      }),
    })
  }
  lines.push(
    {
      mood: 'think',
      text: t(
        'The wind carries the question away, the tide brings the answer back.'
      ),
    },
    { mood: 'cry', text: t("Don't tug on my teddy bear!") },
    { mood: 'think', text: t('Keep your API keys somewhere safe, okay?') },
    { mood: 'toast', text: t("I'll stay right here in the corner~") }
  )
  const line = speaking ? lines[(said - 1) % lines.length] : undefined

  return (
    <div
      data-slot='desk-mascot'
      data-pose={speaking ? 'stand' : 'peek'}
      className='wt-desk-mascot group fixed right-4 bottom-0 z-40 hidden lg:block'
    >
      {/* Her lines are read out as they appear. */}
      <div aria-live='polite'>
        <MascotFigure
          mood={line?.mood}
          line={line?.text}
          bubbleSide='left'
          sizes='112px'
          className='items-start'
          imageClassName='wt-mascot-idle size-28'
          pictureAction={{
            label: t('Talk to Cecilia'),
            onClick: () => {
              setSaid((count) => count + 1)
              setSpeaking(true)
            },
          }}
        />
      </div>
      <Button
        variant='outline'
        size='icon-xs'
        aria-label={t('Let Cecilia rest')}
        onClick={() => rest(true)}
        className='absolute -top-1 right-0 rounded-full opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100'
      >
        <X />
      </Button>
    </div>
  )
}
