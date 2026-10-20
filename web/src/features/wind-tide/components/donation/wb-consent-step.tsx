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
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'

import { WB_REALM_LABELS } from '../../lib/donation'
import type { WbRealm } from '../../types'

const CONSENT_ID = 'wb-donate-consent'

const WB_REALMS: readonly WbRealm[] = ['cn', 'global']

/** What the member agrees to before the account is signed in; i18n keys. */
const WB_DONATION_RISKS = [
  'The pool keeps the sign-in token of that account and refreshes it; whoever runs the pool can act as the account',
  "Every member of this site shares the account's credits; there is no daily count per member",
  'The pool checks the account in every day on your behalf, so its credits keep coming',
  'Tencent may rate-limit, risk-control or ban accounts used this way; donate accounts you would not miss',
  'You can remove the account from the pool here at any time',
] as const

type WbConsentStepProps = {
  /** The sign-in chosen last time, or the default. */
  realm: WbRealm
  onContinue: (realm: WbRealm) => void
  onCancel: () => void
}

export function WbConsentStep(props: WbConsentStepProps) {
  const { t } = useTranslation()
  const [agreed, setAgreed] = useState(false)
  const [realm, setRealm] = useState<WbRealm>(props.realm)

  return (
    <div className='space-y-4'>
      <p className='text-sm'>
        {t(
          "Donating signs a CodeBuddy or WorkBuddy account in to the pool on Tencent's own page. Read this first:"
        )}
      </p>
      <ul className='text-muted-foreground list-disc space-y-1 pl-5 text-sm'>
        {WB_DONATION_RISKS.map((risk) => (
          <li key={risk}>{t(risk)}</li>
        ))}
      </ul>
      <fieldset className='space-y-2'>
        <legend className='text-sm font-medium'>{t('Which sign-in?')}</legend>
        <RadioGroup
          value={realm}
          onValueChange={(value) => {
            if (value === 'cn' || value === 'global') setRealm(value)
          }}
          className='sm:grid-cols-2'
        >
          {WB_REALMS.map((option) => (
            <div key={option} className='flex items-center gap-2'>
              <RadioGroupItem id={`wb-realm-${option}`} value={option} />
              <Label htmlFor={`wb-realm-${option}`} className='font-normal'>
                {t(WB_REALM_LABELS[option])}
              </Label>
            </div>
          ))}
        </RadioGroup>
      </fieldset>
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
        <Button
          type='button'
          disabled={!agreed}
          onClick={() => props.onContinue(realm)}
        >
          {t('Continue')}
        </Button>
      </div>
    </div>
  )
}
