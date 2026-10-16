/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, test } from 'vitest'

import type { SystemStatus } from '@/features/auth/types'
import { statusQueryOptions } from '@/lib/status-query'

import { DiscordSignIn } from '../discord-sign-in'

const status = {
  discord_oauth: true,
  discord_client_id: '100000000000000001',
} as SystemStatus

afterEach(() => {
  localStorage.clear()
})

describe('Discord sign-in steps', () => {
  test('the last step promises balance for checking in without naming the amount', () => {
    const client = new QueryClient()
    client.setQueryData(statusQueryOptions.queryKey, status)

    render(
      <QueryClientProvider client={client}>
        <DiscordSignIn />
      </QueryClientProvider>
    )

    const steps = screen.getAllByRole('listitem')
    expect(steps.at(-1)).toHaveTextContent(
      'Check in every day for bonus balance'
    )
    // How much a check-in brings is a surprise kept for members.
    expect(document.body).not.toHaveTextContent(/9[,. ]?999/)
  })
})
