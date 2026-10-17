/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'

import { useDonateFlow, type DonateTarget } from '../../hooks/use-donate-flow'
import { DonateAccountStep } from './donate-account-step'
import { DonateAuthorizeStep } from './donate-authorize-step'
import { DonateVerifyStep } from './donate-verify-step'

type DonateDialogProps = {
  open: boolean
  onClose: () => void
  /** The donation to authorize again; null donates a new account. */
  target: DonateTarget
}

const STEP_INDEX = { account: 1, authorize: 2, verify: 3 } as const
const STEP_LABELS = {
  account: 'Account',
  authorize: 'Authorize',
  verify: 'Verify',
} as const

/**
 * The three-step donate dialog (wind-tide/DESIGN.md §14): the account and the
 * risks, Google's link and the pasted callback address, then the verdict.
 * Mount it with a key per opening so every run starts from step 1.
 */
export function DonateDialog(props: DonateDialogProps) {
  const { t } = useTranslation()
  const flow = useDonateFlow(props.target)
  const busy = flow.start.isPending || flow.submit.isPending
  const close = () => {
    if (!busy) props.onClose()
  }
  const typedEmail = flow.start.variables ?? flow.target?.email ?? ''

  return (
    <Dialog
      open={props.open}
      onOpenChange={(isOpen) => {
        if (!isOpen) close()
      }}
      title={
        props.target ? t('Authorize the account again') : t('Donate an account')
      }
      description={t('Step {{current}} of {{total}}: {{label}}', {
        current: STEP_INDEX[flow.step],
        total: 3,
        label: t(STEP_LABELS[flow.step]),
      })}
      contentClassName='sm:max-w-lg'
    >
      {flow.step === 'account' && (
        <DonateAccountStep
          defaultEmail={flow.target?.email ?? flow.start.variables ?? ''}
          emailLocked={flow.target !== null}
          pending={flow.start.isPending}
          error={flow.startError}
          onStart={(email) => flow.start.mutate(email)}
          onCancel={close}
        />
      )}
      {flow.step === 'authorize' && flow.started && (
        <DonateAuthorizeStep
          started={flow.started}
          pending={flow.submit.isPending}
          error={flow.submitError}
          onSubmit={(callbackUrl) => flow.submit.mutate(callbackUrl)}
          onRestart={flow.restart}
          onCancel={close}
        />
      )}
      {flow.step === 'verify' && (
        <DonateVerifyStep
          progress={flow.progress}
          typedEmail={typedEmail}
          onRetry={flow.retry}
          onClose={props.onClose}
        />
      )}
    </Dialog>
  )
}
