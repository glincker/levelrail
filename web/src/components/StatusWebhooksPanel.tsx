import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CirclePlusGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import {
  WEBHOOK_EVENTS,
  type StatusWebhook,
  useCreateStatusWebhook,
  useDeleteStatusWebhook,
  useStatusWebhookAction,
  useStatusWebhookDeliveries,
  useStatusWebhooks,
  useTestStatusWebhook,
} from '../queries/statusPage'

function SecretBanner({
  secret,
  onDismiss,
}: {
  secret: string
  onDismiss: () => void
}) {
  const { t } = useTranslation('statusPage')
  return (
    <div
      role="status"
      className="space-y-2 rounded-lg border border-border bg-muted p-3 text-sm"
    >
      <p className="font-medium text-foreground">{t('webhooks.secretTitle')}</p>
      <p className="text-xs text-muted-foreground">
        {t('webhooks.secretHint')}
      </p>
      <code className="block break-all rounded bg-background p-2 font-mono text-xs">
        {secret}
      </code>
      <Button type="button" variant="outline" size="sm" onClick={onDismiss}>
        {t('webhooks.secretDone')}
      </Button>
    </div>
  )
}

function DeliveryLog({ id }: { id: string }) {
  const { t } = useTranslation('statusPage')
  const { data, isLoading } = useStatusWebhookDeliveries(id, true)
  if (isLoading) return <ListSkeleton rows={2} />
  if (!data || data.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">{t('webhooks.logEmpty')}</p>
    )
  }
  return (
    <table className="w-full text-left text-xs">
      <thead className="text-muted-foreground">
        <tr>
          <th className="py-1 pr-2 font-medium">{t('webhooks.logWhen')}</th>
          <th className="py-1 pr-2 font-medium">{t('webhooks.logEvent')}</th>
          <th className="py-1 pr-2 font-medium">{t('webhooks.logState')}</th>
          <th className="py-1 pr-2 font-medium">{t('webhooks.logCode')}</th>
          <th className="py-1 font-medium">{t('webhooks.logError')}</th>
        </tr>
      </thead>
      <tbody className="divide-y divide-border">
        {data.map((d) => (
          <tr key={d.id}>
            <td className="py-1 pr-2">
              {new Date(d.created_at).toLocaleString()}
            </td>
            <td className="py-1 pr-2">{d.event_type}</td>
            <td className="py-1 pr-2">
              {d.state} ({d.attempts})
            </td>
            <td className="py-1 pr-2">{d.status_code || '-'}</td>
            <td className="py-1 break-all">{d.last_error ?? ''}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function WebhookRow({
  hook,
  onSecret,
}: {
  hook: StatusWebhook
  onSecret: (secret: string) => void
}) {
  const { t } = useTranslation('statusPage')
  const [showLog, setShowLog] = useState(false)
  const [testNote, setTestNote] = useState('')
  const action = useStatusWebhookAction()
  const test = useTestStatusWebhook()
  const del = useDeleteStatusWebhook()
  const events =
    hook.events.length === 0 ? t('webhooks.allEvents') : hook.events.join(', ')

  const runTest = () => {
    test.mutate(hook.id, {
      onSuccess: (r) => {
        setTestNote(
          r.delivered
            ? t('webhooks.testOk', { code: r.status_code })
            : t('webhooks.testFailed', { error: r.error ?? '' }),
        )
        setShowLog(true)
      },
    })
  }

  return (
    <li className="space-y-2 py-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-foreground">
            {hook.label || hook.url}
          </p>
          <p className="truncate text-xs text-muted-foreground">{hook.url}</p>
          <p className="text-xs text-muted-foreground">{events}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={hook.paused ? 'muted' : 'success'}>
            {hook.paused ? t('webhooks.paused') : t('webhooks.active')}
          </Badge>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={runTest}
            disabled={test.isPending}
          >
            {t('webhooks.test')}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              action.mutate({
                id: hook.id,
                action: hook.paused ? 'resume' : 'pause',
              })
            }
          >
            {hook.paused ? t('webhooks.resume') : t('webhooks.pause')}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              action.mutate(
                { id: hook.id, action: 'rotate' },
                {
                  onSuccess: (h) =>
                    h.signing_secret && onSecret(h.signing_secret),
                },
              )
            }
          >
            {t('webhooks.rotate')}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setShowLog((v) => !v)}
          >
            {t('webhooks.log')}
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('webhooks.delete', { name: hook.label || hook.url })}
            onClick={() => del.mutate(hook.id)}
          >
            <TrashGlassIcon className="size-5" aria-hidden="true" />
          </Button>
        </div>
      </div>
      {hook.paused && hook.paused_reason ? (
        <p className="text-xs text-muted-foreground">{hook.paused_reason}</p>
      ) : null}
      {testNote ? <p className="text-xs text-foreground">{testNote}</p> : null}
      {showLog ? <DeliveryLog id={hook.id} /> : null}
    </li>
  )
}

export function StatusWebhooksPanel() {
  const { t } = useTranslation('statusPage')
  const { data, isLoading, error } = useStatusWebhooks()
  const create = useCreateStatusWebhook()
  const [url, setUrl] = useState('')
  const [label, setLabel] = useState('')
  const [events, setEvents] = useState<string[]>([])
  const [secret, setSecret] = useState('')

  if (isLoading) return <ListSkeleton rows={2} />
  if (error) return <p className="text-sm text-destructive">{error.message}</p>

  const toggle = (e: string) =>
    setEvents((cur) =>
      cur.includes(e) ? cur.filter((x) => x !== e) : [...cur, e],
    )

  const submit = (ev: React.FormEvent) => {
    ev.preventDefault()
    create.mutate(
      { url: url.trim(), label: label.trim(), events },
      {
        onSuccess: (h) => {
          setSecret(h.signing_secret ?? '')
          setUrl('')
          setLabel('')
          setEvents([])
        },
      },
    )
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">{t('webhooks.intro')}</p>
      {secret ? (
        <SecretBanner secret={secret} onDismiss={() => setSecret('')} />
      ) : null}
      <form
        onSubmit={submit}
        className="space-y-3 rounded-lg border border-border p-3"
      >
        <div className="grid gap-3 sm:grid-cols-[2fr_1fr_auto]">
          <Input
            type="url"
            required
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://example.com/hooks/status"
            aria-label={t('webhooks.url')}
          />
          <Input
            value={label}
            onChange={(e) => setLabel(e.target.value)}
            placeholder={t('webhooks.labelPlaceholder')}
            aria-label={t('webhooks.label')}
            maxLength={80}
          />
          <Button
            type="submit"
            disabled={create.isPending || url.trim() === ''}
          >
            <CirclePlusGlassIcon className="size-5" aria-hidden="true" />
            {t('webhooks.add')}
          </Button>
        </div>
        <fieldset className="flex flex-wrap gap-x-4 gap-y-1">
          <legend className="sr-only">{t('webhooks.events')}</legend>
          {WEBHOOK_EVENTS.map((e) => (
            <label
              key={e}
              className="flex items-center gap-1.5 text-xs text-muted-foreground"
            >
              <input
                type="checkbox"
                checked={events.includes(e)}
                onChange={() => toggle(e)}
              />
              {e}
            </label>
          ))}
        </fieldset>
        <p className="text-xs text-muted-foreground">
          {t('webhooks.eventsHint')}
        </p>
        {create.isError ? (
          <p role="alert" className="text-sm text-destructive">
            {create.error.message}
          </p>
        ) : null}
      </form>
      {data && data.length > 0 ? (
        <ul className="divide-y divide-border">
          {data.map((h) => (
            <WebhookRow key={h.id} hook={h} onSecret={setSecret} />
          ))}
        </ul>
      ) : (
        <p className="text-sm text-muted-foreground">{t('webhooks.empty')}</p>
      )}
    </div>
  )
}
