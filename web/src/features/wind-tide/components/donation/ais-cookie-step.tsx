/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { ClipboardPaste, Download } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

const COOKIE_ID = 'ais-donate-cookie'

/** The one-click helper the site ships (wind-tide/DEPLOY.md §10). */
export const AIS_COOKIE_HELPER_URL = '/tools/ais-cookie-helper.zip'

type AisCookieStepProps = {
  pending: boolean
  error: string | null
  onSubmit: (cookies: string) => void
  onCancel: () => void
}

/**
 * Step 2: the Cookie. It is held here until the member presses Donate and
 * then travels in that one request; nothing logs or stores it.
 */
export function AisCookieStep(props: AisCookieStepProps) {
  const { t } = useTranslation()
  const [cookies, setCookies] = useState('')
  const [pasteHint, setPasteHint] = useState<string | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const blockedHint = t(
    'This browser does not let the page read the clipboard; press Ctrl+V in the box instead'
  )

  const paste = async () => {
    const clipboard = navigator.clipboard
    if (!clipboard || typeof clipboard.readText !== 'function') {
      textareaRef.current?.focus()
      setPasteHint(blockedHint)
      return
    }
    try {
      const text = await clipboard.readText()
      if (!text.trim()) {
        textareaRef.current?.focus()
        setPasteHint(t('The clipboard is empty; copy the Cookie first'))
        return
      }
      setCookies(text)
      setPasteHint(null)
    } catch {
      textareaRef.current?.focus()
      setPasteHint(blockedHint)
    }
  }

  return (
    <form
      className='space-y-4'
      noValidate
      onSubmit={(event) => {
        event.preventDefault()
        props.onSubmit(cookies.trim())
      }}
    >
      <div className='space-y-2'>
        <Label htmlFor={COOKIE_ID}>
          {t('Cookie of the signed-in AI Studio page')}
        </Label>
        <Textarea
          id={COOKIE_ID}
          ref={textareaRef}
          value={cookies}
          onChange={(event) => setCookies(event.target.value)}
          rows={5}
          spellCheck={false}
          autoComplete='off'
          autoCapitalize='off'
          autoCorrect='off'
          placeholder='__Secure-1PSID=…; __Secure-1PAPISID=…; SAPISID=…'
          className='font-mono text-xs break-all'
        />
        <div className='flex flex-wrap items-center gap-x-3 gap-y-1'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={() => void paste()}
          >
            <ClipboardPaste className='size-3.5' />
            {t('Paste')}
          </Button>
          <span className='text-muted-foreground text-xs'>
            {pasteHint ??
              t(
                'The Cookie goes to the pool once and is not kept in your browser.'
              )}
          </span>
        </div>
      </div>

      <div className='bg-muted/40 space-y-2 rounded-lg border p-3 text-xs'>
        <p className='text-sm font-medium'>{t('How to get the Cookie')}</p>
        <p className='text-muted-foreground'>
          {t(
            'Sign in to aistudio.google.com in a browser profile used for nothing else, export the Cookie there, then close that profile and do not open the account again.'
          )}
        </p>
        <ol className='text-muted-foreground list-decimal space-y-2 pl-4'>
          <li>
            <span className='text-foreground font-medium'>
              {t('One-click helper')}
            </span>
            {' — '}
            {t(
              'Install it once: open chrome://extensions, turn on Developer mode, choose “Load unpacked” and pick the unzipped folder. Then, on aistudio.google.com, click the helper icon and it copies the Cookie.'
            )}{' '}
            <a
              href={AIS_COOKIE_HELPER_URL}
              download
              className='text-primary inline-flex items-center gap-1 underline underline-offset-4'
            >
              <Download className='size-3' aria-hidden='true' />
              {t('Download the helper (zip)')}
            </a>
          </li>
          <li>
            <span className='text-foreground font-medium'>Cookie-Editor</span>
            {' — '}
            {t(
              'Install Cookie-Editor from the Chrome, Edge or Firefox store, open aistudio.google.com, then choose Export → Header String.'
            )}
          </li>
          <li>
            <span className='text-foreground font-medium'>Android</span>
            {' — '}
            {t(
              'Paste the JSON that the aistudio-api extractor app exports, as it is.'
            )}
          </li>
        </ol>
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
        <Button type='submit' disabled={!cookies.trim() || props.pending}>
          {props.pending ? t('Donating…') : t('Donate')}
        </Button>
      </div>
    </form>
  )
}
