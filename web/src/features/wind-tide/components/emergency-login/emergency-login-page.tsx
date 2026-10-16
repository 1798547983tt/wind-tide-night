/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { Link } from '@tanstack/react-router'
import { isAxiosError } from 'axios'
import { KeyRound, Loader2 } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { AuthLayout } from '@/features/auth/auth-layout'
import { useAuthRedirect } from '@/features/auth/hooks/use-auth-redirect'

import { emergencyLogin } from '../../api'

type Failure = 'missing' | 'rejected' | 'throttled'

/**
 * Signs the owner in with a one-time link printed by `wt emergency-login`
 * (wind-tide/API.md §4). The token sits in the URL fragment, so it never
 * reaches server or proxy logs, and the page removes it from the address bar
 * as soon as it has been read.
 */
export function EmergencyLoginPage() {
  const { t } = useTranslation()
  const { handleLoginResult } = useAuthRedirect()
  const [token] = useState(
    () => new URLSearchParams(window.location.hash.slice(1)).get('token') ?? ''
  )
  const [failure, setFailure] = useState<Failure | null>(
    token ? null : 'missing'
  )
  // The link works once, so a repeated effect must reuse the first exchange.
  const exchange = useRef<ReturnType<typeof emergencyLogin> | null>(null)

  useEffect(() => {
    if (window.location.hash) {
      window.history.replaceState(
        window.history.state,
        '',
        window.location.pathname + window.location.search
      )
    }
    if (!token) return

    let active = true
    exchange.current ??= emergencyLogin(token)
    void (async () => {
      try {
        const response = await exchange.current
        if (!active) return
        if (response?.data.success) {
          await handleLoginResult(response.data.data)
          return
        }
      } catch (error) {
        if (!active) return
        // The rate limit answers before the server looks at the link, so the
        // link is not used up and can be opened again later.
        if (isAxiosError(error) && error.response?.status === 429) {
          setFailure('throttled')
          return
        }
      }
      setFailure('rejected')
    })()
    return () => {
      active = false
    }
  }, [handleLoginResult, token])

  let failureTitle = t('This emergency link is invalid or has expired.')
  let failureDetail = t(
    'Run the emergency-login command on the server again to get a new link. Each link works once within 10 minutes.'
  )
  if (failure === 'missing') {
    failureTitle = t('This emergency link is incomplete.')
  }
  if (failure === 'throttled') {
    failureTitle = t('Too many sign-in attempts from this network.')
    failureDetail = t(
      'Wait a while, then open the link again. If it has expired by then, run the emergency-login command on the server for a new one.'
    )
  }

  return (
    <AuthLayout>
      <div className='space-y-7'>
        <header className='space-y-4'>
          {/* A key in a seal frame: the owner's own way in. */}
          <div className='border-wt-crimson/60 text-wt-crimson flex size-14 rotate-[-4deg] items-center justify-center rounded-xl border-2'>
            <KeyRound className='size-7' aria-hidden='true' />
          </div>
          <h2 className='font-mincho text-[1.7rem] leading-snug font-semibold sm:text-3xl'>
            {t('Emergency sign-in')}
          </h2>
        </header>

        {failure === null ? (
          <p
            role='status'
            className='flex items-center gap-2 text-sm font-medium'
          >
            <Loader2 className='size-4 animate-spin' aria-hidden='true' />
            {t('Signing you in as the site owner...')}
          </p>
        ) : (
          <div
            role='alert'
            className='border-destructive/30 bg-destructive/5 space-y-2 rounded-xl border p-4'
          >
            <p className='font-medium'>{failureTitle}</p>
            <p className='text-muted-foreground text-sm leading-relaxed'>
              {failureDetail}
            </p>
          </div>
        )}

        {failure !== null && (
          <Button
            variant='outline'
            size='lg'
            className='h-11 rounded-full px-6'
            render={<Link to='/sign-in' />}
          >
            {t('Back to sign-in')}
          </Button>
        )}
      </div>
    </AuthLayout>
  )
}
