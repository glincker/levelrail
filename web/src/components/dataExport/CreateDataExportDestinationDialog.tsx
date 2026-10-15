import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusIcon } from '@phosphor-icons/react/dist/ssr'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { BrandIcon } from '../BrandIcon'
import { VendorFields } from './VendorFields'
import { KIND_MARKS } from '../../lib/dataExportKinds'
import { useCreateDataExportDestination } from '../../queries/dataExport'
import { useStorageDestinationsOptional } from '../../queries/storage'
import type {
  CreateDataExportDestinationRequest,
  DataExportKind,
} from '../../types/dataExport'

const KINDS: DataExportKind[] = [
  'logs_newrelic',
  'logs_datadog',
  'logs_axiom',
  'logs_betterstack',
  'logs_loki',
  'logs_otlp',
  'logs_http',
  'logs_syslog',
  'audit_webhook',
  'audit_s3',
  'metrics_otlp',
  'metrics_remote_write',
]

const KIND_KEYS = {
  logs_http: 'kinds.logs_http',
  logs_loki: 'kinds.logs_loki',
  logs_otlp: 'kinds.logs_otlp',
  logs_syslog: 'kinds.logs_syslog',
  logs_newrelic: 'kinds.logs_newrelic',
  logs_datadog: 'kinds.logs_datadog',
  logs_axiom: 'kinds.logs_axiom',
  logs_betterstack: 'kinds.logs_betterstack',
  audit_webhook: 'kinds.audit_webhook',
  audit_s3: 'kinds.audit_s3',
  metrics_otlp: 'kinds.metrics_otlp',
  metrics_remote_write: 'kinds.metrics_remote_write',
} as const satisfies Record<DataExportKind, string>

const LEVELS = ['any', 'debug', 'info', 'warn', 'error'] as const

const LEVEL_KEYS = {
  any: 'levels.any',
  debug: 'levels.debug',
  info: 'levels.info',
  warn: 'levels.warn',
  error: 'levels.error',
} as const

const isLogKind = (k: DataExportKind) => k.startsWith('logs_')
const isVendorKind = (k: DataExportKind) =>
  k === 'logs_newrelic' ||
  k === 'logs_datadog' ||
  k === 'logs_axiom' ||
  k === 'logs_betterstack'
const isMetricsKind = (k: DataExportKind) => k.startsWith('metrics_')
const hasAppFilter = (k: DataExportKind) => isLogKind(k) || isMetricsKind(k)

function parseHeaders(raw: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const line of raw.split('\n')) {
    const idx = line.indexOf(':')
    if (idx <= 0) continue
    const name = line.slice(0, idx).trim()
    const value = line.slice(idx + 1).trim()
    if (name && value) out[name] = value
  }
  return out
}

export function CreateDataExportDestinationDialog() {
  const { t } = useTranslation('dataExport')
  const create = useCreateDataExportDestination()
  const storage = useStorageDestinationsOptional()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [kind, setKind] = useState<DataExportKind>('logs_loki')
  const [endpoint, setEndpoint] = useState('')
  const [apps, setApps] = useState('')
  const [level, setLevel] = useState<(typeof LEVELS)[number]>('any')
  const [headers, setHeaders] = useState('')
  const [hmacKey, setHmacKey] = useState('')
  const [target, setTarget] = useState('')
  const [prefix, setPrefix] = useState('')
  const [interval, setInterval] = useState('')
  const [backfill, setBackfill] = useState(false)
  const [apiKey, setApiKey] = useState('')
  const [region, setRegion] = useState('us')
  const [site, setSite] = useState('')
  const [dataset, setDataset] = useState('')
  const [username, setUsername] = useState('')

  function reset() {
    setName('')
    setEndpoint('')
    setApps('')
    setLevel('any')
    setHeaders('')
    setHmacKey('')
    setTarget('')
    setPrefix('')
    setInterval('')
    setBackfill(false)
    setApiKey('')
    setUsername('')
    setRegion('us')
    setSite('')
    setDataset('')
    create.reset()
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const req: CreateDataExportDestinationRequest = {
      name: name.trim(),
      kind,
      enabled: true,
    }
    if (kind !== 'audit_s3' && endpoint.trim() !== '') {
      req.endpoint = endpoint.trim()
    }
    if (
      isVendorKind(kind) ||
      (kind === 'metrics_remote_write' && apiKey.trim())
    )
      req.api_key = apiKey.trim()
    if (hasAppFilter(kind)) {
      const list = apps
        .split(',')
        .map((a) => a.trim())
        .filter(Boolean)
      if (list.length > 0) req.apps = list
    }
    if (isLogKind(kind) && level !== 'any') req.min_level = level
    const parsed = parseHeaders(headers)
    if (Object.keys(parsed).length > 0 && kind !== 'logs_syslog') {
      req.headers = parsed
    }
    if (kind === 'audit_webhook' && hmacKey) req.hmac_key = hmacKey
    const options: NonNullable<CreateDataExportDestinationRequest['options']> =
      {}
    if (kind === 'audit_s3') {
      options.storage_target_id = target
      if (prefix.trim()) options.prefix = prefix.trim()
    }
    if (kind === 'metrics_remote_write' && username.trim()) {
      options.username = username.trim()
    }
    if (isMetricsKind(kind) && interval) {
      options.interval_seconds = Number(interval)
    }
    if ((kind === 'audit_s3' || kind === 'audit_webhook') && backfill) {
      options.backfill = true
    }
    if (kind === 'logs_newrelic' || kind === 'logs_axiom') {
      options.region = region
    }
    if (kind === 'logs_datadog' && site.trim()) options.site = site.trim()
    if (kind === 'logs_axiom') options.dataset = dataset.trim()
    if (Object.keys(options).length > 0) req.options = options

    create.mutate(req, {
      onSuccess: (created) => {
        setOpen(false)
        reset()
        toast.add({
          title: t('form.created', { name: created.name }),
          type: 'success',
        })
      },
    })
  }

  const endpointPlaceholder = {
    logs_http: t('form.endpointHttp'),
    logs_loki: t('form.endpointLoki'),
    logs_otlp: t('form.endpointOtlp'),
    logs_syslog: t('form.endpointSyslog'),
    logs_newrelic: t('form.endpointOverride'),
    logs_datadog: t('form.endpointOverride'),
    logs_axiom: t('form.endpointOverride'),
    logs_betterstack: t('form.endpointOverride'),
    audit_webhook: t('form.endpointHttp'),
    audit_s3: '',
    metrics_otlp: t('form.endpointOtlp'),
    metrics_remote_write: t('form.endpointRemoteWrite'),
  }[kind]

  const targets = storage.data ?? []
  const canSubmit =
    name.trim() !== '' &&
    (kind === 'audit_s3'
      ? target !== ''
      : isVendorKind(kind)
        ? apiKey.trim() !== '' &&
          (kind !== 'logs_axiom' || dataset.trim() !== '')
        : endpoint !== '')

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) reset()
      }}
    >
      <DialogTrigger render={<Button />}>
        <PlusIcon className="size-3.5" aria-hidden="true" />
        {t('add')}
      </DialogTrigger>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('form.title')}</DialogTitle>
          <DialogDescription>{t('form.description')}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-4">
          <Field>
            <FieldLabel htmlFor="export-name">{t('form.name')}</FieldLabel>
            <Input
              id="export-name"
              value={name}
              placeholder={t('form.namePlaceholder')}
              onChange={(e) => {
                setName(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="export-kind">{t('form.kind')}</FieldLabel>
            <Select
              value={kind}
              onValueChange={(v) => {
                if (typeof v === 'string') setKind(v)
              }}
            >
              <SelectTrigger id="export-kind" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {KINDS.map((k) => (
                  <SelectItem key={k} value={k}>
                    <span className="flex items-center gap-2">
                      {KIND_MARKS[k] ? (
                        <BrandIcon name={KIND_MARKS[k]} className="size-4" />
                      ) : (
                        <span className="size-4" aria-hidden="true" />
                      )}
                      {t(KIND_KEYS[k])}
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          {kind === 'audit_s3' ? (
            <>
              <Field>
                <FieldLabel htmlFor="export-target">
                  {t('form.storageTarget')}
                </FieldLabel>
                <Select
                  value={target}
                  onValueChange={(v) => {
                    if (typeof v === 'string') setTarget(v)
                  }}
                >
                  <SelectTrigger id="export-target" className="w-full">
                    <SelectValue
                      placeholder={t('form.storageTargetPlaceholder')}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {targets.map((d) => (
                      <SelectItem key={d.id} value={d.id}>
                        {d.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {targets.length === 0 ? (
                  <FieldDescription>
                    {t('form.storageTargetNone')}
                  </FieldDescription>
                ) : null}
              </Field>
              <Field>
                <FieldLabel htmlFor="export-prefix">
                  {t('form.prefix')}
                </FieldLabel>
                <Input
                  id="export-prefix"
                  value={prefix}
                  placeholder="audit-log"
                  onChange={(e) => {
                    setPrefix(e.target.value)
                  }}
                />
              </Field>
            </>
          ) : (
            <Field>
              <FieldLabel htmlFor="export-endpoint">
                {t('form.endpoint')}
              </FieldLabel>
              <Input
                id="export-endpoint"
                value={endpoint}
                placeholder={endpointPlaceholder}
                onChange={(e) => {
                  setEndpoint(e.target.value)
                }}
              />
            </Field>
          )}

          {isVendorKind(kind) ? (
            <VendorFields
              kind={kind}
              values={{ apiKey, region, site, dataset }}
              onChange={(next) => {
                if (next.apiKey !== undefined) setApiKey(next.apiKey)
                if (next.region !== undefined) setRegion(next.region)
                if (next.site !== undefined) setSite(next.site)
                if (next.dataset !== undefined) setDataset(next.dataset)
              }}
            />
          ) : null}

          {hasAppFilter(kind) ? (
            <Field>
              <FieldLabel htmlFor="export-apps">{t('form.apps')}</FieldLabel>
              <Input
                id="export-apps"
                value={apps}
                placeholder="web, api"
                onChange={(e) => {
                  setApps(e.target.value)
                }}
              />
            </Field>
          ) : null}

          {isLogKind(kind) ? (
            <>
              <Field>
                <FieldLabel htmlFor="export-level">
                  {t('form.minLevel')}
                </FieldLabel>
                <Select
                  value={level}
                  onValueChange={(v) => {
                    if (typeof v === 'string') {
                      setLevel(v)
                    }
                  }}
                >
                  <SelectTrigger id="export-level" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {LEVELS.map((l) => (
                      <SelectItem key={l} value={l}>
                        {t(LEVEL_KEYS[l])}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </>
          ) : null}

          {kind !== 'logs_syslog' && kind !== 'audit_s3' ? (
            <Field>
              <FieldLabel htmlFor="export-headers">
                {t('form.headers')}
              </FieldLabel>
              <Textarea
                id="export-headers"
                value={headers}
                autoComplete="off"
                rows={3}
                onChange={(e) => {
                  setHeaders(e.target.value)
                }}
              />
              <FieldDescription>{t('form.headersHint')}</FieldDescription>
            </Field>
          ) : null}

          {kind === 'audit_webhook' ? (
            <Field>
              <FieldLabel htmlFor="export-hmac">{t('form.hmacKey')}</FieldLabel>
              <Input
                id="export-hmac"
                type="password"
                autoComplete="off"
                value={hmacKey}
                onChange={(e) => {
                  setHmacKey(e.target.value)
                }}
              />
              <FieldDescription>{t('form.hmacHint')}</FieldDescription>
            </Field>
          ) : null}

          {kind === 'metrics_remote_write' ? (
            <>
              <Field>
                <FieldLabel htmlFor="export-rw-user">
                  {t('form.username')}
                </FieldLabel>
                <Input
                  id="export-rw-user"
                  autoComplete="off"
                  value={username}
                  onChange={(e) => {
                    setUsername(e.target.value)
                  }}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="export-rw-key">
                  {t('form.remoteWriteKey')}
                </FieldLabel>
                <Input
                  id="export-rw-key"
                  type="password"
                  autoComplete="off"
                  value={apiKey}
                  onChange={(e) => {
                    setApiKey(e.target.value)
                  }}
                />
                <FieldDescription>
                  {t('form.remoteWriteKeyHint')}
                </FieldDescription>
              </Field>
            </>
          ) : null}

          {isMetricsKind(kind) ? (
            <Field>
              <FieldLabel htmlFor="export-interval">
                {t('form.interval')}
              </FieldLabel>
              <Input
                id="export-interval"
                type="number"
                min={10}
                max={3600}
                value={interval}
                placeholder="60"
                onChange={(e) => {
                  setInterval(e.target.value)
                }}
              />
            </Field>
          ) : null}

          {kind === 'audit_s3' || kind === 'audit_webhook' ? (
            <label className="flex items-center gap-2 text-sm text-foreground">
              <Switch checked={backfill} onCheckedChange={setBackfill} />
              {t('form.backfill')}
            </label>
          ) : null}

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
            <Button type="submit" disabled={!canSubmit || create.isPending}>
              {create.isPending ? t('form.submitting') : t('form.submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
