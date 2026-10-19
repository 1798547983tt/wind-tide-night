/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { getServerErrorMessage } from '@/lib/server-error-message'

import { giveAisCalls, takeAisCalls } from '../../api'
import { invalidatePoolQueries, parseCallCount } from '../../lib/pool'
import type { AisModelPool } from '../../types'

export type TransferMode = 'give' | 'take'

const COUNT_ID = 'ais-transfer-count'

type AisTransferDialogProps = {
  mode: TransferMode
  /** The model whose calls move; a give or take touches one pool only. */
  pool: AisModelPool
  open: boolean
  onClose: () => void
}

/**
 * Give calls of one model to its public pool or take some from it
 * (wind-tide/API.md §15). The checks here use the server's own words, so a
 * member reads the same sentence whichever side catches the mistake; after
 * a transfer the pool figures are fetched again and the dialog closes.
 */
export function AisTransferDialog(props: AisTransferDialogProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const queryClient = useQueryClient()
  const [raw, setRaw] = useState('')
  const [error, setError] = useState<string | null>(null)
  const giving = props.mode === 'give'
  const pool = props.pool
  const max = giving
    ? pool.remaining
    : Math.min(pool.take_left, pool.pool_balance)

  const transfer = useMutation({
    mutationFn: (count: number) => {
      const input = { model: pool.model, count }
      return giving ? giveAisCalls(input) : takeAisCalls(input)
    },
    onSuccess: async () => {
      await invalidatePoolQueries(queryClient)
      props.onClose()
    },
    onError: (cause) =>
      setError(
        getServerErrorMessage(cause, t('The pool did not accept the transfer'))
      ),
  })

  const submit = (event: FormEvent) => {
    event.preventDefault()
    const count = parseCallCount(raw)
    if (count === null) {
      setError(t('Enter a whole number of calls, 1 or more'))
      return
    }
    if (giving && count > pool.remaining) {
      setError(t('You can only give calls you still have today'))
      return
    }
    if (!giving && count > pool.take_left) {
      setError(
        t('You can take at most {{max}} calls a day from the pool', {
          max: pool.take_max,
        })
      )
      return
    }
    if (!giving && count > pool.pool_balance) {
      setError(t('The pool does not hold that many calls right now'))
      return
    }
    setError(null)
    transfer.mutate(count)
  }

  const close = () => {
    if (!transfer.isPending) props.onClose()
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={(isOpen) => {
        if (!isOpen) close()
      }}
      title={
        giving ? t('Give calls to the pool') : t('Take calls from the pool')
      }
      description={
        giving
          ? t(
              'You have {{remaining}} {{model}} calls left today; whatever you give, everyone can take.',
              {
                remaining: formatNumber(pool.remaining, locale),
                model: pool.model,
              }
            )
          : t(
              'The pool holds {{balance}} {{model}} calls today, and you can still take {{count}}.',
              {
                balance: formatNumber(pool.pool_balance, locale),
                model: pool.model,
                count: formatNumber(pool.take_left, locale),
              }
            )
      }
    >
      <form className='space-y-4' noValidate onSubmit={submit}>
        <div className='space-y-2'>
          <Label htmlFor={COUNT_ID}>{t('Number of calls')}</Label>
          <Input
            id={COUNT_ID}
            inputMode='numeric'
            autoComplete='off'
            value={raw}
            onChange={(event) => setRaw(event.target.value)}
            placeholder={String(Math.max(1, Math.min(10, max)))}
          />
          <p className='text-muted-foreground text-xs'>
            {t('Up to {{max}}', { max: formatNumber(max, locale) })}
          </p>
        </div>
        {error && (
          <Alert variant='destructive'>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <div className='flex justify-end gap-2'>
          <Button
            type='button'
            variant='outline'
            disabled={transfer.isPending}
            onClick={close}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' disabled={transfer.isPending || !raw.trim()}>
            {giving ? t('Give') : t('Take')}
          </Button>
        </div>
      </form>
    </Dialog>
  )
}
