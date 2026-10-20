/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { ExternalLink } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

import { useNowSeconds } from '../../hooks/use-now-seconds'
import { formatCountdown } from '../../lib/donation'
import type { WbDonationStart } from '../../types'

type WbSignInStepProps = {
  /** The sign-in the pool started; null while it is being asked for. */
  started: WbDonationStart | null
  /** The pool refused to start one. */
  error: string | null
  onRestart: () => void
  onCancel: () => void
}

/**
 * Step 2 of a WorkBuddy donation (wind-tide/API.md §16): Tencent's link to
 * open and a countdown, while the pool waits for the sign-in to finish. The
 * dialog moves on by itself once the record settles.
 */
export function WbSignInStep(props: WbSignInStepProps) {
  const { t } = useTranslation()
  const now = useNowSeconds()

  if (props.error) {
    return (
      <div className='space-y-4'>
        <Alert variant='destructive'>
          <AlertDescription>{props.error}</AlertDescription>
        </Alert>
        <div className='flex justify-end gap-2'>
          <Button type='button' variant='outline' onClick={props.onCancel}>
            {t('Cancel')}
          </Button>
          <Button type='button' onClick={props.onRestart}>
            {t('Try again')}
          </Button>
        </div>
      </div>
    )
  }

  if (!props.started) {
    return (
      <div className='space-y-4'>
        <LoadingState message={t('Asking the pool for a sign-in link…')} />
        <div className='flex justify-end'>
          <Button type='button' variant='outline' onClick={props.onCancel}>
            {t('Cancel')}
          </Button>
        </div>
      </div>
    )
  }

  const secondsLeft = props.started.expires_at - now
  const expired = secondsLeft <= 0

  return (
    <div className='space-y-4'>
      <p className='text-sm'>
        {t(
          'Open the link and sign in with the account you are donating. Once Tencent is done, the pool collects the account by itself and this dialog moves on.'
        )}
      </p>
      {!expired && (
        <div className='flex flex-wrap gap-2'>
          <Button
            render={
              <a
                href={props.started.authorization_url}
                target='_blank'
                rel='noopener noreferrer'
              />
            }
          >
            <ExternalLink className='size-4' />
            {t('Open Tencent sign-in')}
          </Button>
          <CopyButton value={props.started.authorization_url} variant='outline'>
            {t('Copy link')}
          </CopyButton>
        </div>
      )}
      {!expired && (
        <LoadingState message={t('Waiting for you to finish the sign-in…')} />
      )}
      <p className='text-muted-foreground text-xs'>
        {expired
          ? t('This link has expired, start again')
          : t('This link expires in {{time}}', {
              time: formatCountdown(secondsLeft),
            })}
      </p>
      <div className='flex justify-end gap-2'>
        <Button type='button' variant='outline' onClick={props.onCancel}>
          {t('Cancel')}
        </Button>
        {expired && (
          <Button type='button' onClick={props.onRestart}>
            {t('Start again')}
          </Button>
        )}
      </div>
    </div>
  )
}
