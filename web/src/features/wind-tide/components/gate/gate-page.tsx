/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { IconDiscord } from '@/assets/brand-icons'
import { Button } from '@/components/ui/button'
import { useOAuthLogin } from '@/features/auth/hooks/use-oauth-login'
import { useStatus } from '@/hooks/use-status'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { useGateStore } from '@/stores/gate-store'

import type { GateState } from '../../types'
import { AuthStage } from '../auth-stage/auth-stage'
import { MemberAvatar } from '../member-avatar'
import { SealStamp } from '../seal-stamp'

// Calligraphy on the admission ticket, read as a picture: the heading says
// the same in words, so these are not translation keys.
const TICKET_WORD = '入场券'
const GATE_SEALS: Record<
  GateState,
  { text: string; tone: 'slot' | 'crimson' | 'ink' }
> = {
  pending: { text: '待批', tone: 'slot' },
  rejected: { text: '改日', tone: 'crimson' },
  removed: { text: '暂别', tone: 'ink' },
}

/**
 * Tells a Discord account the admission gate turned away where it stands
 * (wind-tide/API.md §3): its entry request waits for the owner, was rejected,
 * or its access was removed. The request is drawn as an admission ticket that
 * waits for the owner's seal. Signing in again is how an approved applicant
 * gets in, since there is no session to poll with.
 */
export function GatePage() {
  const { t } = useTranslation()
  const refusal = useGateStore((state) => state.refusal)
  const { status } = useStatus()
  const { isLoading, handleDiscordLogin } = useOAuthLogin(status)

  // The route sends a visit without a refusal to the sign-in page.
  if (!refusal) return null

  const applicant = refusal.applicant
  const name =
    applicant.display_name || applicant.username || applicant.discord_id
  const seal = GATE_SEALS[refusal.wt_gate]
  let title: string
  let detail: string
  switch (refusal.wt_gate) {
    case 'pending':
      title = t('Waiting for the site owner to approve you')
      detail = t(
        'Your entry request has reached the site owner. Once it is approved, sign in with Discord again to enter.'
      )
      break
    case 'rejected':
      title = t('Your entry request was not approved')
      detail =
        refusal.retry_after * 1000 > Date.now()
          ? t('You can send a new request after {{time}}.', {
              time: formatTimestampToDate(refusal.retry_after),
            })
          : t(
              'You can send a new request now by signing in with Discord again.'
            )
      break
    case 'removed':
      title = t('Your access to this site has been removed')
      detail = t('Only the site owner can restore your access.')
      break
  }

  return (
    <AuthStage painting='tea'>
      <div className='space-y-7'>
        <header className='space-y-2'>
          <p className='font-mincho text-primary text-sm tracking-[0.3em]'>
            {t('Entry request')}
          </p>
          <h2 className='font-mincho text-[1.7rem] leading-snug font-semibold sm:text-3xl'>
            {title}
          </h2>
        </header>

        {/* The drop shadow follows the ticket's notches, a box shadow would not. */}
        <div className='drop-shadow-[0_18px_28px_rgb(43_34_49/0.14)] dark:drop-shadow-[0_18px_32px_rgb(0_0_0/0.45)]'>
          <div className='wt-ticket bg-card grid grid-cols-[var(--wt-stub)_minmax(0,1fr)] rounded-2xl'>
            <div className='relative flex flex-col items-center justify-between gap-4 py-5'>
              <p
                aria-hidden='true'
                className='wt-vertical font-brush text-primary/85 text-2xl leading-none tracking-[0.2em]'
              >
                {TICKET_WORD}
              </p>
              <SealStamp
                text={seal.text}
                tone={seal.tone}
                className={cn(
                  'size-12',
                  seal.tone === 'slot' && 'wt-await text-primary'
                )}
              />
              <span
                aria-hidden='true'
                className='wt-ticket-perforation text-border absolute inset-y-4 right-0 w-px'
              />
            </div>

            <div className='min-w-0 space-y-4 px-5 py-5'>
              <div className='flex items-center gap-3'>
                <MemberAvatar
                  name={name}
                  avatarUrl={applicant.avatar_url}
                  className='ring-background size-14 ring-4'
                  fallbackClassName='text-lg font-normal'
                />
                {/* Long names wrap: an unbreakable line would widen a phone screen. */}
                <div className='min-w-0'>
                  <p className='font-medium wrap-anywhere'>{name}</p>
                  {applicant.username && applicant.username !== name && (
                    <p className='text-muted-foreground text-sm wrap-anywhere'>
                      @{applicant.username}
                    </p>
                  )}
                </div>
              </div>

              <div className='space-y-2'>
                <p className='text-muted-foreground text-sm leading-relaxed'>
                  {detail}
                </p>
                {refusal.wt_gate === 'pending' && refusal.requested_at > 0 && (
                  <p className='text-muted-foreground/80 text-xs'>
                    {t('Requested at {{time}}', {
                      time: formatTimestampToDate(refusal.requested_at),
                    })}
                  </p>
                )}
              </div>
            </div>
          </div>
        </div>

        {refusal.wt_gate === 'pending' && (
          <p className='font-mincho text-muted-foreground text-sm'>
            {t(
              'Have a cup of tea while you wait. The owner will stamp your ticket when they see it.'
            )}
          </p>
        )}

        <div className='flex flex-col gap-2 sm:flex-row'>
          {status?.discord_oauth && refusal.wt_gate !== 'removed' && (
            <Button
              type='button'
              size='lg'
              className='h-11 rounded-full px-6'
              disabled={isLoading}
              onClick={() => void handleDiscordLogin()}
            >
              <IconDiscord className='size-4' aria-hidden='true' />
              {t('Sign in with Discord again')}
            </Button>
          )}
          <Button
            variant='outline'
            size='lg'
            className='h-11 rounded-full px-6'
            render={<Link to='/' />}
          >
            {t('Back to Home')}
          </Button>
        </div>
      </div>
    </AuthStage>
  )
}
