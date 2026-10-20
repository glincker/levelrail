import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  useChannelQuietHours,
  useSaveChannelQuietHours,
  type QuietHours,
} from '../queries/alertSetup'

function QuietHoursForm({
  channelId,
  initial,
}: {
  channelId: string
  initial: QuietHours
}) {
  const { t } = useTranslation('alertSetup')
  const save = useSaveChannelQuietHours(channelId)
  const [form, setForm] = useState<QuietHours>({
    ...initial,
    start: initial.start || '22:00',
    end: initial.end || '07:00',
    timezone:
      initial.timezone ||
      Intl.DateTimeFormat().resolvedOptions().timeZone ||
      'UTC',
  })

  async function run(
    value: QuietHours | null,
    doneKey: 'quiet.saved' | 'quiet.cleared',
  ) {
    try {
      await save.mutateAsync(value)
      toast.add({ title: t(doneKey), type: 'success' })
    } catch (err) {
      toast.add({
        title: err instanceof Error ? err.message : t('quiet.saveError'),
        type: 'error',
      })
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <Field orientation="horizontal">
        <Switch
          id={`quiet-enabled-${channelId}`}
          checked={form.enabled}
          onCheckedChange={(enabled) => setForm({ ...form, enabled })}
        />
        <FieldLabel htmlFor={`quiet-enabled-${channelId}`}>
          {t('quiet.enabled')}
        </FieldLabel>
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field>
          <FieldLabel htmlFor={`quiet-start-${channelId}`}>
            {t('quiet.start')}
          </FieldLabel>
          <Input
            id={`quiet-start-${channelId}`}
            type="time"
            value={form.start}
            onChange={(e) => setForm({ ...form, start: e.target.value })}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`quiet-end-${channelId}`}>
            {t('quiet.end')}
          </FieldLabel>
          <Input
            id={`quiet-end-${channelId}`}
            type="time"
            value={form.end}
            onChange={(e) => setForm({ ...form, end: e.target.value })}
          />
        </Field>
      </div>
      <Field>
        <FieldLabel htmlFor={`quiet-tz-${channelId}`}>
          {t('quiet.timezone')}
        </FieldLabel>
        <Input
          id={`quiet-tz-${channelId}`}
          value={form.timezone}
          onChange={(e) => setForm({ ...form, timezone: e.target.value })}
        />
        <FieldHint>{t('quiet.description')}</FieldHint>
      </Field>
      <div className="flex gap-2">
        <Button
          type="button"
          disabled={save.isPending}
          onClick={() => void run(form, 'quiet.saved')}
        >
          {save.isPending ? t('quiet.saving') : t('quiet.save')}
        </Button>
        {initial.start ? (
          <Button
            type="button"
            variant="ghost"
            disabled={save.isPending}
            onClick={() => void run(null, 'quiet.cleared')}
          >
            {t('quiet.clear')}
          </Button>
        ) : null}
      </div>
    </div>
  )
}

// Daily window during which a channel holds non-critical firing alerts.
export function ChannelQuietHours({ channelId }: { channelId: string }) {
  const { t } = useTranslation('alertSetup')
  const quiet = useChannelQuietHours(channelId)
  return (
    <section className="flex flex-col gap-3 border-t border-border pt-4">
      <h3 className="text-sm font-medium text-foreground">
        {t('quiet.title')}
      </h3>
      {quiet.isError ? (
        <p className="text-sm text-destructive">{t('quiet.loadError')}</p>
      ) : quiet.data ? (
        <QuietHoursForm
          key={`${quiet.data.start}-${quiet.data.end}-${quiet.data.enabled}`}
          channelId={channelId}
          initial={quiet.data}
        />
      ) : null}
    </section>
  )
}
