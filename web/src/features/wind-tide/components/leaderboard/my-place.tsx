/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useTranslation } from 'react-i18next'

import { gapToPlaceAbove } from '../../lib/leaderboard'
import type { Leaderboard, LeaderboardBoard } from '../../types'
import { SealStamp } from '../seal-stamp'

// Calligraphy on the seals, read as a picture: the text beside each says the
// same in words, so these are not translation keys.
const PLACE_SEAL = '名次'
const OWNER_SEAL = '站长'

// What gets a member onto each board, as i18n keys, told to one not on it
// yet; on the failed boards, that they are better off as they are.
const NOT_ON_BOARD_NOTE: Record<LeaderboardBoard, string> = {
  calls: 'One call puts you on the board~',
  failed: 'No failed calls; long may it last~',
  success_streak: 'One successful call starts your streak~',
  fail_streak: 'No failed calls; long may it last~',
  balance: 'Check in to get on the board~',
}

type MyPlaceProps = {
  board: Leaderboard
  formatValue: (value: number) => string
}

/**
 * The signed-in member's own place, pinned to the bottom of the screen while
 * the list scrolls by: their rank and value and how far the place above is,
 * or why they have no place.
 */
export function MyPlace(props: MyPlaceProps) {
  const { t } = useTranslation()
  const me = props.board.me

  let seal = <SealStamp text={PLACE_SEAL} className='size-11 shrink-0' />
  let headline: string
  let note: string | undefined
  if (!me) {
    seal = (
      <SealStamp text={OWNER_SEAL} tone='ink' className='size-11 shrink-0' />
    )
    headline = t('The owner is not ranked')
  } else if (me.value <= 0) {
    seal = (
      <SealStamp
        text={PLACE_SEAL}
        tone='slot'
        className='text-primary size-11 shrink-0'
      />
    )
    headline = t('Not on the board yet')
    note = t(NOT_ON_BOARD_NOTE[props.board.board])
  } else {
    headline = t('No. {{rank}}', { rank: me.rank })
    const gap = gapToPlaceAbove(props.board)
    if (me.rank === 1) {
      note = t("You're at the top!")
    } else if (gap !== undefined) {
      note = t('{{gap}} behind the place above', {
        gap: props.formatValue(gap),
      })
    }
  }

  return (
    <aside aria-label={t('My place')} className='sticky bottom-4 z-10'>
      <div className='bg-background/85 mx-auto flex max-w-2xl items-center gap-4 rounded-2xl border px-4 py-3 shadow-lg backdrop-blur-md sm:px-6'>
        {seal}
        <div className='min-w-0 flex-1'>
          <p className='text-muted-foreground text-xs'>{t('My place')}</p>
          <p className='font-mincho truncate text-base font-semibold sm:text-lg'>
            {headline}
          </p>
          {note && (
            <p className='text-muted-foreground truncate text-xs'>{note}</p>
          )}
        </div>
        {me && me.value > 0 && (
          <p className='font-display shrink-0 text-xl font-semibold whitespace-nowrap [font-variant-numeric:lining-nums_tabular-nums] sm:text-2xl'>
            {props.formatValue(me.value)}
          </p>
        )}
      </div>
    </aside>
  )
}
