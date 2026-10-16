/*
Copyright (C) 2026 风与潮之夜 (Night of Wind & Tide) contributors

Part of a modified version of new-api by QuantumNous, licensed under the GNU
Affero General Public License v3.0 or later. See the LICENSE file.
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { handleServerError } from '@/lib/handle-server-error'
import { requireServerSuccess } from '@/lib/server-error-message'

import { addAllowlistEntry } from '../../api'
import { invalidateAccessQueries } from '../../lib/allowlist'
import {
  ALLOWLIST_ENTRY_FORM_DEFAULTS,
  allowlistEntryFormSchema,
  type AllowlistEntryFormValues,
} from '../../lib/allowlist-form'
import { useAllowlist } from './allowlist-provider'

const FORM_ID = 'allowlist-add-form'

export function AllowlistAddDialog() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { open, setOpen } = useAllowlist()
  const form = useForm<AllowlistEntryFormValues>({
    resolver: zodResolver(allowlistEntryFormSchema),
    defaultValues: ALLOWLIST_ENTRY_FORM_DEFAULTS,
  })
  const close = () => {
    setOpen(null)
    form.reset(ALLOWLIST_ENTRY_FORM_DEFAULTS)
  }
  const add = useMutation({
    mutationFn: async (values: AllowlistEntryFormValues) =>
      requireServerSuccess(
        await addAllowlistEntry({
          discord_id: values.discord_id || undefined,
          discord_username: values.discord_username || undefined,
          note: values.note || undefined,
        })
      ),
    onSuccess: async () => {
      toast.success(t('Added to the allowlist'))
      close()
      await invalidateAccessQueries(queryClient)
    },
    onError: (error) =>
      handleServerError(error, t('Failed to add to the allowlist')),
  })

  return (
    <Dialog
      open={open === 'add'}
      onOpenChange={(isOpen) => {
        if (!isOpen && !add.isPending) close()
      }}
      title={t('Add entry')}
      description={t(
        'Fill in a Discord user ID, a username, or both. An entry with only a username locks to the first account that signs in with it.'
      )}
      contentClassName='sm:max-w-lg'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            disabled={add.isPending}
            onClick={close}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={FORM_ID} disabled={add.isPending}>
            {t('Add')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id={FORM_ID}
          className='space-y-4'
          onSubmit={form.handleSubmit((values) => add.mutate(values))}
        >
          <FormField
            control={form.control}
            name='discord_id'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Discord user ID')}</FormLabel>
                <FormControl>
                  <Input {...field} inputMode='numeric' autoComplete='off' />
                </FormControl>
                <FormDescription>
                  {t(
                    '17 to 20 digits. In Discord, turn on Developer Mode, then right-click the person and choose Copy User ID.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='discord_username'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Discord username')}</FormLabel>
                <FormControl>
                  <Input {...field} autoComplete='off' />
                </FormControl>
                <FormDescription>
                  {t('The name after @, not the display name.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='note'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Note')}</FormLabel>
                <FormControl>
                  <Input {...field} autoComplete='off' />
                </FormControl>
                <FormDescription>
                  {t('Optional. Only you can see it.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </form>
      </Form>
    </Dialog>
  )
}
