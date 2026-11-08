import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  SlidersHorizontalGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { useAbilities } from '../../hooks/useAbilities'
import {
  runtimeOptionsQueryOptions,
  useSetRuntimeOptions,
  type RuntimeOptionsResource,
  type RuntimeOptionValues,
  type RuntimeUlimit,
} from '../../queries/runtimeOptions'

// Lazy panel: the query module is imported only here, never from a route loader.
// With several services a native select picks which one the card edits.
export default function RuntimeOptionsPanel({
  appName,
  services,
}: {
  appName: string
  services: string[]
}) {
  const { t } = useTranslation('runtimeOptions')
  const [picked, setPicked] = useState(services[0] ?? appName)
  const selectId = useId()
  const service = services.includes(picked) ? picked : (services[0] ?? appName)
  return (
    <div className="space-y-2">
      {services.length > 1 && (
        <div className="flex items-center gap-2">
          <Label htmlFor={selectId}>{t('service')}</Label>
          <select
            id={selectId}
            value={service}
            onChange={(e) => setPicked(e.target.value)}
            className="h-8 rounded-md border border-input bg-transparent px-2 text-sm"
          >
            {services.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </div>
      )}
      <RuntimeOptionsCard key={service} appName={appName} service={service} />
    </div>
  )
}

export function RuntimeOptionsCard({
  appName,
  service,
}: {
  appName: string
  service: string
}) {
  const query = useQuery(runtimeOptionsQueryOptions(appName, service))
  if (!query.data) {
    return null
  }
  return (
    <RuntimeOptionsForm
      key={query.data.revision}
      appName={appName}
      service={service}
      resource={query.data}
    />
  )
}

const lines = (v: string[] | undefined): string => (v ?? []).join('\n')
const splitLines = (v: string): string[] =>
  v
    .split('\n')
    .map((l) => l.trim())
    .filter((l) => l !== '')

function ulimitsToText(u: Record<string, RuntimeUlimit> | undefined): string {
  return Object.entries(u ?? {})
    .map(([k, v]) => `${k}=${v.soft}:${v.hard}`)
    .join('\n')
}

function sysctlsToText(s: Record<string, string> | undefined): string {
  return Object.entries(s ?? {})
    .map(([k, v]) => `${k}=${v}`)
    .join('\n')
}

// null means a line did not parse; the server still checks names and bounds.
function parseUlimits(text: string): Record<string, RuntimeUlimit> | null {
  const out: Record<string, RuntimeUlimit> = {}
  for (const line of splitLines(text)) {
    const m = /^([a-z]+)\s*=\s*(-?\d+)(?::(-?\d+))?$/.exec(line)
    const [, name, softText, hardText] = m ?? []
    if (!name || !softText) return null
    const soft = Number(softText)
    out[name] = { soft, hard: hardText === undefined ? soft : Number(hardText) }
  }
  return out
}

function parseSysctls(text: string): Record<string, string> | null {
  const out: Record<string, string> = {}
  for (const line of splitLines(text)) {
    const i = line.indexOf('=')
    if (i < 1) return null
    out[line.slice(0, i).trim()] = line.slice(i + 1).trim()
  }
  return out
}

function RuntimeOptionsForm({
  appName,
  service,
  resource,
}: {
  appName: string
  service: string
  resource: RuntimeOptionsResource
}) {
  const { t } = useTranslation('runtimeOptions')
  const { can } = useAbilities()
  const writable = can('write:sensitive')
  const save = useSetRuntimeOptions(appName, service)
  const base = useId()
  const { runtime, editable, bounds } = resource
  const has = (key: string) => editable.includes(key)

  const [tmpfs, setTmpfs] = useState(lines(runtime.tmpfs))
  const [ulimits, setUlimits] = useState(ulimitsToText(runtime.ulimits))
  const [init, setInit] = useState(runtime.init ?? false)
  const [stop, setStop] = useState(
    runtime.stop_timeout === undefined ? '' : String(runtime.stop_timeout),
  )
  const [hosts, setHosts] = useState(lines(runtime.extra_hosts))
  const [dns, setDns] = useState(lines(runtime.dns))
  const [sysctls, setSysctls] = useState(sysctlsToText(runtime.sysctls))
  const [pids, setPids] = useState(
    runtime.pids_limit ? String(runtime.pids_limit) : '',
  )
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [failure, setFailure] = useState<string | null>(null)

  const submit = () => {
    const next: Record<string, string> = {}
    const values: RuntimeOptionValues = {}
    if (has('tmpfs')) values.tmpfs = splitLines(tmpfs)
    if (has('ulimits')) {
      const parsed = parseUlimits(ulimits)
      if (parsed === null) next.ulimits = t('badUlimit')
      else values.ulimits = parsed
    }
    if (has('init')) values.init = init
    if (has('stop_timeout')) {
      if (stop.trim() !== '') values.stop_timeout = Number(stop)
    }
    if (has('extra_hosts')) values.extra_hosts = splitLines(hosts)
    if (has('dns')) values.dns = splitLines(dns)
    if (has('sysctls')) {
      const parsed = parseSysctls(sysctls)
      if (parsed === null) next.sysctls = t('badSysctl')
      else values.sysctls = parsed
    }
    if (has('pids_limit') && pids.trim() !== '') {
      values.pids_limit = Number(pids)
    }
    setFailure(null)
    setErrors(next)
    if (Object.keys(next).length > 0) return
    save.mutate(
      { revision: resource.revision, values },
      {
        onSuccess: () => toast.add({ title: t('saved'), type: 'success' }),
        onError: (e) => {
          setErrors(e.fieldErrors)
          if (Object.keys(e.fieldErrors).length === 0) setFailure(e.message)
        },
      },
    )
  }

  const field = (
    key: string,
    label: string,
    hint: string,
    el: React.ReactNode,
  ) =>
    has(key) && (
      <div className="space-y-1">
        <Label htmlFor={`${base}-${key}`}>{label}</Label>
        {el}
        <p
          id={`${base}-${key}-msg`}
          className={
            errors[key]
              ? 'text-xs text-destructive'
              : 'text-xs text-muted-foreground'
          }
          role={errors[key] ? 'alert' : undefined}
        >
          {errors[key] ?? hint}
        </p>
      </div>
    )

  const area = (key: string, value: string, set: (v: string) => void) => (
    <Textarea
      id={`${base}-${key}`}
      rows={2}
      value={value}
      onChange={(e) => set(e.target.value)}
      disabled={!writable || save.isPending}
      aria-invalid={errors[key] ? true : undefined}
      aria-describedby={`${base}-${key}-msg`}
      spellCheck={false}
      className="font-mono text-xs"
    />
  )

  const num = (key: string, value: string, set: (v: string) => void) => (
    <Input
      id={`${base}-${key}`}
      type="number"
      inputMode="numeric"
      min={0}
      value={value}
      onChange={(e) => set(e.target.value)}
      disabled={!writable || save.isPending}
      aria-invalid={errors[key] ? true : undefined}
      aria-describedby={`${base}-${key}-msg`}
    />
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <SlidersHorizontalGlassIcon className="size-5 text-muted-foreground" />
          {t('title', { service })}
        </CardTitle>
        <CardDescription>{t('description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 md:grid-cols-2">
          {field(
            'tmpfs',
            t('tmpfs.label'),
            t('tmpfs.hint', { max: bounds.tmpfs_max }),
            area('tmpfs', tmpfs, setTmpfs),
          )}
          {field(
            'ulimits',
            t('ulimits.label'),
            t('ulimits.hint', {
              max: bounds.ulimit_count_max,
              names: bounds.ulimits.join(', '),
            }),
            area('ulimits', ulimits, setUlimits),
          )}
          {field(
            'extra_hosts',
            t('extraHosts.label'),
            t('extraHosts.hint', { max: bounds.extra_hosts_max }),
            area('extra_hosts', hosts, setHosts),
          )}
          {field(
            'dns',
            t('dns.label'),
            t('dns.hint', { max: bounds.dns_max }),
            area('dns', dns, setDns),
          )}
          {field(
            'sysctls',
            t('sysctls.label'),
            t('sysctls.hint', { names: bounds.sysctls.join(', ') }),
            area('sysctls', sysctls, setSysctls),
          )}
          <div className="grid grid-cols-2 gap-4">
            {field(
              'stop_timeout',
              t('stopTimeout.label'),
              t('stopTimeout.hint', { max: bounds.stop_timeout_max_seconds }),
              num('stop_timeout', stop, setStop),
            )}
            {field(
              'pids_limit',
              t('pids.label'),
              t('pids.hint', { max: bounds.pids_limit_max }),
              num('pids_limit', pids, setPids),
            )}
          </div>
        </div>
        {has('init') && (
          <div className="flex items-center gap-2">
            <Switch
              id={`${base}-init`}
              checked={init}
              onCheckedChange={setInit}
              disabled={!writable || save.isPending}
              aria-label={t('init.label')}
            />
            <Label htmlFor={`${base}-init`}>{t('init.label')}</Label>
            <span className="text-xs text-muted-foreground">
              {t('init.hint')}
            </span>
          </div>
        )}
        {failure && (
          <Alert variant="destructive" role="alert">
            <TriangleAlertGlassIcon size="1.7em" />
            <AlertDescription>{failure}</AlertDescription>
          </Alert>
        )}
        <div className="flex flex-wrap items-center gap-3">
          <Button
            size="sm"
            onClick={submit}
            disabled={!writable || save.isPending}
          >
            {save.isPending ? t('saving') : t('save')}
          </Button>
          <p className="text-xs text-muted-foreground">
            {writable ? t('recreates') : t('readOnly')}
          </p>
        </div>
      </CardContent>
    </Card>
  )
}
