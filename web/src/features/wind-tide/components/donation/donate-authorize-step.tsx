/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { ExternalLink } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

import { useNowSeconds } from '../../hooks/use-now-seconds'
import {
  CALLBACK_PRECHECK_MESSAGES,
  formatCountdown,
  precheckCallbackAddress,
} from '../../lib/donation'
import type { DonationStart } from '../../types'

const ADDRESS_ID = 'donate-callback-address'

type DonateAuthorizeStepProps = {
  started: DonationStart
  pending: boolean
  error: string | null
  onSubmit: (callbackUrl: string) => void
  onRestart: () => void
  onCancel: () => void
}

export function DonateAuthorizeStep(props: DonateAuthorizeStepProps) {
  const { t } = useTranslation()
  const [address, setAddress] = useState('')
  const now = useNowSeconds()
  const secondsLeft = props.started.expires_at - now
  const expired = secondsLeft <= 0
  const precheck = precheckCallbackAddress(address)
  let hint = t(
    'The address starts with http://localhost:51121/ and carries state and code parameters.'
  )
  if (precheck === 'no_state' || precheck === 'no_code') {
    hint = t(CALLBACK_PRECHECK_MESSAGES[precheck])
  }

  return (
    <div className='space-y-4'>
      <p className='text-sm'>
        {t(
          'Sign in with {{email}} in the new tab. When Google is done, the tab lands on a localhost page that cannot be opened: copy its whole address from the address bar and paste it below.',
          { email: props.started.email }
        )}
      </p>
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
          {t('Open Google sign-in')}
        </Button>
        <CopyButton value={props.started.authorization_url} variant='outline'>
          {t('Copy link')}
        </CopyButton>
      </div>
      <div className='space-y-2'>
        <Label htmlFor={ADDRESS_ID}>{t('Address Google sent you to')}</Label>
        <Textarea
          id={ADDRESS_ID}
          value={address}
          onChange={(event) => setAddress(event.target.value)}
          rows={3}
          spellCheck={false}
          disabled={expired}
        />
        <p className='text-muted-foreground text-xs'>{hint}</p>
      </div>
      {props.error && (
        <Alert variant='destructive'>
          <AlertDescription>{props.error}</AlertDescription>
        </Alert>
      )}
      <p className='text-muted-foreground text-xs'>
        {expired
          ? t('This link has expired, start again')
          : t('This link expires in {{time}}', {
              time: formatCountdown(secondsLeft),
            })}
      </p>
      <div className='flex justify-end gap-2'>
        <Button
          type='button'
          variant='outline'
          disabled={props.pending}
          onClick={props.onCancel}
        >
          {t('Cancel')}
        </Button>
        {expired ? (
          <Button type='button' onClick={props.onRestart}>
            {t('Start again')}
          </Button>
        ) : (
          <Button
            type='button'
            disabled={precheck === 'empty' || props.pending}
            onClick={() => props.onSubmit(address.trim())}
          >
            {t('Submit')}
          </Button>
        )}
      </div>
    </div>
  )
}
