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

import type { DonationProgress, WbDonationStart } from '../../../types'
import { WbDonateDialog } from '../wb-donate-dialog'

const LINK = 'https://copilot.tencent.com/oauth?state=abc123'

let started: WbDonationStart
let progress: DonationProgress
let onClose: Mock<() => void>

function renderDialog() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  onClose = vi.fn<() => void>()
  render(
    <QueryClientProvider client={client}>
      <WbDonateDialog open onClose={onClose} />
    </QueryClientProvider>
  )
}

function consent() {
  return screen.getByRole('checkbox', {
    name: 'I understand and want to continue',
  })
}

async function agree() {
  await userEvent.click(consent())
  await userEvent.click(screen.getByRole('button', { name: 'Continue' }))
}

beforeEach(() => {
  started = {
    id: 12,
    status: 'verifying',
    authorization_url: LINK,
    expires_at: Math.floor(Date.now() / 1000) + 15 * 60,
  }
  progress = {
    id: 12,
    email: '',
    nickname: 'Alpha',
    status: 'active',
    fail_reason: '',
  }
  vi.spyOn(api, 'post').mockImplementation(async (url) => {
    if (url === '/api/wt/donations/workbuddy') {
      return { data: { success: true, data: started } }
    }
    throw new Error(`Unexpected POST ${url}`)
  })
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === `/api/wt/donations/${progress.id}`) {
      return { data: { success: true, data: progress } }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('step 1: the risks and the sign-in', () => {
  test('Continue stays disabled until the risks are accepted, and CodeBuddy is the default', async () => {
    renderDialog()
    expect(screen.getByText('Step 1 of 3: Consent')).toBeInTheDocument()
    expect(
      screen.getByText(/Tencent may rate-limit, risk-control or ban accounts/)
    ).toBeInTheDocument()
    expect(
      screen.getByRole('radio', { name: 'CodeBuddy (China)' })
    ).toBeChecked()
    expect(
      screen.getByRole('radio', { name: 'WorkBuddy (international)' })
    ).not.toBeChecked()
    const next = screen.getByRole('button', { name: 'Continue' })
    expect(next).toBeDisabled()
    await userEvent.click(consent())
    expect(next).toBeEnabled()
    expect(api.post).not.toHaveBeenCalled()
  })

  test('Continue asks the pool for a sign-in of the chosen realm', async () => {
    progress = { ...progress, status: 'verifying', nickname: '' }
    renderDialog()
    await userEvent.click(
      screen.getByRole('radio', { name: 'WorkBuddy (international)' })
    )
    await agree()
    await waitFor(() =>
      expect(api.post).toHaveBeenCalledWith('/api/wt/donations/workbuddy', {
        realm: 'global',
        accepted: true,
      })
    )
    expect(await screen.findByText('Step 2 of 3: Sign in')).toBeInTheDocument()
  })
})

describe('step 2: the Tencent link', () => {
  test('shows the link to open or copy, a countdown, and waits for the sign-in', async () => {
    progress = { ...progress, status: 'verifying', nickname: '' }
    renderDialog()
    await agree()
    // The anchor is a Button rendered as a link, so it announces as a button.
    const open = await screen.findByRole('button', {
      name: 'Open Tencent sign-in',
    })
    expect(open).toHaveAttribute('href', LINK)
    expect(open).toHaveAttribute('target', '_blank')
    expect(
      screen.getByRole('button', { name: 'Copy to clipboard' })
    ).toHaveTextContent('Copy link')
    expect(
      screen.getByText('Waiting for you to finish the sign-in…')
    ).toBeInTheDocument()
    expect(
      screen.getByText(/^This link expires in 1[45]:\d\d$/)
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Start again' })
    ).not.toBeInTheDocument()
    // The record is polled while the pool waits for Tencent.
    await waitFor(() =>
      expect(api.get).toHaveBeenCalledWith('/api/wt/donations/12')
    )
    expect(screen.getByText('Step 2 of 3: Sign in')).toBeInTheDocument()
    expect(onClose).not.toHaveBeenCalled()
  })

  test('a refusal from the pool shows in place, and Try again asks once more', async () => {
    progress = { ...progress, status: 'verifying', nickname: '' }
    vi.mocked(api.post).mockResolvedValueOnce({
      data: {
        success: false,
        message:
          'WorkBuddy did not hand out a sign-in link; try again in a moment',
      },
    })
    renderDialog()
    await agree()
    expect(
      await screen.findByText(
        'WorkBuddy did not hand out a sign-in link; try again in a moment'
      )
    ).toBeInTheDocument()
    expect(screen.getByText('Step 2 of 3: Sign in')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Open Tencent sign-in' })
    ).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(
      await screen.findByRole('button', { name: 'Open Tencent sign-in' })
    ).toBeInTheDocument()
    expect(api.post).toHaveBeenCalledTimes(2)
    expect(api.post).toHaveBeenLastCalledWith('/api/wt/donations/workbuddy', {
      realm: 'cn',
      accepted: true,
    })
  })

  test('an expired link says so and Start again fetches a fresh one', async () => {
    progress = { ...progress, status: 'verifying', nickname: '' }
    started = { ...started, expires_at: Math.floor(Date.now() / 1000) - 1 }
    renderDialog()
    await agree()
    expect(
      await screen.findByText('This link has expired, start again')
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Open Tencent sign-in' })
    ).not.toBeInTheDocument()
    started = {
      ...started,
      id: 13,
      expires_at: Math.floor(Date.now() / 1000) + 15 * 60,
    }
    progress = { ...progress, id: 13 }
    await userEvent.click(screen.getByRole('button', { name: 'Start again' }))
    expect(
      await screen.findByRole('button', { name: 'Open Tencent sign-in' })
    ).toBeInTheDocument()
    expect(api.post).toHaveBeenCalledTimes(2)
    await waitFor(() =>
      expect(api.get).toHaveBeenCalledWith('/api/wt/donations/13')
    )
  })
})

describe('step 3: the verdict', () => {
  test('once the pool has the account, the dialog names it and Done closes', async () => {
    renderDialog()
    await agree()
    expect(await screen.findByText('Step 3 of 3: Verify')).toBeInTheDocument()
    expect(screen.getByText('Added to the pool')).toBeInTheDocument()
    expect(
      screen.getByText(
        'Alpha is now in the WorkBuddy pool and its credits are shared with everyone on this site. Thank you!'
      )
    ).toBeInTheDocument()
    expect(api.get).toHaveBeenCalledWith('/api/wt/donations/12')
    await userEvent.click(screen.getByRole('button', { name: 'Done' }))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  test('without a nickname the verdict still says the credits are shared', async () => {
    progress = { ...progress, nickname: '' }
    renderDialog()
    await agree()
    expect(
      await screen.findByText(
        'The account is now in the WorkBuddy pool and its credits are shared with everyone on this site. Thank you!'
      )
    ).toBeInTheDocument()
  })

  test('a failure shows the reason, and Retry starts a fresh sign-in', async () => {
    progress = {
      ...progress,
      status: 'failed',
      nickname: '',
      fail_reason: 'The sign-in was not finished in time; start again',
    }
    renderDialog()
    await agree()
    expect(await screen.findByText('The donation failed')).toBeInTheDocument()
    expect(
      screen.getByText('The sign-in was not finished in time; start again')
    ).toBeInTheDocument()
    // The server hands out a new record for the fresh attempt.
    started = { ...started, id: 13 }
    progress = { ...progress, id: 13, status: 'verifying', fail_reason: '' }
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('Step 2 of 3: Sign in')).toBeInTheDocument()
    expect(
      await screen.findByRole('button', { name: 'Open Tencent sign-in' })
    ).toBeInTheDocument()
    expect(api.post).toHaveBeenCalledTimes(2)
    expect(onClose).not.toHaveBeenCalled()
  })
})
