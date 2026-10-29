import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ActivityGlassIcon } from '@thesvg/ui-react'
import { Badge } from '../ui/badge'
import { Button } from '../ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../ui/card'
import { Field, FieldDescription, FieldLabel } from '../ui/field'
import { SecretInput } from '../ui/secret-input'
import { Switch } from '../ui/switch'
import { toast } from '../ui/toast'
import {
  heartbeatQueryOptions,
  useUpdateHeartbeat,
  type HeartbeatStatus,
} from '../../queries/heartbeat'

function resultVariant(r: HeartbeatStatus['last_result']) {
  if (r === 'ok') return 'success' as const
  if (r === 'error') return 'destructive' as const
  return 'muted' as const
}

function HeartbeatForm({ status }: { status: HeartbeatStatus }) {
  const { t } = useTranslation('settings')
  const save = useUpdateHeartbeat()
  const [url, setUrl] = useState('')
  const [failUrl, setFailUrl] = useState('')
  const [silenceUrl, setSilenceUrl] = useState('')
  const envManaged = status.source === 'env'
  const locked = envManaged || !status.can_store_url
  const silenceLocked = !envManaged && !status.can_store_url

  const done = () => toast.add({ title: t('heartbeat.saved'), type: 'success' })
  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    save.mutate(
      {
        ...(url.trim() ? { url: url.trim() } : {}),
        ...(failUrl.trim() ? { fail_url: failUrl.trim() } : {}),
        ...(silenceUrl.trim() ? { agent_silence_url: silenceUrl.trim() } : {}),
      },
      {
        onSuccess: () => {
          setUrl('')
          setFailUrl('')
          setSilenceUrl('')
          done()
        },
        onError: (err) => toast.add({ title: err.message, type: 'error' }),
      },
    )
  }

  return (
    <form className="space-y-4" onSubmit={submit}>
      {envManaged ? (
        <p className="text-sm text-muted-foreground" role="status">
          {t('heartbeat.envManaged')}
        </p>
      ) : null}
      {!envManaged && !status.can_store_url ? (
        <p className="text-sm text-muted-foreground" role="status">
          {t('heartbeat.noKey')}
        </p>
      ) : null}
      <Field>
        <FieldLabel htmlFor="heartbeat-url">{t('heartbeat.url')}</FieldLabel>
        <SecretInput
          id="heartbeat-url"
          className="min-h-11"
          value={url}
          disabled={locked}
          hasExistingValue={status.configured}
          placeholder={t('heartbeat.urlPlaceholder')}
          onChange={(e) => setUrl(e.target.value)}
        />
        <FieldDescription>
          {status.host
            ? t('heartbeat.urlSet', { host: status.host })
            : t('heartbeat.urlHint')}
        </FieldDescription>
      </Field>
      <Field>
        <FieldLabel htmlFor="heartbeat-fail-url">
          {t('heartbeat.failUrl')}
        </FieldLabel>
        <SecretInput
          id="heartbeat-fail-url"
          className="min-h-11"
          value={failUrl}
          disabled={locked}
          hasExistingValue={Boolean(status.fail_host)}
          onChange={(e) => setFailUrl(e.target.value)}
        />
        <FieldDescription>{t('heartbeat.failUrlHint')}</FieldDescription>
      </Field>
      <Field>
        <FieldLabel htmlFor="heartbeat-agent-silence-url">
          {t('heartbeat.agentSilenceUrl')}
        </FieldLabel>
        <SecretInput
          id="heartbeat-agent-silence-url"
          className="min-h-11"
          value={silenceUrl}
          disabled={silenceLocked}
          hasExistingValue={Boolean(status.agent_silence_host)}
          onChange={(e) => setSilenceUrl(e.target.value)}
        />
        <FieldDescription>
          {status.agent_silence_host
            ? t('heartbeat.agentSilenceSet', {
                host: status.agent_silence_host,
              })
            : t('heartbeat.agentSilenceHint')}
        </FieldDescription>
      </Field>
      <div className="flex min-h-11 items-center justify-between gap-3">
        <FieldLabel htmlFor="heartbeat-body">{t('heartbeat.body')}</FieldLabel>
        <Switch
          id="heartbeat-body"
          checked={status.body_enabled}
          disabled={locked}
          onCheckedChange={(on) =>
            save.mutate({ body_enabled: on }, { onSuccess: done })
          }
        />
      </div>
      <div className="flex flex-wrap gap-2">
        <Button
          type="submit"
          className="min-h-11"
          disabled={
            save.isPending ||
            (locked && silenceLocked) ||
            (!url.trim() && !failUrl.trim() && !silenceUrl.trim())
          }
        >
          {t('heartbeat.save')}
        </Button>
        {status.configured && !envManaged ? (
          <Button
            type="button"
            variant="outline"
            className="min-h-11"
            disabled={save.isPending}
            onClick={() => save.mutate({ url: '' }, { onSuccess: done })}
          >
            {t('heartbeat.clear')}
          </Button>
        ) : null}
        {status.agent_silence_host ? (
          <Button
            type="button"
            variant="outline"
            className="min-h-11"
            disabled={save.isPending}
            onClick={() =>
              save.mutate({ agent_silence_url: '' }, { onSuccess: done })
            }
          >
            {t('heartbeat.clearAgentSilence')}
          </Button>
        ) : null}
      </div>
    </form>
  )
}

export function HeartbeatCard() {
  const { t } = useTranslation('settings')
  const { data } = useQuery(heartbeatQueryOptions())
  if (!data) return null
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ActivityGlassIcon size="1.2em" aria-hidden="true" />
          {t('heartbeat.title')}
        </CardTitle>
        <CardDescription>{t('heartbeat.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {data.configured ? (
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <Badge variant={resultVariant(data.last_result)}>
              {data.last_result
                ? t(`heartbeat.result.${data.last_result}`)
                : t('heartbeat.result.none')}
            </Badge>
            {data.last_ping_at ? (
              <time
                dateTime={data.last_ping_at}
                className="text-muted-foreground"
              >
                {new Date(data.last_ping_at).toLocaleString()}
              </time>
            ) : null}
            {data.last_error ? (
              <span className="text-muted-foreground">{data.last_error}</span>
            ) : null}
          </div>
        ) : null}
        <HeartbeatForm status={data} />
      </CardContent>
    </Card>
  )
}
