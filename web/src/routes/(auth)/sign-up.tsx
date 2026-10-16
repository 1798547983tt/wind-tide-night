/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { createFileRoute, redirect } from '@tanstack/react-router'

import { SignUp } from '@/features/auth/sign-up'
import { ensureStatus } from '@/lib/status-query'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/(auth)/sign-up')({
  component: SignUp,
  beforeLoad: async ({ context }) => {
    const { auth } = useAuthStore.getState()

    // 如果已经有用户信息，说明已登录，注册页对其无意义，跳转到 dashboard
    if (auth.user) {
      throw redirect({ to: '/dashboard' })
    }

    // wind-tide: on a Discord-only site the Discord sign-in also asks for
    // entry, so visitors go there. Without a status the page still opens.
    const status = await ensureStatus(context.queryClient).catch(() => null)
    if (status?.wt_discord_only) {
      throw redirect({ to: '/sign-in', replace: true })
    }
  },
})
