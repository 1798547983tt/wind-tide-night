/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { z } from 'zod'

// Messages are i18n keys; FormMessage translates them. The server repeats
// every check (model/wt_access.go), so these only save a round trip.

// Discord user IDs are snowflakes: 17 to 20 digits.
const DISCORD_ID_PATTERN = /^\d{17,20}$/
const NOTE_MAX_LENGTH = 255

const noteSchema = z
  .string()
  .trim()
  // The server counts characters, not UTF-16 code units.
  .refine((note) => [...note].length <= NOTE_MAX_LENGTH, {
    message: 'Note must be 255 characters or fewer',
  })

// Entries name a Discord user ID only: anyone can take a Discord username
// nobody holds, so a name identifies nobody.
export const allowlistEntryFormSchema = z.object({
  discord_id: z
    .string()
    .trim()
    .min(1, { message: 'Enter a Discord user ID' })
    .regex(DISCORD_ID_PATTERN, {
      message: 'A Discord user ID is 17 to 20 digits',
    }),
  note: noteSchema,
})

export type AllowlistEntryFormValues = z.infer<typeof allowlistEntryFormSchema>

export const ALLOWLIST_ENTRY_FORM_DEFAULTS: AllowlistEntryFormValues = {
  discord_id: '',
  note: '',
}

export const allowlistNoteFormSchema = z.object({ note: noteSchema })

export type AllowlistNoteFormValues = z.infer<typeof allowlistNoteFormSchema>
