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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, test } from 'vitest'

import { SignIn } from '../index'

// The sign-in page at its app route id, under the (auth) route group.
function openSignIn(status: Record<string, unknown>) {
  const root = createRootRoute()
  const authGroup = createRoute({ getParentRoute: () => root, id: '(auth)' })
  const signIn = createRoute({
    getParentRoute: () => authGroup,
    path: 'sign-in',
    component: SignIn,
  })
  const router = createRouter({
    routeTree: root.addChildren([authGroup.addChildren([signIn])]),
    history: createMemoryHistory({ initialEntries: ['/sign-in'] }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  // A fresh status entry, so the page reads it without fetching.
  client.setQueryData(['status'], status)
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  )
}

const discordOnly = {
  wt_discord_only: true,
  discord_oauth: true,
  discord_client_id: 'discord-client',
  register_enabled: true,
  password_login_enabled: false,
  password_register_enabled: false,
}

describe('sign-in page of a Discord-only site', () => {
  test('offers Discord as the way in without a separate sign-up link', async () => {
    openSignIn(discordOnly)

    expect(
      await screen.findByRole('button', { name: 'Sign in with Discord' })
    ).toBeEnabled()
    expect(screen.queryByRole('link', { name: 'Sign up' })).toBeNull()
    expect(screen.queryByLabelText('Password')).toBeNull()
  })

  test('a site with a user agreement keeps Discord off until the visitor agrees', async () => {
    openSignIn({ ...discordOnly, user_agreement_enabled: true })

    const button = await screen.findByRole('button', {
      name: 'Sign in with Discord',
    })
    expect(button).toBeDisabled()

    await userEvent.click(screen.getByRole('checkbox'))

    expect(button).toBeEnabled()
  })

  test('before the owner sets Discord up the button is off and says why', async () => {
    openSignIn({ ...discordOnly, discord_oauth: false, discord_client_id: '' })

    expect(
      await screen.findByRole('button', { name: 'Sign in with Discord' })
    ).toBeDisabled()
    expect(
      screen.getByText(
        'Discord sign-in is not ready yet. The site owner still needs to set it up.'
      )
    ).toBeVisible()
  })

  test('a site that also takes registrations keeps the sign-up link', async () => {
    openSignIn({ ...discordOnly, wt_discord_only: false })

    expect(await screen.findByRole('link', { name: 'Sign up' })).toBeVisible()
  })
})
