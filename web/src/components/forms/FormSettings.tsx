import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { useNotificationChannelsOptional } from '../../queries/notificationChannels'
import { useFormActions, type AppForm } from '../../queries/forms'

export function FormSettings({ app, form }: { app: string; form: AppForm }) {
  const { t } = useTranslation('forms')
  const { save } = useFormActions(app)
  const channels = useNotificationChannelsOptional()
  const [redirect, setRedirect] = useState(form.redirect_url)
  const [honeypot, setHoneypot] = useState(form.honeypot_field)
  const [selected, setSelected] = useState<string[]>(form.notify_channel_ids)

  const toggle = (id: string, on: boolean) =>
    setSelected((cur) => (on ? [...cur, id] : cur.filter((c) => c !== id)))

  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    save.mutate(
      {
        form: form.name,
        input: {
          redirect_url: redirect.trim(),
          honeypot_field: honeypot.trim(),
          notify_channel_ids: selected,
        },
      },
      {
        onSuccess: () =>
          toast.add({ title: t('settings.saved'), type: 'success' }),
      },
    )
  }

  const list = channels.data ?? []
  const idPrefix = `form-${form.id}`
  return (
    <form onSubmit={submit} className="space-y-3">
      <h3 className="text-sm font-medium">{t('settings.title')}</h3>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor={`${idPrefix}-redirect`}>
            {t('settings.redirect')}
          </Label>
          <Input
            id={`${idPrefix}-redirect`}
            type="url"
            value={redirect}
            onChange={(e) => setRedirect(e.target.value)}
            placeholder="https://example.com/thanks"
          />
          <p className="text-xs text-muted-foreground">
            {t('settings.redirectHint')}
          </p>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor={`${idPrefix}-honeypot`}>
            {t('settings.honeypot')}
          </Label>
          <Input
            id={`${idPrefix}-honeypot`}
            value={honeypot}
            onChange={(e) => setHoneypot(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            {t('settings.honeypotHint')}
          </p>
        </div>
      </div>
      <fieldset className="space-y-1.5">
        <legend className="text-sm font-medium">
          {t('settings.channels')}
        </legend>
        {list.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            {t('settings.noChannels')}
          </p>
        ) : (
          list.map((c) => (
            <Label key={c.id} className="font-normal">
              <Checkbox
                checked={selected.includes(c.id)}
                onCheckedChange={(on) => toggle(c.id, on)}
              />
              {c.name}
            </Label>
          ))
        )}
      </fieldset>
      {save.isError && (
        <p role="alert" className="text-sm text-destructive">
          {save.error.message}
        </p>
      )}
      <Button
        type="submit"
        writeAction
        variant="outline"
        disabled={save.isPending}
      >
        {save.isPending ? t('settings.saving') : t('settings.save')}
      </Button>
    </form>
  )
}
