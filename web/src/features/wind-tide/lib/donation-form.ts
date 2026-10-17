/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { z } from 'zod'

// Messages are i18n keys; FormMessage translates them. The server repeats the
// check (model/wt_donation.go), so this only saves a round trip.

// Enough to catch typos: something@something.tld, no spaces or brackets.
const EMAIL_PATTERN = /^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/
const EMAIL_MAX_LENGTH = 254

export const donateFormSchema = z.object({
  email: z
    .string()
    .trim()
    .refine(
      (email) => email.length <= EMAIL_MAX_LENGTH && EMAIL_PATTERN.test(email),
      { message: 'Enter a valid email address' }
    ),
})

export type DonateFormValues = z.infer<typeof donateFormSchema>

export const DONATE_FORM_DEFAULTS: DonateFormValues = { email: '' }
