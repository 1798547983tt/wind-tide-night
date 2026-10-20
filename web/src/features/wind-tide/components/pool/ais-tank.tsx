/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { Link } from '@tanstack/react-router'
import { useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button, buttonVariants } from '@/components/ui/button'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import { COOKIE_POOL_TEXTS } from '../../lib/donation'
import { aisLevel } from '../../lib/pool'
import type { AisModelPool, AisPoolStatus, CookieProvider } from '../../types'
import { AisTransferDialog, type TransferMode } from './ais-transfer-dialog'
import { PoolTank, TankGauge, type TankTone } from './pool-tank'

type AisTankProps = {
  status: AisPoolStatus
}

type DialogState = {
  key: number
  mode: TransferMode
  pool: AisModelPool
} | null

/** The liquid of each Cookie pool: sun gold for the playground, matcha for BUILD. */
const TANK_TONES: Record<CookieProvider, TankTone> = {
  aistudio: 'sun',
  build: 'matcha',
}

/**
 * The panel of a Cookie pool, the AIS playground or BUILD (wind-tide/API.md
 * §15): a gauge per model with the member's calls for today beside the
 * public pool's, each with its own Give and Take. A donor gives and takes;
 * anyone else sees the panel behind glass with the way in.
 */
export function AisTank(props: AisTankProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const status = props.status
  const texts = COOKIE_POOL_TEXTS[status.provider]
  const donor = status.accounts > 0
  // A key per opening, so every dialog starts with an empty box.
  const [dialog, setDialog] = useState<DialogState>(null)
  const openings = useRef(0)
  const openDialog = (mode: TransferMode, pool: AisModelPool) => {
    openings.current += 1
    setDialog({ key: openings.current, mode, pool })
  }

  let overlay: ReactNode
  if (!donor) {
    overlay = (
      <Link
        to='/donations'
        search={{ tab: status.provider }}
        className={cn(buttonVariants(), 'font-mincho')}
      >
        {t(texts.unlock)}
      </Link>
    )
  }

  return (
    <>
      <PoolTank tone={TANK_TONES[status.provider]} overlay={overlay}>
        <div className='space-y-5'>
          <div className='flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1'>
            <p className='text-muted-foreground text-xs font-medium tracking-[0.2em] uppercase'>
              {t(texts.name)}
            </p>
            <p className='text-muted-foreground text-xs'>
              {t('Your active accounts: {{count}}', {
                count: formatNumber(status.accounts, locale),
              })}
            </p>
          </div>
          <div className='grid gap-6 sm:grid-cols-2'>
            {status.models.map((pool) => (
              <TankGauge
                key={pool.model}
                compact
                level={aisLevel(pool)}
                title={pool.relay_model}
                figure={
                  <>
                    <span>{formatNumber(pool.remaining, locale)}</span>
                    <span className='text-muted-foreground text-xl not-italic sm:text-2xl'>
                      {' / '}
                      {formatNumber(pool.allowance, locale)}
                    </span>
                  </>
                }
              >
                <p className='text-muted-foreground text-xs'>
                  {t('Calls left today')}
                </p>
                <p className='text-sm'>
                  {t('Public pool today: {{balance}}', {
                    balance: formatNumber(pool.pool_balance, locale),
                  })}
                </p>
                <p className='text-muted-foreground text-xs'>
                  {t('Used today: {{calls}}', {
                    calls: formatNumber(pool.calls, locale),
                  })}
                </p>
                {donor && (
                  <div className='flex flex-wrap items-center gap-2 pt-1'>
                    <Button
                      size='sm'
                      disabled={pool.remaining <= 0}
                      onClick={() => openDialog('give', pool)}
                    >
                      {t('Give calls')}
                    </Button>
                    <Button
                      size='sm'
                      variant='outline'
                      disabled={pool.take_left <= 0 || pool.pool_balance <= 0}
                      onClick={() => openDialog('take', pool)}
                    >
                      {t('Take calls')}
                    </Button>
                    <span className='text-muted-foreground text-xs'>
                      {t('You can still take {{count}} today', {
                        count: formatNumber(pool.take_left, locale),
                      })}
                    </span>
                  </div>
                )}
              </TankGauge>
            ))}
          </div>
        </div>
      </PoolTank>

      {dialog && (
        <AisTransferDialog
          key={dialog.key}
          open
          mode={dialog.mode}
          pool={dialog.pool}
          onClose={() => setDialog(null)}
        />
      )}
    </>
  )
}
