/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import type { UseQueryResult } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

import type { DonationProgress, DonationProvider } from '../../types'

type DonateVerifyStepProps = {
  progress: UseQueryResult<DonationProgress>
  /** Which pool is verifying; the Antigravity one unless said otherwise. */
  provider?: DonationProvider
  /** The email typed in step 1; Google may have authorized another account. */
  typedEmail?: string
  onRetry: () => void
  onClose: () => void
}

export function DonateVerifyStep(props: DonateVerifyStepProps) {
  const { t } = useTranslation()
  const record = props.progress.data
  const aistudio = props.provider === 'aistudio'

  if (props.progress.isError) {
    return (
      <div className='space-y-4'>
        <Alert variant='destructive'>
          <AlertDescription>
            {t('Could not check the donation; close this and look at the list')}
          </AlertDescription>
        </Alert>
        <div className='flex justify-end'>
          <Button type='button' onClick={props.onClose}>
            {t('Close')}
          </Button>
        </div>
      </div>
    )
  }

  if (!record || record.status === 'verifying') {
    return (
      <div className='space-y-4'>
        <LoadingState
          message={
            aistudio
              ? t('Waiting for the pool to sign in with the account…')
              : t('Waiting for the pool to confirm the account…')
          }
        />
        <p className='text-muted-foreground text-sm'>
          {aistudio
            ? t(
                'This can take up to two minutes. You can close this; the list keeps showing the progress.'
              )
            : t(
                'This usually takes a few seconds. You can close this; the list keeps showing the progress.'
              )}
        </p>
        <div className='flex justify-end'>
          <Button type='button' variant='outline' onClick={props.onClose}>
            {t('Close')}
          </Button>
        </div>
      </div>
    )
  }

  if (record.status === 'failed') {
    return (
      <div className='space-y-4'>
        <Alert variant='destructive'>
          <AlertTitle>{t('The donation failed')}</AlertTitle>
          <AlertDescription>{t(record.fail_reason)}</AlertDescription>
        </Alert>
        <div className='flex justify-end gap-2'>
          <Button type='button' variant='outline' onClick={props.onClose}>
            {t('Close')}
          </Button>
          <Button type='button' onClick={props.onRetry}>
            {t('Retry')}
          </Button>
        </div>
      </div>
    )
  }

  if (aistudio) {
    return (
      <div className='space-y-4'>
        <Alert>
          <AlertTitle>{t('Added to the pool')}</AlertTitle>
          <AlertDescription>
            {record.email
              ? t(
                  '{{email}} is now in the AIS playground pool and raises your AIS PG calls for every day it stays. Thank you!',
                  { email: record.email }
                )
              : t(
                  'The account is now in the AIS playground pool and raises your AIS PG calls for every day it stays. Thank you!'
                )}
          </AlertDescription>
        </Alert>
        <div className='flex justify-end'>
          <Button type='button' onClick={props.onClose}>
            {t('Done')}
          </Button>
        </div>
      </div>
    )
  }

  const typedEmail = props.typedEmail ?? ''
  const mismatch = record.email !== typedEmail.trim().toLowerCase()
  return (
    <div className='space-y-4'>
      <Alert>
        <AlertTitle>{t('Added to the pool')}</AlertTitle>
        <AlertDescription>
          <span>
            {t(
              '{{email}} is now shared with everyone on this site. Thank you!',
              {
                email: record.email,
              }
            )}
          </span>
          {mismatch && (
            <span>
              {t(
                'Google authorized {{actual}} rather than {{typed}}; the pool keeps the account that was authorized.',
                { actual: record.email, typed: typedEmail }
              )}
            </span>
          )}
        </AlertDescription>
      </Alert>
      <div className='flex justify-end'>
        <Button type='button' onClick={props.onClose}>
          {t('Done')}
        </Button>
      </div>
    </div>
  )
}
