import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon, KeyGlassIcon, LinkGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  useDisableWebhook,
  useFunctionTriggers,
  useSetWebhook,
} from '../../queries/functionTriggers'
import type { FunctionItem } from '../../queries/functions'
import { DeadLetters } from './DeadLetters'

function WebhookCard({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('functions')
  const { data, error } = useFunctionTriggers(name)
  const setWebhook = useSetWebhook(name)
  const disable = useDisableWebhook(name)
  const [path, setPath] = useState('/')
  const [secret, setSecret] = useState('')

  if (error) {
    return (
      <p className="text-sm text-destructive">
        {t('triggers.error', { message: error.message })}
      </p>
    )
  }
  if (!data) return null

  const hook = data.webhook
  const fullUrl = hook ? `${window.location.origin}${hook.url}` : ''
  const onError = (err: Error) => {
    toast.add({ title: err.message, type: 'error' })
  }
  const save = (rotate: boolean) => {
    setWebhook.mutate(
      { path: hook ? hook.path : path.trim() || '/', rotate },
      {
        onSuccess: (res) => {
          setSecret(res.secret ?? '')
        },
        onError,
      },
    )
  }
  const copy = (text: string) => {
    void navigator.clipboard.writeText(text)
  }

  return (
    <div className="space-y-4 rounded-lg border p-4">
      <div className="space-y-2">
        <h3 className="flex items-center gap-2 text-sm font-semibold">
          <LinkGlassIcon size="1.2em" aria-hidden="true" />
          {t('triggers.webhook.title')}
        </h3>
        {hook ? (
          <>
            <p className="text-sm text-muted-foreground">
              {t('triggers.webhook.on', {
                path: hook.path,
                seconds: hook.replay_window_seconds,
              })}
            </p>
            <div className="flex items-center gap-2">
              <code className="min-w-0 flex-1 truncate rounded bg-muted px-2 py-1 text-xs">
                {fullUrl}
              </code>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  copy(fullUrl)
                }}
              >
                <CopyGlassIcon size="1.2em" />
                {t('templates.copy')}
              </Button>
            </div>
            {secret ? (
              <div className="space-y-1 rounded bg-muted p-2">
                <p className="text-xs text-muted-foreground">
                  {t('triggers.webhook.secretOnce')}
                </p>
                <div className="flex items-center gap-2">
                  <code className="min-w-0 flex-1 break-all text-xs">
                    {secret}
                  </code>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      copy(secret)
                    }}
                  >
                    <CopyGlassIcon size="1.2em" />
                    {t('templates.copy')}
                  </Button>
                </div>
              </div>
            ) : null}
            <p className="text-xs text-muted-foreground">
              {t('triggers.webhook.signing')}
            </p>
            <div className="flex gap-2">
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={setWebhook.isPending}
                onClick={() => {
                  save(true)
                }}
              >
                <KeyGlassIcon size="1.2em" />
                {t('triggers.webhook.rotate')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                disabled={disable.isPending}
                onClick={() => {
                  setSecret('')
                  disable.mutate(undefined, { onError })
                }}
              >
                {t('triggers.webhook.disable')}
              </Button>
            </div>
          </>
        ) : (
          <div className="flex flex-wrap items-center gap-2">
            <Input
              className="max-w-xs"
              value={path}
              aria-label={t('triggers.webhook.path')}
              onChange={(e) => {
                setPath(e.target.value)
              }}
            />
            <Button
              type="button"
              size="sm"
              disabled={setWebhook.isPending}
              onClick={() => {
                save(false)
              }}
            >
              {t('triggers.webhook.enable')}
            </Button>
          </div>
        )}
      </div>

      <div className="space-y-1">
        <h3 className="text-sm font-semibold">
          {t('triggers.schedules.title')}
        </h3>
        {data.schedules.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('triggers.schedules.none')}
          </p>
        ) : (
          <ul className="text-sm">
            {data.schedules.map((s) => (
              <li key={s.id}>
                <span className="font-medium">{s.name}</span>{' '}
                <span className="font-mono text-xs text-muted-foreground">
                  {s.schedule} {s.timezone}
                </span>
              </li>
            ))}
          </ul>
        )}
        <Link
          to="/apps/$name"
          params={{ name }}
          className="text-xs text-muted-foreground hover:underline"
        >
          {t('triggers.schedules.manage')}
        </Link>
      </div>

      <p className="text-sm text-muted-foreground">
        {t('triggers.queue', {
          queued: data.queue.queued ?? 0,
          running: data.queue.running ?? 0,
          dead: data.queue.dead ?? 0,
        })}
      </p>
    </div>
  )
}

export function FunctionTriggers({
  items,
}: Readonly<{ items: FunctionItem[] }>) {
  const { t } = useTranslation('functions')
  const [selected, setSelected] = useState('')
  const name = selected || items[0]?.name || ''

  return (
    <div className="space-y-6">
      {name ? (
        <section className="space-y-3" aria-labelledby="triggers-title">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 id="triggers-title" className="text-base font-semibold">
              {t('triggers.title')}
            </h2>
            <select
              className="rounded-md border bg-background px-2 py-1 text-sm"
              aria-label={t('triggers.pick')}
              value={name}
              onChange={(e) => {
                setSelected(e.target.value)
              }}
            >
              {items.map((fn) => (
                <option key={fn.name} value={fn.name}>
                  {fn.name}
                </option>
              ))}
            </select>
          </div>
          <WebhookCard key={name} name={name} />
        </section>
      ) : null}
      <DeadLetters />
    </div>
  )
}
