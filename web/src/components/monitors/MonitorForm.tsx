import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { suggestMonitorName } from '@/lib/monitors'
import { useCreateMonitor } from '../../queries/monitors'
import type {
  CreateMonitorInput,
  Monitor,
  MonitorKind,
} from '../../types/monitor'

// Defaults are the common case so a URL alone is enough to save.
const DEFAULT_INTERVAL_SECONDS = 60
const DEFAULT_TIMEOUT_MS = 5000
// Cron jobs and certificates change slowly, so their intervals start long.
const DEFAULT_INTERVAL_BY_KIND: Record<MonitorKind, number> = {
  http: DEFAULT_INTERVAL_SECONDS,
  tcp: DEFAULT_INTERVAL_SECONDS,
  heartbeat: 3600,
  tls: 86400,
}
const DEFAULT_TLS_PORT = '443'
const KINDS: MonitorKind[] = ['http', 'tcp', 'heartbeat', 'tls']

function parseKind(v: string): MonitorKind {
  return KINDS.find((k) => k === v) ?? 'http'
}

function pingCurl(path: string): string {
  return `curl -fsS -m 10 --retry 3 ${window.location.origin}${path}`
}

function PingPanel({
  monitor,
  onDone,
}: {
  monitor: Monitor
  onDone: () => void
}) {
  const { t } = useTranslation('monitors')
  const line = pingCurl(monitor.ping_path ?? '')
  return (
    <div className="space-y-2 rounded-lg border border-border bg-card p-3">
      <p className="text-sm font-medium">{t('form.pingTitle')}</p>
      <div className="flex items-center gap-2">
        <code className="min-w-0 flex-1 rounded bg-muted px-2 py-1 font-mono text-xs break-all">
          {line}
        </code>
        <Button
          type="button"
          size="icon-sm"
          variant="outline"
          aria-label={t('form.copy')}
          onClick={() => {
            void navigator.clipboard.writeText(line).then(() => {
              toast.add({ title: t('form.copied'), type: 'success' })
            })
          }}
        >
          <CopyGlassIcon className="size-5" aria-hidden="true" />
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">{t('form.pingHint')}</p>
      <Button type="button" size="sm" onClick={onDone}>
        {t('form.done')}
      </Button>
    </div>
  )
}

export function MonitorForm({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation('monitors')
  const create = useCreateMonitor()
  const [kind, setKind] = useState<MonitorKind>('http')
  const [url, setUrl] = useState('')
  const [host, setHost] = useState('')
  const [port, setPort] = useState('')
  const [name, setName] = useState('')
  const [nameTouched, setNameTouched] = useState(false)
  const [interval, setIntervalValue] = useState(
    String(DEFAULT_INTERVAL_SECONDS),
  )
  const [timeout, setTimeoutValue] = useState(String(DEFAULT_TIMEOUT_MS))
  const [grace, setGrace] = useState('')
  const [warnDays, setWarnDays] = useState('')
  const [critDays, setCritDays] = useState('')
  const [created, setCreated] = useState<Monitor | null>(null)
  const [statusLink, setStatusLink] = useState(false)
  const hostKind = kind === 'tcp' || kind === 'tls'
  const [error, setError] = useState<string | null>(null)

  const effectiveName =
    nameTouched || kind !== 'http' ? name : name || suggestMonitorName(url)

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    const input: CreateMonitorInput = {
      name: effectiveName || (kind === 'http' ? url : host || kind),
      kind,
      interval_seconds: Number(interval),
      timeout_ms: Number(timeout),
      ...(statusLink ? { status_link: true } : {}),
      ...(kind === 'http' ? { url } : {}),
      ...(hostKind ? { host, port: Number(port) } : {}),
      ...(kind === 'heartbeat' && grace
        ? { grace_seconds: Number(grace) }
        : {}),
      ...(kind === 'tls' && warnDays ? { warn_days: Number(warnDays) } : {}),
      ...(kind === 'tls' && critDays ? { crit_days: Number(critDays) } : {}),
    }
    create.mutate(input, {
      onSuccess: (m) => {
        toast.add({
          title: t('form.created', { name: m.name }),
          type: 'success',
        })
        if (m.ping_path) setCreated(m)
        else onDone()
      },
      onError: (err) => {
        setError(t('form.error', { message: err.message }))
      },
    })
  }

  if (created) return <PingPanel monitor={created} onDone={onDone} />

  return (
    <form
      onSubmit={onSubmit}
      className="flex flex-wrap items-end gap-3 rounded-lg border border-border bg-card p-3"
    >
      <div className="space-y-1">
        <Label htmlFor="monitor-kind">{t('form.kind')}</Label>
        <select
          id="monitor-kind"
          value={kind}
          onChange={(e) => {
            const next = parseKind(e.target.value)
            setKind(next)
            setIntervalValue(String(DEFAULT_INTERVAL_BY_KIND[next]))
            if (next === 'tls' && !port) setPort(DEFAULT_TLS_PORT)
          }}
          className="h-8 rounded-lg border border-input bg-transparent px-2 text-sm focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring dark:bg-input/30"
        >
          <option value="http">{t('form.http')}</option>
          <option value="tcp">{t('form.tcp')}</option>
          <option value="heartbeat">{t('form.heartbeat')}</option>
          <option value="tls">{t('form.tls')}</option>
        </select>
      </div>
      {kind === 'http' ? (
        <div className="min-w-48 flex-1 space-y-1">
          <Label htmlFor="monitor-url">{t('form.url')}</Label>
          <Input
            id="monitor-url"
            type="url"
            required
            placeholder={t('form.urlPlaceholder')}
            value={url}
            onChange={(e) => {
              setUrl(e.target.value)
            }}
          />
        </div>
      ) : null}
      {hostKind ? (
        <>
          <div className="min-w-36 flex-1 space-y-1">
            <Label htmlFor="monitor-host">{t('form.host')}</Label>
            <Input
              id="monitor-host"
              required
              value={host}
              onChange={(e) => {
                setHost(e.target.value)
              }}
            />
          </div>
          <div className="w-24 space-y-1">
            <Label htmlFor="monitor-port">{t('form.port')}</Label>
            <Input
              id="monitor-port"
              type="number"
              min={1}
              max={65535}
              required
              value={port}
              onChange={(e) => {
                setPort(e.target.value)
              }}
            />
          </div>
        </>
      ) : null}
      {kind === 'heartbeat' ? (
        <div className="w-28 space-y-1" title={t('form.graceHint')}>
          <Label htmlFor="monitor-grace">{t('form.grace')}</Label>
          <Input
            id="monitor-grace"
            type="number"
            min={0}
            placeholder={t('form.serverDefault')}
            value={grace}
            onChange={(e) => {
              setGrace(e.target.value)
            }}
          />
        </div>
      ) : null}
      {kind === 'tls' ? (
        <>
          <div className="w-28 space-y-1" title={t('form.daysHint')}>
            <Label htmlFor="monitor-warn">{t('form.warnDays')}</Label>
            <Input
              id="monitor-warn"
              type="number"
              min={1}
              placeholder={t('form.serverDefault')}
              value={warnDays}
              onChange={(e) => {
                setWarnDays(e.target.value)
              }}
            />
          </div>
          <div className="w-28 space-y-1" title={t('form.daysHint')}>
            <Label htmlFor="monitor-crit">{t('form.critDays')}</Label>
            <Input
              id="monitor-crit"
              type="number"
              min={1}
              placeholder={t('form.serverDefault')}
              value={critDays}
              onChange={(e) => {
                setCritDays(e.target.value)
              }}
            />
          </div>
        </>
      ) : null}
      <div className="min-w-32 flex-1 space-y-1">
        <Label htmlFor="monitor-name">{t('form.name')}</Label>
        <Input
          id="monitor-name"
          value={effectiveName}
          onChange={(e) => {
            setName(e.target.value)
            setNameTouched(true)
          }}
        />
      </div>
      <div className="w-24 space-y-1">
        <Label htmlFor="monitor-interval">{t('form.interval')}</Label>
        <Input
          id="monitor-interval"
          type="number"
          min={1}
          value={interval}
          onChange={(e) => {
            setIntervalValue(e.target.value)
          }}
        />
      </div>
      {kind !== 'heartbeat' ? (
        <div className="w-28 space-y-1">
          <Label htmlFor="monitor-timeout">{t('form.timeout')}</Label>
          <Input
            id="monitor-timeout"
            type="number"
            min={1}
            value={timeout}
            onChange={(e) => {
              setTimeoutValue(e.target.value)
            }}
          />
        </div>
      ) : null}
      <div className="flex items-center gap-2 pb-1.5">
        <Checkbox
          id="monitor-status-link"
          checked={statusLink}
          onCheckedChange={(v) => {
            setStatusLink(v)
          }}
        />
        <Label htmlFor="monitor-status-link" title={t('form.statusLinkHint')}>
          {t('form.statusLink')}
        </Label>
      </div>
      <Button type="submit" size="sm" disabled={create.isPending}>
        {create.isPending ? t('form.saving') : t('form.save')}
      </Button>
      {error ? (
        <p role="alert" className="basis-full text-xs text-destructive">
          {error}
        </p>
      ) : null}
    </form>
  )
}
