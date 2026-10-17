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

import type { DonationProgress } from '../../../types'
import { DonateDialog } from '../donate-dialog'

const AUTHORIZATION_URL =
  'https://accounts.google.com/o/oauth2/v2/auth?client_id=test&state=abc123'
const CALLBACK =
  'http://localhost:51121/oauth-callback?state=abc123&code=4%2Fxyz&scope=email'
const EMAIL = 'spare@example.com'

let progress: DonationProgress
let expiresAt: number
let onClose: Mock<() => void>

function nowSeconds() {
  return Math.floor(Date.now() / 1000)
}

function renderDialog(target: { id: number; email: string } | null = null) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  onClose = vi.fn<() => void>()
  render(
    <QueryClientProvider client={client}>
      <DonateDialog open onClose={onClose} target={target} />
    </QueryClientProvider>
  )
}

function consent() {
  return screen.getByRole('checkbox', {
    name: 'I understand and want to continue',
  })
}

async function agreeAndStart(email = EMAIL) {
  const input = screen.getByLabelText('Google account email')
  if (!(input as HTMLInputElement).readOnly) {
    await userEvent.clear(input)
    await userEvent.type(input, email)
  }
  await userEvent.click(consent())
  await userEvent.click(screen.getByRole('button', { name: 'Start' }))
}

async function pasteAddress(address: string) {
  const box = screen.getByLabelText('Address Google sent you to')
  await userEvent.clear(box)
  await userEvent.click(box)
  await userEvent.paste(address)
}

beforeEach(() => {
  progress = { id: 12, email: EMAIL, status: 'active', fail_reason: '' }
  expiresAt = nowSeconds() + 300
  vi.spyOn(api, 'post').mockImplementation(async (url, body) => {
    if (url === '/api/wt/donations/start') {
      const input = body as { email: string }
      return {
        data: {
          success: true,
          data: {
            flow_token: 'flow-1',
            authorization_url: AUTHORIZATION_URL,
            expires_at: expiresAt,
            email: input.email,
          },
        },
      }
    }
    if (url === '/api/wt/donations/callback') {
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

describe('step 1: the account', () => {
  test('Start stays disabled until the risks are accepted, then fetches the link', async () => {
    renderDialog()
    expect(screen.getByText('Step 1 of 3: Account')).toBeInTheDocument()
    const start = screen.getByRole('button', { name: 'Start' })
    expect(start).toBeDisabled()
    await userEvent.click(consent())
    expect(start).toBeEnabled()
    await userEvent.type(screen.getByLabelText('Google account email'), EMAIL)
    await userEvent.click(start)

    expect(
      await screen.findByText('Step 2 of 3: Authorize')
    ).toBeInTheDocument()
    expect(api.post).toHaveBeenCalledWith('/api/wt/donations/start', {
      email: EMAIL,
    })
    const link = screen.getByText('Open Google sign-in').closest('a')
    expect(link).toHaveAttribute('href', AUTHORIZATION_URL)
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
    expect(
      screen.getByText(/This link expires in [45]:\d\d/)
    ).toBeInTheDocument()
  })

  test('a typo in the email is caught before the server is asked', async () => {
    renderDialog()
    await agreeAndStart('not-an-email')
    expect(
      await screen.findByText('Enter a valid email address')
    ).toBeInTheDocument()
    expect(api.post).not.toHaveBeenCalled()
  })

  test('a server refusal shows inside the dialog and keeps the step', async () => {
    vi.mocked(api.post).mockResolvedValueOnce({
      data: { success: false, message: 'You can donate at most 10 accounts' },
    })
    renderDialog()
    await agreeAndStart()
    expect(
      await screen.findByText('You can donate at most 10 accounts')
    ).toBeInTheDocument()
    expect(screen.getByText('Step 1 of 3: Account')).toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()
  })

  test('a re-authorization locks the email and names the record', async () => {
    renderDialog({ id: 4, email: 'old@example.com' })
    expect(screen.getByText('Authorize the account again')).toBeInTheDocument()
    const input = screen.getByLabelText('Google account email')
    expect(input).toHaveValue('old@example.com')
    expect(input).toHaveAttribute('readonly')
    await agreeAndStart()
    await screen.findByText('Step 2 of 3: Authorize')
    expect(api.post).toHaveBeenCalledWith('/api/wt/donations/start', {
      email: 'old@example.com',
      donation_id: 4,
    })
  })
})

describe('step 2: the callback address', () => {
  test('a doubtful address only gets a hint; the server decides', async () => {
    renderDialog()
    await agreeAndStart()
    await screen.findByText('Step 2 of 3: Authorize')
    expect(
      screen.getByText(/starts with http:\/\/localhost:51121\//)
    ).toBeInTheDocument()

    await pasteAddress('http://localhost:51121/oauth-callback?code=4%2Fxyz')
    expect(screen.getByText(/it has no state parameter/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Submit' })).toBeEnabled()

    await pasteAddress('http://localhost:51121/oauth-callback?state=abc123')
    expect(screen.getByText(/The address has no code yet/)).toBeInTheDocument()

    await pasteAddress(CALLBACK)
    await userEvent.click(screen.getByRole('button', { name: 'Submit' }))
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/wt/donations/callback', {
        flow_token: 'flow-1',
        callback_url: CALLBACK,
      })
    )
    expect(await screen.findByText('Added to the pool')).toBeInTheDocument()
    expect(screen.getByText('Step 3 of 3: Verify')).toBeInTheDocument()
    expect(
      screen.getByText(
        `${EMAIL} is now shared with everyone on this site. Thank you!`
      )
    ).toBeInTheDocument()
    expect(screen.queryByText(/rather than/)).not.toBeInTheDocument()
  })

  test('a refused address shows under the box and keeps the link', async () => {
    renderDialog()
    await agreeAndStart()
    await screen.findByText('Step 2 of 3: Authorize')
    vi.mocked(api.post).mockResolvedValueOnce({
      data: {
        success: false,
        message: 'This address does not belong to the current donation',
      },
    })
    await pasteAddress(CALLBACK)
    await userEvent.click(screen.getByRole('button', { name: 'Submit' }))
    expect(
      await screen.findByText(
        'This address does not belong to the current donation'
      )
    ).toBeInTheDocument()
    expect(screen.getByText('Step 2 of 3: Authorize')).toBeInTheDocument()
    expect(screen.getByLabelText('Address Google sent you to')).toHaveValue(
      CALLBACK
    )
  })

  test('an expired link offers to start again with the email kept', async () => {
    expiresAt = nowSeconds() - 1
    renderDialog()
    await agreeAndStart()
    expect(
      await screen.findByText('This link has expired, start again')
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Submit' })
    ).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Start again' }))
    expect(screen.getByText('Step 1 of 3: Account')).toBeInTheDocument()
    expect(screen.getByLabelText('Google account email')).toHaveValue(EMAIL)
    expect(screen.getByRole('button', { name: 'Start' })).toBeDisabled()
  })
})

describe('step 3: the verdict', () => {
  test('while the pool is verifying, the dialog says so and keeps polling', async () => {
    progress = { ...progress, status: 'verifying' }
    renderDialog()
    await agreeAndStart()
    await screen.findByText('Step 2 of 3: Authorize')
    await pasteAddress(CALLBACK)
    await userEvent.click(screen.getByRole('button', { name: 'Submit' }))
    expect(
      await screen.findByText('Waiting for the pool to confirm the account…')
    ).toBeInTheDocument()
    expect(screen.getByText('Step 3 of 3: Verify')).toBeInTheDocument()
    expect(api.get).toHaveBeenCalledWith('/api/wt/donations/12')
    // The step's own Close, not the dialog's corner X (also named Close).
    const stepClose = screen
      .getAllByRole('button', { name: 'Close' })
      .find((button) => button.dataset.slot !== 'dialog-close')
    if (!stepClose) throw new Error('The verify step has no Close button')
    await userEvent.click(stepClose)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  test('a failure shows the reason, and Retry re-authorizes the same record', async () => {
    progress = {
      ...progress,
      status: 'failed',
      fail_reason: 'Google denied the authorization',
    }
    renderDialog()
    await agreeAndStart()
    await screen.findByText('Step 2 of 3: Authorize')
    await pasteAddress(CALLBACK)
    await userEvent.click(screen.getByRole('button', { name: 'Submit' }))
    expect(await screen.findByText('The donation failed')).toBeInTheDocument()
    expect(
      screen.getByText('Google denied the authorization')
    ).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(screen.getByText('Step 1 of 3: Account')).toBeInTheDocument()
    const input = screen.getByLabelText('Google account email')
    expect(input).toHaveValue(EMAIL)
    expect(input).toHaveAttribute('readonly')
    await agreeAndStart()
    await screen.findByText('Step 2 of 3: Authorize')
    expect(api.post).toHaveBeenLastCalledWith('/api/wt/donations/start', {
      email: EMAIL,
      donation_id: 12,
    })
  })

  test('when Google authorized another account, the dialog says which one', async () => {
    progress = { ...progress, email: 'other@example.com' }
    renderDialog()
    await agreeAndStart()
    await screen.findByText('Step 2 of 3: Authorize')
    await pasteAddress(CALLBACK)
    await userEvent.click(screen.getByRole('button', { name: 'Submit' }))
    expect(await screen.findByText('Added to the pool')).toBeInTheDocument()
    expect(
      screen.getByText(
        `Google authorized other@example.com rather than ${EMAIL}; the pool keeps the account that was authorized.`
      )
    ).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Done' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})
