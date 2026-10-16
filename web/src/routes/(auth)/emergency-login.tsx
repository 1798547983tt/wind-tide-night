/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { createFileRoute } from '@tanstack/react-router'

import { EmergencyLoginPage } from '@/features/wind-tide/components/emergency-login/emergency-login-page'

export const Route = createFileRoute('/(auth)/emergency-login')({
  component: EmergencyLoginPage,
})
