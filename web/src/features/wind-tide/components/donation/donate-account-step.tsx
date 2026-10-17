/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import {
  DONATE_FORM_DEFAULTS,
  donateFormSchema,
  type DonateFormValues,
} from '../../lib/donation-form'

const FORM_ID = 'donate-account-form'
const CONSENT_ID = 'donate-consent'

/** What the member agrees to before the account leaves their hands; i18n keys. */
const DONATION_RISKS = [
  'The refresh token of the account is stored in the pool on this site',
  "The account's remaining allowance is shared by every member and may run out quickly",
  'Google may apply risk controls to the account; donate unimportant spare accounts only',
  'You can remove the account from the pool here at any time',
] as const

type DonateAccountStepProps = {
  defaultEmail: string
  /** A re-authorization keeps the record's email. */
  emailLocked: boolean
  pending: boolean
  error: string | null
  onStart: (email: string) => void
  onCancel: () => void
}

export function DonateAccountStep(props: DonateAccountStepProps) {
  const { t } = useTranslation()
  const [agreed, setAgreed] = useState(false)
  const form = useForm<DonateFormValues>({
    resolver: zodResolver(donateFormSchema),
    defaultValues: { ...DONATE_FORM_DEFAULTS, email: props.defaultEmail },
  })

  return (
    <Form {...form}>
      <form
        id={FORM_ID}
        className='space-y-4'
        noValidate
        onSubmit={form.handleSubmit((values) => props.onStart(values.email))}
      >
        <FormField
          control={form.control}
          name='email'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Google account email')}</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  type='email'
                  inputMode='email'
                  autoComplete='off'
                  readOnly={props.emailLocked}
                />
              </FormControl>
              <FormDescription>
                {t(
                  'The Gmail address of the account you are donating. Google is asked to sign in with it.'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <ul className='text-muted-foreground list-disc space-y-1 pl-5 text-sm'>
          {DONATION_RISKS.map((risk) => (
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
        {props.error && (
          <Alert variant='destructive'>
            <AlertDescription>{props.error}</AlertDescription>
          </Alert>
        )}
        <div className='flex justify-end gap-2'>
          <Button
            type='button'
            variant='outline'
            disabled={props.pending}
            onClick={props.onCancel}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' disabled={!agreed || props.pending}>
            {t('Start')}
          </Button>
        </div>
      </form>
    </Form>
  )
}
