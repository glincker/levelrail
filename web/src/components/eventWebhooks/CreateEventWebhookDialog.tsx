import { useState } from 'react'
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
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { useCreateEventWebhook } from '../../queries/eventWebhooks'

export function CreateEventWebhookDialog() {
  const { t } = useTranslation('eventWebhooks')
  const create = useCreateEventWebhook()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [events, setEvents] = useState('')
  const [secret, setSecret] = useState<string | null>(null)

  function reset() {
    setName('')
    setUrl('')
    setEvents('')
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
        <PlusGlassIcon className="size-4" aria-hidden="true" />
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
            <Field>
              <FieldLabel htmlFor="hook-name">{t('form.name')}</FieldLabel>
              <Input
                id="hook-name"
                value={name}
                placeholder={t('form.namePlaceholder')}
                onChange={(e) => {
                  setName(e.target.value)
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="hook-url">{t('form.url')}</FieldLabel>
              <Input
                id="hook-url"
                value={url}
                placeholder={t('form.urlPlaceholder')}
                onChange={(e) => {
                  setUrl(e.target.value)
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="hook-events">{t('form.events')}</FieldLabel>
              <Input
                id="hook-events"
                value={events}
                placeholder={t('form.eventsPlaceholder')}
                onChange={(e) => {
                  setEvents(e.target.value)
                }}
              />
              <FieldDescription>{t('form.eventsHint')}</FieldDescription>
            </Field>
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
                  name.trim() === '' || url.trim() === '' || create.isPending
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
