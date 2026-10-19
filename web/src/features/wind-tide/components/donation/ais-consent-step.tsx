/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'

const CONSENT_ID = 'ais-donate-consent'

/** What the member agrees to before the Cookie leaves their hands; i18n keys. */
const AIS_DONATION_RISKS = [
  'The Cookie is the full sign-in of that Google account: whoever holds it can act as the account',
  'Use a throwaway account, signed in alone in a browser profile of its own; only the first account of the browser sign-in (u/0) is used',
  'The pool keeps the Cookie; this site keeps only the account id and the email',
  'AI Studio may rate-limit or ban accounts used this way; donate accounts you would not miss',
  'You can remove the account from the pool here at any time',
] as const

type AisConsentStepProps = {
  onContinue: () => void
  onCancel: () => void
}

export function AisConsentStep(props: AisConsentStepProps) {
  const { t } = useTranslation()
  const [agreed, setAgreed] = useState(false)

  return (
    <div className='space-y-4'>
      <p className='text-sm'>
        {t(
          'Donating hands the pool the Cookie of a signed-in AI Studio page. Read this first:'
        )}
      </p>
      <ul className='text-muted-foreground list-disc space-y-1 pl-5 text-sm'>
        {AIS_DONATION_RISKS.map((risk) => (
          <li key={risk}>{t(risk)}</li>
        ))}
      </ul>
      <div className='flex items-start gap-2'>
        <Checkbox
          id={CONSENT_ID}
          checked={agreed}
          onCheckedChange={(checked) => setAgreed(checked === true)}
        />
        <Label htmlFor={CONSENT_ID} className='font-normal'>
          {t('I understand and want to continue')}
        </Label>
      </div>
      <div className='flex justify-end gap-2'>
        <Button type='button' variant='outline' onClick={props.onCancel}>
          {t('Cancel')}
        </Button>
        <Button type='button' disabled={!agreed} onClick={props.onContinue}>
          {t('Continue')}
        </Button>
      </div>
    </div>
  )
}
