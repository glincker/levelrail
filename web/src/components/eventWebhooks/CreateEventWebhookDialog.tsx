import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { useCreateEventWebhook } from '../../queries/eventWebhooks'
import {
  FIELD_RESOURCES,
  fieldDescriptorsQueryOptions,
} from '../../queries/fieldDescriptors'
import { DescriptorFields } from '../DescriptorFields'

export function CreateEventWebhookDialog() {
  const { t } = useTranslation('eventWebhooks')
  const create = useCreateEventWebhook()
  const [open, setOpen] = useState(false)
  const { data: descriptors } = useQuery({
    ...fieldDescriptorsQueryOptions(FIELD_RESOURCES.eventWebhook),
    retry: false,
  })
  const fields = descriptors?.fields ?? []
  const [values, setValues] = useState<Record<string, string>>({})
  const name = values.name ?? ''
  const url = values.url ?? ''
  const events = values.events ?? ''
  const [secret, setSecret] = useState<string | null>(null)

  function reset() {
    setValues({})
    setSecret(null)
    create.reset()
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const list = events
      .split(',')
      .map((v) => v.trim())
      .filter(Boolean)
    create.mutate(
      {
        name: name.trim(),
        url: url.trim(),
        enabled: true,
        ...(list.length > 0 ? { events: list } : {}),
      },
      {
        onSuccess: (created) => {
          setSecret(created.signing_secret ?? null)
        },
      },
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) reset()
      }}
    >
      <DialogTrigger render={<Button />}>
        <PlusGlassIcon className="size-5" aria-hidden="true" />
        {t('add')}
      </DialogTrigger>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('form.title')}</DialogTitle>
          <DialogDescription>{t('form.description')}</DialogDescription>
        </DialogHeader>
        {secret ? (
          <div className="space-y-4">
            <Alert>
              <AlertDescription className="space-y-1">
                <span className="block">{t('card.secretOnce')}</span>
                <code className="block break-all font-mono text-xs">
                  {secret}
                </code>
              </AlertDescription>
            </Alert>
            <DialogFooter>
              <Button
                onClick={() => {
                  setOpen(false)
                  reset()
                }}
              >
                {t('form.done')}
              </Button>
            </DialogFooter>
          </div>
        ) : (
          <form onSubmit={submit} className="space-y-4">
            {fields.length === 0 ? (
              <Alert variant="destructive">
                <AlertDescription>{t('form.fieldsFailed')}</AlertDescription>
              </Alert>
            ) : (
              <DescriptorFields
                fields={fields}
                values={values}
                onChange={(key, value) => {
                  setValues((cur) => ({ ...cur, [key]: value }))
                }}
                idPrefix="hook"
              />
            )}
            {create.isError ? (
              <Alert variant="destructive">
                <AlertDescription>{create.error.message}</AlertDescription>
              </Alert>
            ) : null}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  setOpen(false)
                }}
              >
                {t('form.cancel')}
              </Button>
              <Button
                type="submit"
                disabled={
                  fields.length === 0 ||
                  name.trim() === '' ||
                  url.trim() === '' ||
                  create.isPending
                }
              >
                {create.isPending ? t('form.submitting') : t('form.submit')}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
