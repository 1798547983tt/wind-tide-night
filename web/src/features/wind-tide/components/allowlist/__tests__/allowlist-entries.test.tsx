/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next, { createInstance, type i18n } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest'

import zh from '@/i18n/locales/zh.json'
import { api } from '@/lib/api'

import { allowlistSearchSchema } from '../../../lib/allowlist'
import type { AllowlistEntry } from '../../../types'
import { AllowlistPage } from '../allowlist-page'

const signedInMember: AllowlistEntry = {
  id: 7,
  discord_id: '223456789012345678',
  discord_username: 'shio',
  note: 'from the forum',
  status: 'active',
  user_id: 5,
  account_status: 'enabled',
  display_name: 'Shio',
  avatar_url: '',
  created_at: 1760000000,
  updated_at: 1760000000,
}

// Added by username; nobody has signed in with it yet.
const prefilled: AllowlistEntry = {
  id: 8,
  discord_id: '',
  discord_username: 'nami',
  note: '',
  status: 'active',
  user_id: 0,
  account_status: '',
  display_name: '',
  avatar_url: '',
  created_at: 1760000100,
  updated_at: 1760000100,
}

const removedMember: AllowlistEntry = {
  id: 9,
  discord_id: '323456789012345678',
  discord_username: 'tide',
  note: '',
  status: 'removed',
  user_id: 6,
  account_status: 'disabled',
  display_name: 'Tide',
  avatar_url: '',
  created_at: 1760000200,
  updated_at: 1760000300,
}

// Removed after the linked account was deleted: the server reports no user.
const deletedMember: AllowlistEntry = {
  id: 11,
  discord_id: '523456789012345678',
  discord_username: 'nagi',
  note: '',
  status: 'removed',
  user_id: 0,
  account_status: 'deleted',
  display_name: 'Nagi',
  avatar_url: '',
  created_at: 1760000400,
  updated_at: 1760000500,
}

let entries: AllowlistEntry[]

function renderAllowlistTab(language: i18n = i18next) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const root = createRootRoute()
  const authenticated = createRoute({
    getParentRoute: () => root,
    id: '_authenticated',
  })
  const page = createRoute({
    getParentRoute: () => authenticated,
    path: '/allowlist/',
    validateSearch: allowlistSearchSchema,
    component: AllowlistPage,
  })
  const router = createRouter({
    routeTree: root.addChildren([authenticated.addChildren([page])]),
    history: createMemoryHistory({
      initialEntries: ['/allowlist?tab=allowlist'],
    }),
  })
  render(
    <I18nextProvider i18n={language}>
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

function rowOf(name: string) {
  const row = screen.getByText(name).closest('tr')
  if (!row) throw new Error(`No table row for ${name}`)
  return row
}

async function openRowMenu(name: string) {
  await screen.findByText(name)
  await userEvent.click(
    within(rowOf(name)).getByRole('button', { name: 'Open menu' })
  )
}

beforeEach(() => {
  entries = [signedInMember, prefilled, removedMember, deletedMember]
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    if (url === '/api/wt/allowlist') {
      return {
        data: {
          success: true,
          data: {
            page: 1,
            page_size: 20,
            total: entries.length,
            items: entries,
          },
        },
      }
    }
    if (url === '/api/wt/entry-requests/pending-count') {
      return { data: { success: true, data: { count: 0 } } }
    }
    if (url === '/api/verify/methods') {
      return {
        data: {
          success: true,
          data: {
            scope: 'admin.user.manage',
            methods: [{ method: '2fa', available: true }],
            oauth_providers: [],
            password_encryption_enabled: false,
          },
        },
      }
    }
    throw new Error(`Unexpected GET ${url}`)
  })
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('the member column', () => {
  test('a long name is cut at the column width, so the status and time columns keep their room', async () => {
    const longName =
      'A very long display name that should truncate nicely on phones'
    entries = [{ ...signedInMember, display_name: longName }]
    renderAllowlistTab()

    const name = await screen.findByText(longName)

    expect(name).toHaveClass('truncate')
    expect(name.closest('.max-w-60')).not.toBeNull()
  })
})

describe('the status filter', () => {
  test('opening it lists each status in the interface language without looking the shown text up again', async () => {
    const lookedUpMissing: string[] = []
    const chinese = createInstance()
    await chinese.init({
      lng: 'zh',
      resources: { zh: { translation: zh.translation } },
      initAsync: false,
      saveMissing: true,
      missingKeyHandler: (_languages, _namespace, key) => {
        lookedUpMissing.push(key)
      },
    })
    renderAllowlistTab(chinese)

    await userEvent.click(
      await screen.findByRole('button', { name: zh.translation['Status'] })
    )

    expect(
      await screen.findByRole('option', { name: zh.translation['Active'] })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('option', { name: zh.translation['Removed'] })
    ).toBeInTheDocument()
    expect(lookedUpMissing).not.toContain(zh.translation['Active'])
    expect(lookedUpMissing).not.toContain(zh.translation['Removed'])
  })
})

describe('the account column', () => {
  test('tells a linked, disabled or deleted account apart from one nobody has signed in with', async () => {
    renderAllowlistTab()

    await screen.findByText('Shio')

    expect(within(rowOf('Shio')).getByText('User 5')).toBeInTheDocument()
    expect(within(rowOf('Shio')).queryByText('Disabled')).toBeNull()
    expect(within(rowOf('Tide')).getByText('User 6')).toBeInTheDocument()
    expect(within(rowOf('Tide')).getByText('Disabled')).toBeInTheDocument()
    expect(
      within(rowOf('nami')).getByText('Not signed in yet')
    ).toBeInTheDocument()
    expect(within(rowOf('Nagi')).getByText('Deleted')).toBeInTheDocument()
  })
})

describe('adding an allowlist entry', () => {
  test('without a Discord user ID or username it asks for one and sends nothing', async () => {
    const post = vi.spyOn(api, 'post')
    renderAllowlistTab()

    await userEvent.click(
      await screen.findByRole('button', { name: 'Add entry' })
    )
    const dialog = await screen.findByRole('dialog')
    await userEvent.click(within(dialog).getByRole('button', { name: 'Add' }))

    expect(
      await within(dialog).findByText('Enter a Discord user ID or username')
    ).toBeInTheDocument()
    expect(post).not.toHaveBeenCalled()
  })

  test('a Discord user ID that is not 17 to 20 digits is refused before sending', async () => {
    const post = vi.spyOn(api, 'post')
    renderAllowlistTab()

    await userEvent.click(
      await screen.findByRole('button', { name: 'Add entry' })
    )
    const dialog = await screen.findByRole('dialog')
    await userEvent.type(
      within(dialog).getByLabelText('Discord user ID'),
      '1234567890'
    )
    await userEvent.click(within(dialog).getByRole('button', { name: 'Add' }))

    expect(
      await within(dialog).findByText('A Discord user ID is 17 to 20 digits')
    ).toBeInTheDocument()
    expect(post).not.toHaveBeenCalled()
  })

  test('a valid entry is sent trimmed and the allowlist refreshes to show it', async () => {
    const added: AllowlistEntry = {
      ...prefilled,
      id: 10,
      discord_id: '423456789012345678',
      discord_username: 'umi',
      note: 'met at the event',
    }
    const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url !== '/api/wt/allowlist') throw new Error(`Unexpected POST ${url}`)
      entries = [added, ...entries]
      return { data: { success: true, data: added } }
    })
    renderAllowlistTab()

    await userEvent.click(
      await screen.findByRole('button', { name: 'Add entry' })
    )
    const dialog = await screen.findByRole('dialog')
    await userEvent.type(
      within(dialog).getByLabelText('Discord user ID'),
      ' 423456789012345678 '
    )
    await userEvent.type(
      within(dialog).getByLabelText('Discord username'),
      'umi'
    )
    await userEvent.type(
      within(dialog).getByLabelText('Note'),
      'met at the event'
    )
    await userEvent.click(within(dialog).getByRole('button', { name: 'Add' }))

    await waitFor(() =>
      expect(post).toHaveBeenCalledWith('/api/wt/allowlist', {
        discord_id: '423456789012345678',
        discord_username: 'umi',
        note: 'met at the event',
      })
    )
    expect(await screen.findByText('umi')).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })
})

describe('removing and restoring allowlist entries', () => {
  test('removing an entry with an account asks for verification and sends the proof bound to that account', async () => {
    const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/verify') {
        return {
          data: {
            success: true,
            data: {
              proof_token: 'remove-proof',
              method: '2fa',
              scope: 'admin.user.manage',
              expires_at: Math.floor(Date.now() / 1000) + 60,
            },
          },
        }
      }
      if (url === '/api/wt/allowlist/7/remove') {
        return {
          data: {
            success: true,
            data: { ...signedInMember, status: 'removed' },
          },
        }
      }
      throw new Error(`Unexpected POST ${url}`)
    })
    renderAllowlistTab()

    await openRowMenu('Shio')
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Remove' })
    )
    const confirm = await screen.findByRole('alertdialog')
    expect(
      within(confirm).getByText(
        'This takes effect at once: they are signed out and their API keys stop working. They cannot apply again until you restore them.'
      )
    ).toBeInTheDocument()
    await userEvent.click(
      within(confirm).getByRole('button', { name: 'Remove' })
    )

    const code = await screen.findByLabelText(
      'Authenticator code or backup code'
    )
    expect(screen.queryByRole('alertdialog')).toBeNull()
    expect(post).not.toHaveBeenCalledWith(
      '/api/wt/allowlist/7/remove',
      undefined,
      expect.anything()
    )
    await userEvent.type(code, '123456')
    await userEvent.click(screen.getByRole('button', { name: 'Verify' }))

    await waitFor(() =>
      expect(post).toHaveBeenCalledWith(
        '/api/wt/allowlist/7/remove',
        undefined,
        expect.objectContaining({
          headers: { 'X-Security-Proof': 'remove-proof' },
          singleUseAuthorization: true,
        })
      )
    )
    expect(post).toHaveBeenCalledWith(
      '/api/verify',
      expect.objectContaining({
        scope: 'admin.user.manage',
        context: { user_id: 5, action: 'disable' },
      }),
      expect.anything()
    )
  })

  test('removing an entry nobody has signed in with needs no verification', async () => {
    const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/wt/allowlist/8/remove') {
        return {
          data: { success: true, data: { ...prefilled, status: 'removed' } },
        }
      }
      throw new Error(`Unexpected POST ${url}`)
    })
    renderAllowlistTab()

    await openRowMenu('nami')
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Remove' })
    )
    const confirm = await screen.findByRole('alertdialog')
    await userEvent.click(
      within(confirm).getByRole('button', { name: 'Remove' })
    )

    await waitFor(() =>
      expect(post).toHaveBeenCalledWith(
        '/api/wt/allowlist/8/remove',
        undefined,
        {}
      )
    )
    expect(post).not.toHaveBeenCalledWith(
      '/api/verify',
      expect.anything(),
      expect.anything()
    )
  })

  test('restoring an entry whose account was deleted needs no verification and says signing in creates a new account', async () => {
    const post = vi.spyOn(api, 'post').mockImplementation(async (url) => {
      if (url === '/api/wt/allowlist/11/restore') {
        return {
          data: {
            success: true,
            data: { ...deletedMember, status: 'active', account_status: '' },
          },
        }
      }
      throw new Error(`Unexpected POST ${url}`)
    })
    renderAllowlistTab()

    await openRowMenu('Nagi')
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Restore' })
    )
    const confirm = await screen.findByRole('alertdialog')
    expect(
      within(confirm).getByText(
        'Their old account was deleted. Once restored, signing in with Discord creates a new account.'
      )
    ).toBeInTheDocument()
    await userEvent.click(
      within(confirm).getByRole('button', { name: 'Restore' })
    )

    await waitFor(() =>
      expect(post).toHaveBeenCalledWith(
        '/api/wt/allowlist/11/restore',
        undefined,
        {}
      )
    )
    expect(post).not.toHaveBeenCalledWith(
      '/api/verify',
      expect.anything(),
      expect.anything()
    )
  })

  test('restoring an entry with an account says its API keys stay off', async () => {
    renderAllowlistTab()

    await openRowMenu('Tide')
    await userEvent.click(
      await screen.findByRole('menuitem', { name: 'Restore' })
    )

    // Removal turned the member's keys off; restore brings back the account
    // and balance only (wind-tide/DESIGN.md §3).
    const confirm = await screen.findByRole('alertdialog')
    expect(
      within(confirm).getByText(
        'Their account and balance come back, and they can sign in again. Their API keys stay off until they turn them back on or create new ones.'
      )
    ).toBeInTheDocument()
  })

  test('Restore is offered only for removed entries', async () => {
    renderAllowlistTab()

    await openRowMenu('Shio')
    expect(
      await screen.findByRole('menuitem', { name: 'Remove' })
    ).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'Restore' })).toBeNull()
    await userEvent.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('menu')).toBeNull())

    await openRowMenu('Tide')
    expect(
      await screen.findByRole('menuitem', { name: 'Restore' })
    ).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: 'Remove' })).toBeNull()
  })
})
