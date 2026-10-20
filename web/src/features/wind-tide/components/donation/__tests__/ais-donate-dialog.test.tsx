/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  test,
  vi,
  type Mock,
} from 'vitest'

import { api } from '@/lib/api'

import type { CookieProvider, DonationProgress } from '../../../types'
import { AisDonateDialog } from '../ais-donate-dialog'

const COOKIE = '__Secure-1PSID=abc; SAPISID=def'
const EMAIL = 'spare@example.com'

let progress: DonationProgress
let onClose: Mock<() => void>

function renderDialog(provider: CookieProvider = 'aistudio') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  onClose = vi.fn<() => void>()
  render(
    <QueryClientProvider client={client}>
      <AisDonateDialog provider={provider} open onClose={onClose} />
    </QueryClientProvider>
  )
}

function consent() {
  return screen.getByRole('checkbox', {
    name: 'I understand and want to continue',
  })
}

function cookieBox() {
  return screen.getByLabelText('Cookie of the signed-in AI Studio page')
}

async function agree() {
  await userEvent.click(consent())
  await userEvent.click(screen.getByRole('button', { name: 'Continue' }))
  await screen.findByText('Step 2 of 3: Cookie')
}

async function donate(cookie = COOKIE) {
  await userEvent.click(cookieBox())
  await userEvent.paste(cookie)
  await userEvent.click(screen.getByRole('button', { name: 'Donate' }))
}

beforeEach(() => {
  progress = { id: 12, email: EMAIL, status: 'active', fail_reason: '' }
  vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (
      url === '/api/wt/donations/aistudio' ||
      url === '/api/wt/donations/build'
    ) {
      return { data: { success: true, data: { id: 12, status: 'verifying' } } }
    }
    throw new Error(`Unexpected POST ${url}`)
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/wt/donations/12') {
      return { data: { success: true, data: progress } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('step 1: the risks', () => {
  test('Continue stays disabled until the risks are accepted', async () => {
    renderDialog()
    expect(screen.getByText('Step 1 of 3: Consent')).toBeInTheDocument()
    expect(
      screen.getByText(/only the first account of the browser sign-in/)
    ).toBeInTheDocument()
    const next = screen.getByRole('button', { name: 'Continue' })
    expect(next).toBeDisabled()
    await userEvent.click(consent())
    expect(next).toBeEnabled()
    await userEvent.click(next)
    expect(await screen.findByText('Step 2 of 3: Cookie')).toBeInTheDocument()
    expect(api.post).not.toHaveBeenCalled()
  })
})

describe('step 2: the Cookie', () => {
  test('explains the three ways to get it and links the helper', async () => {
    renderDialog()
    await agree()
    const helper = screen.getByText('Download the helper (zip)').closest('a')
    expect(helper).toHaveAttribute('href', '/tools/ais-cookie-helper.zip')
    expect(screen.getByText(/Load unpacked/)).toBeInTheDocument()
    expect(screen.getByText(/Export → Header String/)).toBeInTheDocument()
    expect(screen.getByText(/aistudio-api extractor app/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Donate' })).toBeDisabled()
  })

  test('Paste fills the box from the clipboard', async () => {
    const user = userEvent.setup()
    renderDialog()
    await agree()
    await navigator.clipboard.writeText(COOKIE)
    await user.click(screen.getByRole('button', { name: 'Paste' }))
    // readText() resolves on its own tick; wait for the box to fill.
    await waitFor(() => expect(cookieBox()).toHaveValue(COOKIE))
    expect(screen.getByRole('button', { name: 'Donate' })).toBeEnabled()
  })

  test('when the clipboard is closed, Paste points at the box instead', async () => {
    const user = userEvent.setup()
    renderDialog()
    await agree()
    vi.spyOn(navigator.clipboard, 'readText').mockRejectedValueOnce(
      new Error('NotAllowedError')
    )
    await user.click(screen.getByRole('button', { name: 'Paste' }))
    expect(
      await screen.findByText(/press Ctrl\+V in the box instead/)
    ).toBeInTheDocument()
    expect(cookieBox()).toHaveFocus()
    expect(cookieBox()).toHaveValue('')
  })

  test('Donate hands the Cookie to the server once and moves on to the verdict', async () => {
    renderDialog()
    await agree()
    await donate()
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/wt/donations/aistudio', {
        cookies: COOKIE,
        accepted: true,
      })
    )
    expect(await screen.findByText('Step 3 of 3: Verify')).toBeInTheDocument()
    expect(await screen.findByText('Added to the pool')).toBeInTheDocument()
    expect(
      screen.getByText(
        `${EMAIL} is now in the AIS playground pool and raises your AIS PG calls for every day it stays. Thank you!`
      )
    ).toBeInTheDocument()
    expect(api.get).toHaveBeenCalledWith('/api/wt/donations/12')
    await userEvent.click(screen.getByRole('button', { name: 'Done' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  test('for the BUILD pool the same steps post to its own address, and the verdict names it', async () => {
    renderDialog('build')
    expect(screen.getByText('Donate a BUILD account')).toBeInTheDocument()
    await agree()
    await donate()
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/wt/donations/build', {
        cookies: COOKIE,
        accepted: true,
      })
    )
    expect(api.post).not.toHaveBeenCalledWith(
      '/api/wt/donations/aistudio',
      expect.anything()
    )
    expect(await screen.findByText('Added to the pool')).toBeInTheDocument()
    // The BUILD pool never reads the email, so the verdict leaves it out.
    expect(
      screen.getByText(
        'The account is now in the BUILD pool and raises your Build-gemini-3.8-flash calls for every day it stays. Thank you!'
      )
    ).toBeInTheDocument()
    expect(screen.queryByText(new RegExp(EMAIL))).not.toBeInTheDocument()
  })

  test('a server refusal shows under the box and keeps the step and the text', async () => {
    vi.mocked(api.post).mockResolvedValueOnce({
      data: {
        success: false,
        message: 'Paste the Cookie of a signed-in AI Studio page',
      },
    })
    renderDialog()
    await agree()
    await donate('not a cookie')
    expect(
      await screen.findByText('Paste the Cookie of a signed-in AI Studio page')
    ).toBeInTheDocument()
    expect(screen.getByText('Step 2 of 3: Cookie')).toBeInTheDocument()
    expect(cookieBox()).toHaveValue('not a cookie')
    expect(onClose).not.toHaveBeenCalled()
  })
})

describe('step 3: the verdict', () => {
  test('while the pool is signing in, the dialog says so and keeps polling', async () => {
    progress = { ...progress, status: 'verifying', email: '' }
    renderDialog()
    await agree()
    await donate()
    expect(
      await screen.findByText(
        'Waiting for the pool to sign in with the account…'
      )
    ).toBeInTheDocument()
    expect(api.get).toHaveBeenCalledWith('/api/wt/donations/12')
    // The step's own Close, not the dialog's corner X (also named Close).
    const stepClose = screen
      .getAllByRole('button', { name: 'Close' })
      .find((button) => button.dataset.slot !== 'dialog-close')
    if (!stepClose) throw new Error('The verify step has no Close button')
    await userEvent.click(stepClose)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  test('a failure shows the reason, and Retry goes back to the Cookie', async () => {
    progress = {
      ...progress,
      status: 'failed',
      email: '',
      fail_reason: 'The pool lost the account; donate again',
    }
    renderDialog()
    await agree()
    await donate()
    expect(await screen.findByText('The donation failed')).toBeInTheDocument()
    expect(
      screen.getByText('The pool lost the account; donate again')
    ).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(screen.getByText('Step 2 of 3: Cookie')).toBeInTheDocument()
    expect(cookieBox()).toHaveValue('')
    expect(onClose).not.toHaveBeenCalled()
  })
})
