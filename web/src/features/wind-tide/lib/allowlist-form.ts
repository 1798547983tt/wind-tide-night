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
const USERNAME_MAX_LENGTH = 64
const NOTE_MAX_LENGTH = 255

/** The server drops a leading @ the same way. */
function bareUsername(value: string): string {
  return value.replace(/^@/, '')
}

const noteSchema = z
  .string()
  .trim()
  // The server counts characters, not UTF-16 code units.
  .refine((note) => [...note].length <= NOTE_MAX_LENGTH, {
    message: 'Note must be 255 characters or fewer',
  })

export const allowlistEntryFormSchema = z
  .object({
    discord_id: z
      .string()
      .trim()
      .refine((id) => id === '' || DISCORD_ID_PATTERN.test(id), {
        message: 'A Discord user ID is 17 to 20 digits',
      }),
    discord_username: z
      .string()
      .trim()
      .refine(
        (username) => [...bareUsername(username)].length <= USERNAME_MAX_LENGTH,
        { message: 'Username must be 64 characters or fewer' }
      ),
    note: noteSchema,
  })
  .superRefine((values, ctx) => {
    if (
      values.discord_id === '' &&
      bareUsername(values.discord_username) === ''
    ) {
      ctx.addIssue({
        code: 'custom',
        path: ['discord_id'],
        message: 'Enter a Discord user ID or username',
      })
    }
  })

export type AllowlistEntryFormValues = z.infer<typeof allowlistEntryFormSchema>

export const ALLOWLIST_ENTRY_FORM_DEFAULTS: AllowlistEntryFormValues = {
  discord_id: '',
  discord_username: '',
  note: '',
}

export const allowlistNoteFormSchema = z.object({ note: noteSchema })

export type AllowlistNoteFormValues = z.infer<typeof allowlistNoteFormSchema>
