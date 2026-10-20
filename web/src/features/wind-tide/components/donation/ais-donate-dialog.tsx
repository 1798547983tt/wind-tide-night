/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'

import { useAisDonateFlow } from '../../hooks/use-ais-donate-flow'
import { COOKIE_POOL_TEXTS } from '../../lib/donation'
import type { CookieProvider } from '../../types'
import { AisConsentStep } from './ais-consent-step'
import { AisCookieStep } from './ais-cookie-step'
import { DonateVerifyStep } from './donate-verify-step'

type AisDonateDialogProps = {
  /** The pool the Cookie goes to. */
  provider: CookieProvider
  open: boolean
  onClose: () => void
}

const STEP_INDEX = { consent: 1, cookie: 2, verify: 3 } as const
const STEP_LABELS = {
  consent: 'Consent',
  cookie: 'Cookie',
  verify: 'Verify',
} as const

/**
 * The three-step Cookie donate dialog (wind-tide/API.md §15): the risks, the
 * Cookie, then the pool's verdict. Mount it with a key per opening so every
 * run starts from step 1.
 */
export function AisDonateDialog(props: AisDonateDialogProps) {
  const { t } = useTranslation()
  const flow = useAisDonateFlow(props.provider)
  const busy = flow.submit.isPending
  const close = () => {
    if (!busy) props.onClose()
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={(isOpen) => {
        if (!isOpen) close()
      }}
      title={t(COOKIE_POOL_TEXTS[props.provider].donate)}
      description={t('Step {{current}} of {{total}}: {{label}}', {
        current: STEP_INDEX[flow.step],
        total: 3,
        label: t(STEP_LABELS[flow.step]),
      })}
      contentClassName='sm:max-w-xl'
    >
      {flow.step === 'consent' && (
        <AisConsentStep onContinue={flow.accept} onCancel={close} />
      )}
      {flow.step === 'cookie' && (
        <AisCookieStep
          pending={flow.submit.isPending}
          error={flow.submitError}
          onSubmit={(cookies) => flow.submit.mutate(cookies)}
          onCancel={close}
        />
      )}
      {flow.step === 'verify' && (
        <DonateVerifyStep
          provider={props.provider}
          progress={flow.progress}
          onRetry={flow.retry}
          onClose={props.onClose}
        />
      )}
    </Dialog>
  )
}
