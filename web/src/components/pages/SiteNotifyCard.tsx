import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { useAbilities } from '../../hooks/useAbilities'
import { useNotificationChannelsOptional } from '../../queries/notificationChannels'
import {
  draftFromNotify,
  draftIsValid,
  draftToUpdate,
  usePagesNotify,
  useSavePagesNotify,
  type NotifyDraft,
  type PagesNotify,
} from '../../queries/pagesNotify'

/** Deploy outcome notifications for one site: switch, event filter and an attached channel. */
export default function SiteNotifyCard({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const q = usePagesNotify(name)
  return (
    <section
      aria-labelledby="pages-notify-title"
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <h2
        id="pages-notify-title"
        className="text-sm font-semibold"
        title={t('notify.tip')}
      >
        {t('notify.title')}
      </h2>
      {q.isPending ? <Skeleton className="h-6 w-full" /> : null}
      {q.isError ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{t('notify.failed')}</AlertDescription>
        </Alert>
      ) : null}
      {q.data ? <NotifyForm name={name} data={q.data} /> : null}
    </section>
  )
}

function NotifyForm({
  name,
  data,
}: Readonly<{ name: string; data: PagesNotify }>) {
  const { t } = useTranslation('pagesSites')
  const { can } = useAbilities()
  const save = useSavePagesNotify(name)
  const channels = useNotificationChannelsOptional()
  const [draft, setDraft] = useState<NotifyDraft>(() => draftFromNotify(data))
  const canEdit = can('write')
  const locked = !canEdit || save.isPending
  const list = channels.data ?? []
  const attached = data.channels.map(
    (c) => list.find((ch) => ch.id === c.channel_id)?.name ?? c.channel_id,
  )
  const options = list.filter(
    (ch) => !data.channels.some((c) => c.channel_id === ch.id),
  )

  function patch(p: Partial<NotifyDraft>) {
    setDraft((d) => ({ ...d, ...p }))
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <Label className="gap-2">
          <Switch
            size="sm"
            checked={draft.enabled}
            disabled={locked}
            onCheckedChange={(v) => patch({ enabled: v })}
          />
          {t('notify.enabled')}
        </Label>
        <Label className="gap-2">
          <Checkbox
            checked={draft.failure}
            disabled={locked || !draft.enabled}
            onCheckedChange={(v) => patch({ failure: v === true })}
          />
          {t('notify.onFailure')}
        </Label>
        <Label className="gap-2">
          <Checkbox
            checked={draft.success}
            disabled={locked || !draft.enabled}
            onCheckedChange={(v) => patch({ success: v === true })}
          />
          {t('notify.onSuccess')}
        </Label>
        <Label className="gap-2">
          <span>{t('notify.channel')}</span>
          <NativeSelect
            value={draft.channelId}
            disabled={locked || channels.isPending || options.length === 0}
            onChange={(e) => patch({ channelId: e.target.value })}
          >
            <option value="">{t('notify.pick')}</option>
            {options.map((ch) => (
              <option key={ch.id} value={ch.id}>
                {ch.name}
              </option>
            ))}
          </NativeSelect>
        </Label>
        {canEdit ? (
          <Button
            size="sm"
            disabled={save.isPending || !draftIsValid(draft)}
            onClick={() =>
              save.mutate(draftToUpdate(draft), {
                onSuccess: () => patch({ channelId: '' }),
              })
            }
          >
            {t('notify.save')}
          </Button>
        ) : null}
      </div>
      {!canEdit ? (
        <p className="text-xs text-muted-foreground">{t('notify.readOnly')}</p>
      ) : null}
      {!draftIsValid(draft) ? (
        <p role="alert" className="text-xs text-tone-danger">
          {t('notify.needEvent')}
        </p>
      ) : null}
      {save.error ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {channels.isError ? (
        <p className="text-xs text-muted-foreground">
          {t('notify.channelsFailed')}
        </p>
      ) : null}
      {attached.length > 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('notify.sendingTo', { channels: attached.join(', ') })}
        </p>
      ) : (
        <p className="text-xs text-muted-foreground">
          {options.length === 0 && !channels.isPending
            ? t('notify.noChannels')
            : t('notify.noneAttached')}
        </p>
      )}
    </div>
  )
}
