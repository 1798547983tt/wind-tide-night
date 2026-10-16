/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { createInstance } from 'i18next'
import { describe, expect, it } from 'vitest'

import { getBillingSectionNavItems } from '../section-registry'

describe('billing settings sections', () => {
  it('offer no payment gateway on a free relay', async () => {
    const i18n = createInstance()
    await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })

    const titles = getBillingSectionNavItems(i18n.t).map((item) => item.title)

    expect(titles).toContain('Quota Settings')
    expect(titles).toContain('Check-in Rewards')
    expect(titles).not.toContain('Payment Gateway')
  })
})
