/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { updateAllowlistNote } from '../../api'
import { invalidateAccessQueries, memberName } from '../../lib/allowlist'
import {
  allowlistNoteFormSchema,
  type AllowlistNoteFormValues,
} from '../../lib/allowlist-form'
import { useAllowlist } from './allowlist-provider'

const FORM_ID = 'allowlist-note-form'

export function AllowlistNoteDialog() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { open, setOpen, currentEntry } = useAllowlist()
  const isOpen = open === 'edit-note' && currentEntry !== null
  const form = useForm<AllowlistNoteFormValues>({
    resolver: zodResolver(allowlistNoteFormSchema),
    defaultValues: { note: '' },
  })

  // Start from the entry's saved note every time the dialog opens.
  useEffect(() => {
    if (isOpen) form.reset({ note: currentEntry.note })
  }, [isOpen, currentEntry, form])

  const save = useMutation({
    mutationFn: async (values: { id: number; note: string }) =>
      requireServerSuccess(await updateAllowlistNote(values.id, values.note)),
    onSuccess: async () => {
      toast.success(t('Note saved'))
      setOpen(null)
      await invalidateAccessQueries(queryClient)
    },
    onError: (error) => handleServerError(error, t('Failed to save the note')),
  })

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(nextOpen) => {
        if (!nextOpen && !save.isPending) setOpen(null)
      }}
      title={t('Edit note')}
      description={currentEntry ? memberName(currentEntry) : undefined}
      contentClassName='sm:max-w-lg'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            disabled={save.isPending}
            onClick={() => setOpen(null)}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={FORM_ID} disabled={save.isPending}>
            {t('Save')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id={FORM_ID}
          className='space-y-4'
          onSubmit={form.handleSubmit((values) => {
            if (currentEntry) {
              save.mutate({ id: currentEntry.id, note: values.note })
            }
          })}
        >
          <FormField
            control={form.control}
            name='note'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Note')}</FormLabel>
                <FormControl>
                  <Input {...field} autoComplete='off' />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </form>
      </Form>
    </Dialog>
  )
}
