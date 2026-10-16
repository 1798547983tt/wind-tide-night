/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { useStatus } from '@/hooks/use-status'
import { formatQuotaWithCurrency } from '@/lib/currency'

import { checkinQueryOptions } from '../../api'
import { useCheckIn } from '../../hooks/use-check-in'
import { MascotFigure } from '../mascot/mascot-figure'
import { StampSeal } from './stamp-seal'

// The Beijing day the member last closed the dialog on. Only a convenience:
// without storage the dialog simply comes back on the next visit.
const SEEN_DAY_KEY = 'wt.daily-reward.seen'

function readSeenDay() {
  try {
    return window.localStorage.getItem(SEEN_DAY_KEY)
  } catch {
    return null
  }
}

/**
 * The daily reward of a mobile game (wind-tide/DESIGN.md §4): on the first
 * visit to the console each check-in day it offers the day's check-in, one
 * click to stamp and claim. Closing it, either way, keeps it away until the
 * next Beijing day.
 */
export function DailyRewardDialog() {
  const { t } = useTranslation()
  const { status } = useStatus()
  const month = useQuery({
    ...checkinQueryOptions(),
    enabled: status?.checkin_enabled === true,
    // An offer that cannot load simply does not appear.
    meta: { errorToast: false },
  })
  const checkIn = useCheckIn()
  const [seenDay, setSeenDay] = useState(readSeenDay)
  const [claimedDay, setClaimedDay] = useState<string | null>(null)

  const today = month.data?.today
  const claimed = today !== undefined && claimedDay === today
  // Once open it stays to show the claimed reward, until the member closes it.
  const open =
    today !== undefined &&
    seenDay !== today &&
    (claimed || month.data?.checked_in_today === false)

  const reward = formatQuotaWithCurrency(month.data?.reward_quota ?? 0, {
    digitsLarge: 0,
  })
  const close = () => {
    if (today === undefined) return
    try {
      window.localStorage.setItem(SEEN_DAY_KEY, today)
    } catch {
      // Without storage it shows again next time, which does no harm.
    }
    setSeenDay(today)
  }
  const claim = () => {
    checkIn.mutate(undefined, {
      onSuccess: (result) => setClaimedDay(result.checkin_date),
    })
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close()
      }}
      title={claimed ? t('Claimed!') : t("Today's check-in gift")}
      description={
        claimed
          ? t('Checked in. {{amount}} has been added to your balance.', {
              amount: reward,
            })
          : t('Stamp today and {{amount}} goes straight into your balance.', {
              amount: reward,
            })
      }
      contentClassName='sm:max-w-md'
      headerClassName='items-center text-center'
      titleClassName='font-mincho text-xl'
      footer={
        claimed ? (
          <Button type='button' className='rounded-full px-6' onClick={close}>
            {t('Thanks!')}
          </Button>
        ) : (
          <>
            <Button
              type='button'
              variant='ghost'
              className='rounded-full'
              onClick={close}
            >
              {t('Later')}
            </Button>
            <Button
              type='button'
              className='rounded-full px-6'
              disabled={checkIn.isPending}
              onClick={claim}
            >
              {t('Stamp and claim')}
            </Button>
          </>
        )
      }
    >
      <div className='flex items-end justify-center gap-6 pt-4 pb-2'>
        <MascotFigure
          mood={claimed ? 'rich' : 'wave'}
          line={
            claimed
              ? t('All stamped! See you tomorrow~')
              : t("Today's seal is waiting for you~")
          }
          sizes='112px'
          imageClassName='size-24 sm:size-28'
        />
        <StampSeal
          stamped={claimed}
          fresh={claimed}
          pressing={checkIn.isPending}
          reward={reward}
        />
      </div>
    </Dialog>
  )
}
