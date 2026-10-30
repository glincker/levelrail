import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { suggestMonitorName } from '@/lib/monitors'
import { useCreateMonitor } from '../../queries/monitors'
import type { CreateMonitorInput } from '../../types/monitor'

// Defaults are the common case so a URL alone is enough to save.
const DEFAULT_INTERVAL_SECONDS = 60
const DEFAULT_TIMEOUT_MS = 5000

export function MonitorForm({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation('monitors')
  const create = useCreateMonitor()
  const [kind, setKind] = useState<'http' | 'tcp'>('http')
  const [url, setUrl] = useState('')
  const [host, setHost] = useState('')
  const [port, setPort] = useState('')
  const [name, setName] = useState('')
  const [nameTouched, setNameTouched] = useState(false)
  const [interval, setIntervalValue] = useState(
    String(DEFAULT_INTERVAL_SECONDS),
  )
  const [timeout, setTimeoutValue] = useState(String(DEFAULT_TIMEOUT_MS))
  const [statusLink, setStatusLink] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const effectiveName =
    nameTouched || kind === 'tcp' ? name : name || suggestMonitorName(url)

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    const input: CreateMonitorInput = {
      name: effectiveName || (kind === 'tcp' ? host : url),
      kind,
      interval_seconds: Number(interval),
      timeout_ms: Number(timeout),
      ...(statusLink ? { status_link: true } : {}),
      ...(kind === 'http' ? { url } : { host, port: Number(port) }),
    }
    create.mutate(input, {
      onSuccess: (m) => {
        toast.add({
          title: t('form.created', { name: m.name }),
          type: 'success',
        })
        onDone()
      },
      onError: (err) => {
        setError(t('form.error', { message: err.message }))
      },
    })
  }

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
            setKind(e.target.value === 'tcp' ? 'tcp' : 'http')
          }}
          className="h-8 rounded-lg border border-input bg-transparent px-2 text-sm focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring dark:bg-input/30"
        >
          <option value="http">{t('form.http')}</option>
          <option value="tcp">{t('form.tcp')}</option>
        </select>
      </div>
      {kind === 'http' ? (
        <div className="min-w-48 flex-1 space-y-1">
          <Label htmlFor="monitor-url">{t('form.url')}</Label>
          <Input
            id="monitor-url"
            type="url"
            required
            placeholder="https://example.com/health"
            value={url}
            onChange={(e) => {
              setUrl(e.target.value)
            }}
          />
        </div>
      ) : (
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
      )}
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
