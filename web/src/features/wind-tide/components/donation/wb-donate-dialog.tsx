/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'

import { useWbDonateFlow } from '../../hooks/use-wb-donate-flow'
import { WB_TEXTS } from '../../lib/donation'
import { DonateVerifyStep } from './donate-verify-step'
import { WbConsentStep } from './wb-consent-step'
import { WbSignInStep } from './wb-sign-in-step'

type WbDonateDialogProps = {
  open: boolean
  onClose: () => void
}

const STEP_INDEX = { consent: 1, signin: 2, verify: 3 } as const
const STEP_LABELS = {
  consent: 'Consent',
  signin: 'Sign in',
  verify: 'Verify',
} as const

/**
 * The three-step WorkBuddy donate dialog (wind-tide/API.md §16): the risks
 * and the sign-in to use, Tencent's link, then the pool's verdict. Mount it
 * with a key per opening so every run starts from step 1.
 */
export function WbDonateDialog(props: WbDonateDialogProps) {
  const { t } = useTranslation()
  const flow = useWbDonateFlow()
  const close = () => {
    if (!flow.start.isPending) props.onClose()
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={(isOpen) => {
        if (!isOpen) close()
      }}
      title={t(WB_TEXTS.donate)}
      description={t('Step {{current}} of {{total}}: {{label}}', {
        current: STEP_INDEX[flow.step],
        total: 3,
        label: t(STEP_LABELS[flow.step]),
      })}
      contentClassName='sm:max-w-xl'
    >
      {flow.step === 'consent' && (
        <WbConsentStep
          realm={flow.realm}
          onContinue={flow.accept}
          onCancel={close}
        />
      )}
      {flow.step === 'signin' && (
        <WbSignInStep
          started={flow.started}
          error={flow.startError}
          onRestart={flow.restart}
          onCancel={close}
        />
      )}
      {flow.step === 'verify' && (
        <DonateVerifyStep
          provider='workbuddy'
          progress={flow.progress}
          onRetry={flow.restart}
          onClose={props.onClose}
        />
      )}
    </Dialog>
  )
}
