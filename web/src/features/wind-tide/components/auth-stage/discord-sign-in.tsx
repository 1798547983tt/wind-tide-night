/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { Globe, Loader2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IconDiscord } from '@/assets/brand-icons'
import { Button } from '@/components/ui/button'
import { LegalConsent } from '@/features/auth/components/legal-consent'
import { useOAuthLogin } from '@/features/auth/hooks/use-oauth-login'
import { useStatus } from '@/hooks/use-status'

import { BRAND } from '../../constants'

type DiscordSignInProps = {
  /** Where to land after signing in, from the sign-in page's search. */
  redirectTo?: string
}

/**
 * The way in on a Discord-only site (docs/adr/0001). One Discord sign-in
 * both admits an allowlisted member and files a newcomer's entry request, so
 * the page explains the steps instead of offering a sign-up.
 */
export function DiscordSignIn(props: DiscordSignInProps) {
  const { t } = useTranslation()
  const { status, loading } = useStatus()
  const { isLoading, handleDiscordLogin } = useOAuthLogin(
    status,
    props.redirectTo
  )
  const [agreed, setAgreed] = useState(false)

  const ready = Boolean(status?.discord_oauth && status?.discord_client_id)
  const needsConsent = Boolean(
    status?.user_agreement_enabled || status?.privacy_policy_enabled
  )
  const steps = [
    t('Authorize with your Discord account'),
    t('First visit: wait for the site owner to approve you'),
    t('Check in every day to receive $9,999'),
  ]

  return (
    <div className='space-y-8'>
      <header className='space-y-3'>
        <p className='font-display text-primary text-sm font-medium tracking-[0.3em] uppercase italic'>
          {BRAND.nameEn}
        </p>
        <h2 className='font-mincho text-[1.75rem] leading-snug font-semibold sm:text-4xl'>
          {t('Welcome, traveller of the night')}
        </h2>
        <p className='text-muted-foreground text-sm leading-relaxed sm:text-base'>
          {t(
            'This site is open only to members the owner invites. Sign in with Discord: members on the list walk straight in, and a first visit sends an entry request on its own.'
          )}
        </p>
      </header>

      <ol className='border-border/70 divide-border/70 divide-y border-y'>
        {steps.map((step, index) => (
          <li key={step} className='flex items-baseline gap-4 py-3'>
            <span
              aria-hidden='true'
              className='font-display text-primary w-6 shrink-0 text-xl italic'
            >
              {String(index + 1).padStart(2, '0')}
            </span>
            <span className='text-sm'>{step}</span>
          </li>
        ))}
      </ol>

      <div className='space-y-4'>
        <LegalConsent
          status={status}
          checked={agreed}
          onCheckedChange={setAgreed}
        />
        <Button
          type='button'
          size='lg'
          className='h-12 w-full rounded-full text-base shadow-[0_10px_30px_-12px_var(--primary)]'
          disabled={!ready || isLoading || (needsConsent && !agreed)}
          onClick={() => void handleDiscordLogin()}
        >
          {isLoading ? (
            <Loader2 className='size-5 animate-spin' aria-hidden='true' />
          ) : (
            <IconDiscord className='size-5' aria-hidden='true' />
          )}
          {t('Sign in with Discord')}
        </Button>
        {!loading && !ready && (
          <p role='status' className='text-muted-foreground text-sm'>
            {t(
              'Discord sign-in is not ready yet. The site owner still needs to set it up.'
            )}
          </p>
        )}
        <p className='text-muted-foreground flex gap-2 text-xs leading-relaxed'>
          <Globe className='mt-0.5 size-3.5 shrink-0' aria-hidden='true' />
          {t(
            'Signing in opens discord.com, which needs a proxy in mainland China. Once you are in, the site itself works without one.'
          )}
        </p>
      </div>
    </div>
  )
}
